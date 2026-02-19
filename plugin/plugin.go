package plugin

import (
	"fmt"

	plugin_cli "github.com/thegeeklab/wp-plugin-go/v6/cli"
	plugin_base "github.com/thegeeklab/wp-plugin-go/v6/plugin"
	"github.com/thegeeklab/wp-s3-action/aws"
	"github.com/urfave/cli/v3"
)

//go:generate go run ../internal/doc/main.go -output=../docs/data/data-raw.yaml

// Plugin implements provide the plugin implementation.
type Plugin struct {
	*plugin_base.Plugin
	Settings *Settings
}

// Settings for the Plugin.
type Settings struct {
	Endpoint               string
	AccessKey              string //nolint:gosec
	SecretKey              string
	Bucket                 string
	Region                 string
	Source                 string
	Target                 string
	Delete                 bool
	ACL                    map[string]string
	CacheControl           map[string]string
	ContentType            map[string]string
	ContentEncoding        map[string]string
	Metadata               map[string]map[string]string
	Redirects              map[string]string
	CloudFrontDistribution string
	DryRun                 bool
	PathStyle              bool
	AllowEmptySource       bool
	ChecksumCalculation    string
	Jobs                   []Job
	MaxConcurrency         int
}

type Job struct {
	local  string
	remote string
	action string
}

type Result struct {
	j   Job
	err error
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
		&cli.StringFlag{
			Name:        "endpoint",
			Usage:       "endpoint for the s3 connection",
			Sources:     cli.EnvVars("PLUGIN_ENDPOINT", "S3_ENDPOINT"),
			Destination: &settings.Endpoint,
			Category:    category,
		},
		&cli.StringFlag{
			Name:        "access-key",
			Usage:       "s3 access key",
			Sources:     cli.EnvVars("PLUGIN_ACCESS_KEY", "S3_ACCESS_KEY"),
			Destination: &settings.AccessKey,
			Required:    true,
			Category:    category,
		},
		&cli.StringFlag{
			Name:        "secret-key",
			Usage:       "s3 secret key",
			Sources:     cli.EnvVars("PLUGIN_SECRET_KEY", "S3_SECRET_KEY"),
			Destination: &settings.SecretKey,
			Required:    true,
			Category:    category,
		},
		&cli.BoolFlag{
			Name:        "path-style",
			Usage:       "enable path style for bucket paths",
			Sources:     cli.EnvVars("PLUGIN_PATH_STYLE"),
			Destination: &settings.PathStyle,
			Category:    category,
		},
		&cli.StringFlag{
			Name:        "bucket",
			Usage:       "name of the bucket",
			Sources:     cli.EnvVars("PLUGIN_BUCKET"),
			Destination: &settings.Bucket,
			Required:    true,
			Category:    category,
		},
		&cli.StringFlag{
			Name:        "region",
			Usage:       "s3 region",
			Value:       "us-east-1",
			Sources:     cli.EnvVars("PLUGIN_REGION"),
			Destination: &settings.Region,
			Category:    category,
		},
		&cli.StringFlag{
			Name:        "source",
			Usage:       "upload source path",
			Value:       ".",
			Sources:     cli.EnvVars("PLUGIN_SOURCE"),
			Destination: &settings.Source,
			Category:    category,
		},
		&cli.StringFlag{
			Name:        "target",
			Usage:       "upload target path",
			Value:       "/",
			Sources:     cli.EnvVars("PLUGIN_TARGET"),
			Destination: &settings.Target,
			Category:    category,
		},
		&cli.BoolFlag{
			Name:        "delete",
			Usage:       "delete locally removed files from the target",
			Sources:     cli.EnvVars("PLUGIN_DELETE"),
			Destination: &settings.Delete,
			Category:    category,
		},
		&plugin_cli.StringMapFlag{
			Name:        "acl",
			Usage:       "access control list",
			Sources:     cli.EnvVars("PLUGIN_ACL"),
			Destination: &settings.ACL,
			Category:    category,
		},
		&plugin_cli.StringMapFlag{
			Name:        "content-type",
			Usage:       "content-type settings for uploads",
			Sources:     cli.EnvVars("PLUGIN_CONTENT_TYPE"),
			Destination: &settings.ContentType,
			Category:    category,
		},
		&plugin_cli.StringMapFlag{
			Name:        "content-encoding",
			Usage:       "content-encoding settings for uploads",
			Sources:     cli.EnvVars("PLUGIN_CONTENT_ENCODING"),
			Destination: &settings.ContentEncoding,
			Category:    category,
		},
		&plugin_cli.StringMapFlag{
			Name:        "cache-control",
			Usage:       "cache-control settings for uploads",
			Sources:     cli.EnvVars("PLUGIN_CACHE_CONTROL"),
			Destination: &settings.CacheControl,
			Category:    category,
		},
		&plugin_cli.DeepStringMapFlag{
			Name:        "metadata",
			Usage:       "additional metadata for uploads",
			Sources:     cli.EnvVars("PLUGIN_METADATA"),
			Destination: &settings.Metadata,
			Category:    category,
		},
		&plugin_cli.StringMapFlag{
			Name:        "redirects",
			Usage:       "redirects to create",
			Sources:     cli.EnvVars("PLUGIN_REDIRECTS"),
			Destination: &settings.Redirects,
			Category:    category,
		},
		&cli.StringFlag{
			Name:        "cloudfront-distribution",
			Usage:       "ID of cloudfront distribution to invalidate",
			Sources:     cli.EnvVars("PLUGIN_CLOUDFRONT_DISTRIBUTION"),
			Destination: &settings.CloudFrontDistribution,
			Category:    category,
		},
		&cli.BoolFlag{
			Name:        "dry-run",
			Usage:       "dry run disables api calls",
			Sources:     cli.EnvVars("DRY_RUN", "PLUGIN_DRY_RUN"),
			Destination: &settings.DryRun,
			Category:    category,
		},
		&cli.IntFlag{
			Name:        "max-concurrency",
			Usage:       "customize number concurrent files to process",
			Value:       100,
			Sources:     cli.EnvVars("PLUGIN_MAX_CONCURRENCY"),
			Destination: &settings.MaxConcurrency,
			Category:    category,
		},
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
		&cli.BoolFlag{
			Name:        "allow-empty-source",
			Usage:       "allow empty source directory",
			Sources:     cli.EnvVars("PLUGIN_ALLOW_EMPTY_SOURCE"),
			Destination: &settings.AllowEmptySource,
			Category:    category,
		},
	}
}
