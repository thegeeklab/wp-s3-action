package plugin

import (
	"fmt"

	plugin_cli "github.com/thegeeklab/wp-plugin-go/v6/cli"
	plugin_base "github.com/thegeeklab/wp-plugin-go/v6/plugin"
	"github.com/thegeeklab/wp-s3-action/aws"
	"github.com/urfave/cli/v3"
)

//go:generate go run ../hack/docs-gen/main.go -output=../docs/data/data.yaml

// Plugin implements provide the plugin implementation.
type Plugin struct {
	*plugin_base.Plugin
	Settings *Settings
}

// Settings for the Plugin.
type Settings struct {
	ActionStrings       []string
	Action              []S3Action
	Endpoint            string
	AccessKey           string
	SecretKey           string
	Bucket              string
	Region              string
	Source              string
	Target              string
	Redirects           map[string]string
	DryRun              bool
	PathStyle           bool
	ChecksumCalculation string
	MaxConcurrency      int

	Upload     Upload
	CloudFront CloudFront
}

type Upload struct {
	Delete           bool
	ACL              map[string]string
	CacheControl     map[string]string
	ContentType      map[string]string
	ContentEncoding  map[string]string
	Metadata         map[string]map[string]string
	AllowEmptySource bool
}

type CloudFront struct {
	Distribution string
}

type S3Action string

type Job struct {
	local     string
	remote    string
	remoteSet []string
	action    S3Action
}

func New(e plugin_base.ExecuteFunc, build ...string) *Plugin {
	p := &Plugin{
		Settings: &Settings{},
	}

	options := plugin_base.Options{
		Name:                "wp-s3-action",
		Description:         "Perform S3 actions",
		Flags:               Flags(p.Settings, plugin_base.FlagsPluginCategory),
		Execute:             p.run,
		HideWoodpeckerFlags: true,
	}

	if len(build) > 0 {
		options.Version = build[0]
	}

	if len(build) > 1 {
		options.VersionMetadata = fmt.Sprintf("date=%s", build[1])
	}

	if e != nil {
		options.Execute = e
	}

	p.Plugin = plugin_base.New(options)

	return p
}

// Flags returns a slice of CLI flags for the plugin.
func Flags(settings *Settings, category string) []cli.Flag {
	//nolint:mnd
	return []cli.Flag{
		// S3 action to execute. Actions are low-level operations that can be combined to
		// compose higher-level workflows such as a full sync. Supported actions: `upload`,
		// `download`, `delete`, `redirect`, `invalidate-cloudfront`.
		//
		// - **upload:** Uploads files from the local `source` directory to the S3 bucket `target` path.
		//   When combined with the `upload_delete` setting, remote files that no longer exist locally are removed.
		// - **download:** Downloads files from the S3 bucket `target` path to the local `source` directory.
		//   Requires an explicit non-empty `target` so the entire bucket cannot be pulled by accident.
		// - **delete:** Deletes all files from the S3 bucket `target` path.
		//   Requires an explicit non-empty `target` so the entire bucket cannot be wiped by accident.
		// - **redirect:** Creates redirect objects from the `redirects` setting under the `target` path.
		//   Requires a target server that supports the S3 website redirect feature.
		// - **invalidate-cloudfront:** Invalidates the configured CloudFront distribution path.
		//   This action is AWS-specific and does not work with S3-compatible endpoints such as Garage.
		&cli.StringSliceFlag{
			Name:        "action",
			Usage:       "S3 action to execute",
			Sources:     cli.EnvVars("PLUGIN_ACTION"),
			Destination: &settings.ActionStrings,
			Required:    true,
			Category:    category,
		},
		// Endpoint for the s3 connection.
		&cli.StringFlag{
			Name:        "endpoint",
			Usage:       "endpoint for the s3 connection",
			Sources:     cli.EnvVars("PLUGIN_ENDPOINT", "S3_ENDPOINT"),
			Destination: &settings.Endpoint,
			Category:    category,
		},
		// S3 access key.
		&cli.StringFlag{
			Name:        "access-key",
			Usage:       "s3 access key",
			Sources:     cli.EnvVars("PLUGIN_ACCESS_KEY", "S3_ACCESS_KEY", "AWS_ACCESS_KEY_ID"),
			Destination: &settings.AccessKey,
			Required:    true,
			Category:    category,
		},
		// S3 secret key.
		&cli.StringFlag{
			Name:        "secret-key",
			Usage:       "s3 secret key",
			Sources:     cli.EnvVars("PLUGIN_SECRET_KEY", "S3_SECRET_KEY", "AWS_SECRET_ACCESS_KEY"),
			Destination: &settings.SecretKey,
			Required:    true,
			Category:    category,
		},
		// Enable path style for bucket paths.
		&cli.BoolFlag{
			Name:        "path-style",
			Usage:       "enable path style for bucket paths",
			Sources:     cli.EnvVars("PLUGIN_PATH_STYLE"),
			Destination: &settings.PathStyle,
			Category:    category,
		},
		// Name of the bucket.
		&cli.StringFlag{
			Name:        "bucket",
			Usage:       "name of the bucket",
			Sources:     cli.EnvVars("PLUGIN_BUCKET"),
			Destination: &settings.Bucket,
			Required:    true,
			Category:    category,
		},
		// S3 region.
		&cli.StringFlag{
			Name:        "region",
			Usage:       "s3 region",
			Value:       "us-east-1",
			Sources:     cli.EnvVars("PLUGIN_REGION"),
			Destination: &settings.Region,
			Category:    category,
		},
		// Local working directory. Files are read from here during `upload` and written here
		// during `download`.
		&cli.StringFlag{
			Name:        "source",
			Usage:       "upload source path",
			Value:       ".",
			Sources:     cli.EnvVars("PLUGIN_SOURCE"),
			Destination: &settings.Source,
			Category:    category,
		},
		// S3 path prefix for the bucket operations. Used by all actions to scope the S3 key
		// namespace (a leading `/` is stripped). Empty means the bucket root. The `delete` and
		// `download` actions require an explicit non-empty value.
		&cli.StringFlag{
			Name:        "target",
			Usage:       "s3 key prefix used to scope the action (a leading '/' is stripped)",
			Value:       "",
			Sources:     cli.EnvVars("PLUGIN_TARGET"),
			Destination: &settings.Target,
			Category:    category,
		},
		// Delete remote files that are not present in the local source directory during
		// upload.
		&cli.BoolFlag{
			Name:        "upload.delete",
			Usage:       "delete locally removed files from the target",
			Sources:     cli.EnvVars("PLUGIN_UPLOAD_DELETE"),
			Destination: &settings.Upload.Delete,
			Category:    category,
		},
		// Access control list.
		&plugin_cli.StringMapFlag{
			Name:        "upload.acl",
			Usage:       "access control list",
			Sources:     cli.EnvVars("PLUGIN_UPLOAD_ACL"),
			Destination: &settings.Upload.ACL,
			Category:    category,
		},
		// Content-type settings for uploads.
		&plugin_cli.StringMapFlag{
			Name:        "upload.content-type",
			Usage:       "content-type settings for uploads",
			Sources:     cli.EnvVars("PLUGIN_UPLOAD_CONTENT_TYPE"),
			Destination: &settings.Upload.ContentType,
			Category:    category,
		},
		// Content-encoding settings for uploads.
		&plugin_cli.StringMapFlag{
			Name:        "upload.content-encoding",
			Usage:       "content-encoding settings for uploads",
			Sources:     cli.EnvVars("PLUGIN_UPLOAD_CONTENT_ENCODING"),
			Destination: &settings.Upload.ContentEncoding,
			Category:    category,
		},
		// Cache-control settings for uploads.
		&plugin_cli.StringMapFlag{
			Name:        "upload.cache-control",
			Usage:       "cache-control settings for uploads",
			Sources:     cli.EnvVars("PLUGIN_UPLOAD_CACHE_CONTROL"),
			Destination: &settings.Upload.CacheControl,
			Category:    category,
		},
		// Additional metadata for uploads.
		&plugin_cli.DeepStringMapFlag{
			Name:        "upload.metadata",
			Usage:       "additional metadata for uploads",
			Sources:     cli.EnvVars("PLUGIN_UPLOAD_METADATA"),
			Destination: &settings.Upload.Metadata,
			Category:    category,
		},
		// Map of source paths to redirect destinations. Each key is created as an S3 object
		// under the configured `target` path with the `x-amz-website-redirect-location` header
		// set to the corresponding value.
		//
		// The target server must support the S3 website redirect feature, so check your
		// provider's documentation for static website hosting or website redirect support
		// before using the `redirect` action.
		&plugin_cli.StringMapFlag{
			Name:        "redirects",
			Usage:       "redirects to create",
			Sources:     cli.EnvVars("PLUGIN_REDIRECTS"),
			Destination: &settings.Redirects,
			Category:    category,
		},
		// ID of cloudfront distribution to invalidate.
		//
		// CloudFront is an AWS service and is not supported by S3-compatible endpoints such
		// as Garage.
		&cli.StringFlag{
			Name:        "cloudfront.distribution",
			Usage:       "ID of cloudfront distribution to invalidate",
			Sources:     cli.EnvVars("PLUGIN_CLOUDFRONT_DISTRIBUTION"),
			Destination: &settings.CloudFront.Distribution,
			Category:    category,
		},
		// Dry run disables API calls. When enabled, the plugin logs the actions it would take
		// without actually uploading, downloading, deleting, or redirecting.
		&cli.BoolFlag{
			Name:        "dry-run",
			Usage:       "dry run disables api calls",
			Sources:     cli.EnvVars("DRY_RUN", "PLUGIN_DRY_RUN"),
			Destination: &settings.DryRun,
			Category:    category,
		},
		// Customize number concurrent files to process.
		&cli.IntFlag{
			Name:        "max-concurrency",
			Usage:       "customize number concurrent files to process",
			Value:       100,
			Sources:     cli.EnvVars("PLUGIN_MAX_CONCURRENCY"),
			Destination: &settings.MaxConcurrency,
			Category:    category,
		},
		// Checksum calculation mode. Supported values are `required` and `supported`. For
		// third-party S3 implementations, `required` must most likely be used.
		&cli.StringFlag{
			Name:        "checksum-calculation",
			Usage:       fmt.Sprintf("checksum calculation mode (%s or %s)", aws.ChecksumSupported, aws.ChecksumRequired),
			Sources:     cli.EnvVars("PLUGIN_CHECKSUM_CALCULATION"),
			Destination: &settings.ChecksumCalculation,
			Value:       string(aws.ChecksumRequired),
			Validator: func(s string) error {
				var mode aws.ChecksumMode

				return mode.Set(s)
			},
			Category: category,
		},
		// Allow empty source directory. By default this setting will prevent deleting all
		// files from the target if `upload_delete: true` is set and the source directory is
		// empty.
		&cli.BoolFlag{
			Name:        "upload.allow-empty-source",
			Usage:       "allow empty source directory",
			Sources:     cli.EnvVars("PLUGIN_UPLOAD_ALLOW_EMPTY_SOURCE"),
			Destination: &settings.Upload.AllowEmptySource,
			Category:    category,
		},
	}
}
