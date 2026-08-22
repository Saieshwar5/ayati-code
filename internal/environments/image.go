package environments

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

const dockerfileTemplate = `FROM public.ecr.aws/lambda/microvms:al2023-minimal
COPY vmagent /usr/local/bin/vmagent
EXPOSE 8080 9000
CMD ["/usr/local/bin/vmagent", "--data-addr", "0.0.0.0:8080", "--hooks-addr", "0.0.0.0:9000", "--root", "/workspace"]
`

// S3Putter uploads image artifacts. Production adapts aws-sdk-go-v2/service/s3;
// tests use an in-memory implementation.
type S3Putter interface {
	Put(ctx context.Context, bucket, key string, content io.Reader, size int64) error
}

// ImageBuilder packages vmagent into a Lambda MicroVM image.
type ImageBuilder struct {
	Name         string
	Bucket       string
	BuildRoleARN string
	BaseImageARN string
	AgentBinary  []byte
	API          API
	S3           S3Putter
}

// Build generates the zip, uploads it, and creates the microVM image via the
// control plane. No repository content or credentials are included.
func (b *ImageBuilder) Build(ctx context.Context) (ImageRef, error) {
	if b.Name == "" || b.Bucket == "" || b.BaseImageARN == "" || b.BuildRoleARN == "" {
		return ImageRef{}, fmt.Errorf("image name, bucket, base image, and build role are required")
	}
	if b.API == nil || b.S3 == nil {
		return ImageRef{}, fmt.Errorf("image api and s3 are required")
	}
	if len(b.AgentBinary) == 0 {
		return ImageRef{}, fmt.Errorf("vmagent binary is empty")
	}
	zipData, err := buildAgentZip(b.AgentBinary)
	if err != nil {
		return ImageRef{}, err
	}
	key := "images/" + b.Name + "/agent.zip"
	if err := b.S3.Put(ctx, b.Bucket, key, bytes.NewReader(zipData), int64(len(zipData))); err != nil {
		return ImageRef{}, fmt.Errorf("upload agent zip: %w", err)
	}
	if _, err := b.API.CreateMicrovmImage(ctx, ImageBuildInput{
		Name:         b.Name,
		S3URI:        "s3://" + b.Bucket + "/" + key,
		BuildRoleARN: b.BuildRoleARN,
		BaseImageARN: b.BaseImageARN,
	}); err != nil {
		return ImageRef{}, fmt.Errorf("create microvm image: %w", err)
	}
	return b.waitForImage(ctx)
}

func buildAgentZip(agentBinary []byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create("Dockerfile")
	if err != nil {
		return nil, fmt.Errorf("create Dockerfile entry: %w", err)
	}
	if _, err := io.Copy(file, strings.NewReader(dockerfileTemplate)); err != nil {
		return nil, err
	}
	agent, err := writer.CreateHeader(&zip.FileHeader{
		Name: "vmagent", Method: zip.Deflate,
	})
	if err != nil {
		return nil, fmt.Errorf("create vmagent entry: %w", err)
	}
	if _, err := agent.Write(agentBinary); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// waitForImage polls the image state until CREATED/UPDATED or a failure. The
// poll is bounded so a broken build fails the durable job instead of hanging.
func (b *ImageBuilder) waitForImage(ctx context.Context) (ImageRef, error) {
	const attempts = 60 // 60 * 5s = 5 min cap
	for attempt := 0; attempt < attempts; attempt++ {
		ref, err := b.API.GetMicrovmImage(ctx)
		if err == nil {
			switch ref.State {
			case "CREATED", "UPDATED":
				return ref, nil
			case "CREATE_FAILED", "UPDATE_FAILED", "DELETE_FAILED":
				return ImageRef{}, fmt.Errorf("microvm image build failed: %s", ref.State)
			}
		}
		select {
		case <-ctx.Done():
			return ImageRef{}, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return ImageRef{}, fmt.Errorf("timed out waiting for microvm image build")
}
