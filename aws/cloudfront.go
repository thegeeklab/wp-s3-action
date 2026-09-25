package aws

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/rs/zerolog/log"
)

type Cloudfront struct {
	client       CloudfrontAPIClient
	Distribution string
}

// NewCloudfront wraps a CloudfrontAPIClient in the higher-level *Cloudfront
// used by the plugin.
func NewCloudfront(client CloudfrontAPIClient) *Cloudfront {
	return &Cloudfront{client: client}
}

type CloudfrontInvalidateOptions struct {
	Path string
}

// Invalidate invalidates the specified path in the CloudFront distribution.
func (c *Cloudfront) Invalidate(ctx context.Context, opt CloudfrontInvalidateOptions) error {
	log.Debug().Msgf("invalidating '%s'", opt.Path)

	_, err := c.client.CreateInvalidation(ctx, &cloudfront.CreateInvalidationInput{
		DistributionId: aws.String(c.Distribution),
		InvalidationBatch: &types.InvalidationBatch{
			CallerReference: aws.String(time.Now().Format(time.RFC3339Nano)),
			Paths: &types.Paths{
				Quantity: aws.Int32(1),
				Items: []string{
					opt.Path,
				},
			},
		},
	})

	return err
}
