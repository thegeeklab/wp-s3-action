package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Client struct {
	S3         *S3
	Cloudfront *Cloudfront
}

// NewClient creates a new S3 client with the provided configuration.
func NewClient(
	ctx context.Context,
	url, region, accessKey, secretKey string,
	pathStyle bool,
	cm ChecksumMode,
) (*Client, error) {
	checksumModeMap := map[ChecksumMode]aws.RequestChecksumCalculation{
		ChecksumSupported: aws.RequestChecksumCalculationWhenSupported,
		ChecksumRequired:  aws.RequestChecksumCalculationWhenRequired,
	}

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("error while loading AWS config: %w", err)
	}

	if url != "" {
		cfg.BaseEndpoint = aws.String(url)
	}

	// allowing to use the instance role or provide a key and secret
	if accessKey != "" && secretKey != "" {
		cfg.Credentials = credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
	}

	c := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = pathStyle
		o.RequestChecksumCalculation = checksumModeMap[cm]
	})
	cf := cloudfront.NewFromConfig(cfg)

	return &Client{
		S3:         &S3{client: c},
		Cloudfront: &Cloudfront{client: cf},
	}, nil
}
