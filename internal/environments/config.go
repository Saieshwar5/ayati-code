// Package environments owns provider-side microVM image and instance
// management. The local development runtime remains the default.
package environments

import (
	"os"
	"strings"
)

// Config selects the Lambda MicroVMs provider and image.
type Config struct {
	Region           string
	ImageARN         string
	ImageVersion     string
	BuildRoleARN     string
	ExecutionRoleARN string
	S3Bucket         string
	EndpointBase     string // test override for the control-plane API client
}

// LoadLambdaConfig reads Lambda settings from the environment.
func LoadLambdaConfig() Config {
	return Config{
		Region:           strings.TrimSpace(os.Getenv("PERPETUAL_AWS_REGION")),
		ImageARN:         strings.TrimSpace(os.Getenv("PERPETUAL_LAMBDA_IMAGE_ARN")),
		ImageVersion:     strings.TrimSpace(os.Getenv("PERPETUAL_LAMBDA_IMAGE_VERSION")),
		BuildRoleARN:     strings.TrimSpace(os.Getenv("PERPETUAL_AWS_BUILD_ROLE_ARN")),
		ExecutionRoleARN: strings.TrimSpace(os.Getenv("PERPETUAL_AWS_EXECUTION_ROLE_ARN")),
		S3Bucket:         strings.TrimSpace(os.Getenv("PERPETUAL_AWS_S3_BUCKET")),
	}
}

// Validate rejects a lambda runtime without enough information.
func (c Config) Validate() error {
	if c.Region == "" {
		return errMissing("PERPETUAL_AWS_REGION")
	}
	if c.ImageARN == "" {
		return errMissing("PERPETUAL_LAMBDA_IMAGE_ARN")
	}
	return nil
}

func errMissing(name string) error {
	return &missingEnvError{name: name}
}

type missingEnvError struct{ name string }

func (e *missingEnvError) Error() string { return e.name + " is required" }
