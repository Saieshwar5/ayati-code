package environments

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
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
	return b.API.CreateMicrovmImage(ctx)
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
