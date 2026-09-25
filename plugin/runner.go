package plugin

import (
	"context"

	"github.com/thegeeklab/wp-s3-action/aws"
)

// S3Runner is the subset of *aws.S3 that the plugin drives. Declaring the
// interface in the consumer package (rather than reusing *aws.S3) lets tests
// substitute their own fake without depending on aws package internals, and
// keeps the aws package unaware of how downstream consumers mock it.
type S3Runner interface {
	Upload(ctx context.Context, opt aws.S3UploadOptions) (aws.UploadResult, error)
	Download(ctx context.Context, opt aws.S3DownloadOptions) error
	Redirect(ctx context.Context, opt aws.S3RedirectOptions) error
	Delete(ctx context.Context, opt aws.S3DeleteOptions) error
	List(ctx context.Context, opt aws.S3ListOptions, fn func(string) error) error
}

// CloudfrontRunner is the subset of *aws.Cloudfront used by the plugin.
type CloudfrontRunner interface {
	Invalidate(ctx context.Context, opt aws.CloudfrontInvalidateOptions) error
}
