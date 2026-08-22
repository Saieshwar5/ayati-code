package environments

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	mvms "github.com/aws/aws-sdk-go-v2/service/lambdamicrovms"
	"github.com/aws/aws-sdk-go-v2/service/lambdamicrovms/types"
)

// AWSLambdaAPI implements API with the AWS SDK v2 Lambdamicrovms client.
type AWSLambdaAPI struct {
	client *mvms.Client
}

// NewAWSLambdaAPI loads default AWS credentials for a region.
func NewAWSLambdaAPI(region string) (*AWSLambdaAPI, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return &AWSLambdaAPI{client: mvms.NewFromConfig(cfg)}, nil
}

func (a *AWSLambdaAPI) RunMicrovm(ctx context.Context, input RunMicrovmInput) (Instance, error) {
	out, err := a.client.RunMicrovm(ctx, &mvms.RunMicrovmInput{
		ImageIdentifier:  aws.String(input.ImageARN),
		ImageVersion:     aws.String(input.ImageVersion),
		ExecutionRoleArn: stringPointer(input.ExecutionRoleARN),
		IdlePolicy: &types.IdlePolicy{
			AutoResumeEnabled:        aws.Bool(true),
			MaxIdleDurationSeconds:   aws.Int32(900),
			SuspendedDurationSeconds: aws.Int32(300),
		},
	})
	if err != nil {
		return Instance{}, fmt.Errorf("run microvm: %w", err)
	}
	return Instance{
		MicrovmID: aws.ToString(out.MicrovmId),
		Endpoint:  aws.ToString(out.Endpoint),
		State:     string(out.State),
		ImageARN:  aws.ToString(out.ImageArn),
	}, nil
}

func (a *AWSLambdaAPI) AuthToken(ctx context.Context, microvmID string) (string, error) {
	out, err := a.client.CreateMicrovmAuthToken(ctx, &mvms.CreateMicrovmAuthTokenInput{
		MicrovmIdentifier:   aws.String(microvmID),
		ExpirationInMinutes: aws.Int32(30),
		AllowedPorts: []types.PortSpecification{
			&types.PortSpecificationMemberPort{Value: 8080},
		},
	})
	if err != nil {
		return "", fmt.Errorf("create microvm auth token: %w", err)
	}
	return out.AuthToken["X-aws-proxy-auth"], nil
}

func (a *AWSLambdaAPI) SuspendMicrovm(ctx context.Context, id string) error {
	_, err := a.client.SuspendMicrovm(ctx, &mvms.SuspendMicrovmInput{MicrovmIdentifier: aws.String(id)})
	return err
}

func (a *AWSLambdaAPI) ResumeMicrovm(ctx context.Context, id string) error {
	_, err := a.client.ResumeMicrovm(ctx, &mvms.ResumeMicrovmInput{MicrovmIdentifier: aws.String(id)})
	return err
}

func (a *AWSLambdaAPI) TerminateMicrovm(ctx context.Context, id string) error {
	_, err := a.client.TerminateMicrovm(ctx, &mvms.TerminateMicrovmInput{MicrovmIdentifier: aws.String(id)})
	return err
}

func (a *AWSLambdaAPI) GetMicrovm(ctx context.Context, id string) (Instance, error) {
	out, err := a.client.GetMicrovm(ctx, &mvms.GetMicrovmInput{MicrovmIdentifier: aws.String(id)})
	if err != nil {
		return Instance{}, err
	}
	return Instance{
		MicrovmID: aws.ToString(out.MicrovmId),
		Endpoint:  aws.ToString(out.Endpoint),
		State:     string(out.State),
		ImageARN:  aws.ToString(out.ImageArn),
	}, nil
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return aws.String(value)
}

func (a *AWSLambdaAPI) CreateMicrovmImage(ctx context.Context, input ImageBuildInput) (ImageRef, error) {
	out, err := a.client.CreateMicrovmImage(ctx, &mvms.CreateMicrovmImageInput{
		Name:         aws.String(input.Name),
		BaseImageArn: aws.String(input.BaseImageARN),
		BuildRoleArn: aws.String(input.BuildRoleARN),
		CodeArtifact: &types.CodeArtifactMemberUri{Value: input.S3URI},
		Hooks: &types.Hooks{
			Port: aws.Int32(9000),
			MicrovmHooks: &types.MicrovmHooks{
				Run:       types.HookStateEnabled,
				Suspend:   types.HookStateEnabled,
				Resume:    types.HookStateEnabled,
				Terminate: types.HookStateEnabled,
			},
			MicrovmImageHooks: &types.MicrovmImageHooks{
				Ready:    types.HookStateEnabled,
				Validate: types.HookStateDisabled,
			},
		},
	})
	if err != nil {
		return ImageRef{}, fmt.Errorf("create microvm image: %w", err)
	}
	return ImageRef{
		ImageARN: aws.ToString(out.ImageArn),
		State:    string(out.State),
	}, nil
}

func (a *AWSLambdaAPI) GetMicrovmImage(ctx context.Context) (ImageRef, error) {
	identifier := os.Getenv("PERPETUAL_LAMBDA_IMAGE_ARN")
	if identifier == "" {
		return ImageRef{}, fmt.Errorf("PERPETUAL_LAMBDA_IMAGE_ARN is required for image lookup")
	}
	out, err := a.client.GetMicrovmImage(ctx, &mvms.GetMicrovmImageInput{ImageIdentifier: aws.String(identifier)})
	if err != nil {
		return ImageRef{}, fmt.Errorf("get microvm image: %w", err)
	}
	return ImageRef{
		ImageARN: aws.ToString(out.ImageArn),
		State:    string(out.State),
	}, nil
}
