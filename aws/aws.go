package aws

import (
	"context"
	"fmt"
	"net/http"

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
// When httpClient is non-nil it is used for all AWS requests, which is the
// mechanism for honoring framework settings such as insecure_skip_verify.
func NewClient(
	ctx context.Context,
	url, region, accessKey, secretKey string,
	pathStyle bool,
	cm string,
	httpClient *http.Client,
) (*Client, error) {
	var checksumMode ChecksumMode

	if err := checksumMode.Set(cm); err != nil {
		return nil, fmt.Errorf("error while setting checksum mode: %w", err)
	}

	checksumModeMap := map[ChecksumMode]aws.RequestChecksumCalculation{
		ChecksumSupported: aws.RequestChecksumCalculationWhenSupported,
		ChecksumRequired:  aws.RequestChecksumCalculationWhenRequired,
	}

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}
	if httpClient != nil {
		loadOpts = append(loadOpts, config.WithHTTPClient(httpClient))
	}

	cfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("error while loading AWS config: %w", err)
	}

	// allowing to use the instance role or provide a key and secret
	if accessKey != "" && secretKey != "" {
		cfg.Credentials = credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
	}

	c := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if url != "" {
			o.BaseEndpoint = aws.String(url)
		}

		o.UsePathStyle = pathStyle
		o.RequestChecksumCalculation = checksumModeMap[checksumMode]
	})
	cf := cloudfront.NewFromConfig(cfg)

	return &Client{
		S3:         &S3{client: c},
		Cloudfront: &Cloudfront{client: cf},
	}, nil
}

// NewTestClient constructs a Client wired to the provided mock S3 and CloudFront
// clients. It is intended for tests of downstream packages that need to inject
// mock API clients without going through the real AWS configuration loader.
func NewTestClient(s3Client S3APIClient, cfClient CloudfrontAPIClient) *Client {
	return &Client{
		S3:         &S3{client: s3Client},
		Cloudfront: &Cloudfront{client: cfClient},
	}
}
