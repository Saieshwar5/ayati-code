package environments

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3 uploads image artifacts through the AWS SDK.
type S3 struct {
	client *s3.Client
}

// NewS3 builds an S3 client for the provided region.
func NewS3(region string) (*S3, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load aws config for s3: %w", err)
	}
	return &S3{client: s3.NewFromConfig(cfg)}, nil
}

// Put uploads content to bucket/key. The size parameter matches the VM
// object-store seam used by tests and kept for clarity/tracing.
func (s *S3) Put(ctx context.Context, bucket, key string, content io.Reader, _ int64) error {
	if bucket == "" || key == "" {
		return fmt.Errorf("s3 bucket and key are required")
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   content,
	})
	if err != nil {
		return fmt.Errorf("s3 put %s/%s: %w", bucket, key, err)
	}
	return nil
}
