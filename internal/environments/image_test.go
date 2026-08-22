package environments

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"testing"
)

type memoryS3 struct {
	content []byte
}

func (m *memoryS3) Put(_ context.Context, bucket, key string, content io.Reader, size int64) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	if int64(len(data)) != size {
		return io.ErrUnexpectedEOF
	}
	m.content = data
	return nil
}

func TestImageBuilderPackagesZipAndCreatesImage(t *testing.T) {
	s3 := &memoryS3{}
	api := &fakeAPI{endpoint: "example.test"}
	builder := &ImageBuilder{
		Name: "perpetual-agent", Bucket: "perpetual-images",
		BuildRoleARN: "arn:build", BaseImageARN: "arn:base",
		AgentBinary: []byte("vmagent-bytes"), API: api, S3: s3,
	}
	ref, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if ref.ImageARN != "arn:image" || ref.Version != "1.0" {
		t.Fatalf("ref = %#v", ref)
	}
	reader, err := zip.NewReader(bytes.NewReader(s3.content), int64(len(s3.content)))
	if err != nil {
		t.Fatalf("zip reader: %v", err)
	}
	var names []string
	for _, file := range reader.File {
		names = append(names, file.Name)
		v, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		_ = v.Close()
	}
	if len(names) != 2 {
		t.Fatalf("zip entries = %v", names)
	}
	if builder.Name == "" || s3.content == nil {
		t.Fatal("image build did not run through s3")
	}
}
