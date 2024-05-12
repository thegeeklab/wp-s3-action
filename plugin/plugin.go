package plugin

import (
	"fmt"

	wp "github.com/thegeeklab/wp-plugin-go/v2/plugin"
	"github.com/thegeeklab/wp-plugin-go/v2/types"
	"github.com/urfave/cli/v2"
)

//go:generate go run ../internal/doc/main.go -output=../docs/data/data-raw.yaml

// Plugin implements provide the plugin implementation.
type Plugin struct {
	*wp.Plugin
	Settings *Settings
}

// Settings for the Plugin.
type Settings struct {
	Endpoint               string
	AccessKey              string
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

func New(e wp.ExecuteFunc, build ...string) *Plugin {
	p := &Plugin{
		Settings: &Settings{},
	}

	options := wp.Options{
		Name:                "wp-s3-action",
		Description:         "Perform S3 actions",
		Flags:               Flags(p.Settings, wp.FlagsPluginCategory),
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

	p.Plugin = wp.New(options)

	return p
}

// Flags returns a slice of CLI flags for the plugin.
func Flags(settings *Settings, category string) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "endpoint",
			Usage:       "endpoint for the s3 connection",
			EnvVars:     []string{"PLUGIN_ENDPOINT", "S3_ENDPOINT"},
			Destination: &settings.Endpoint,
			Category:    category,
		},
		&cli.StringFlag{
			Name:        "access-key",
			Usage:       "s3 access key",
			EnvVars:     []string{"PLUGIN_ACCESS_KEY", "S3_ACCESS_KEY"},
			Destination: &settings.AccessKey,
			Required:    true,
			Category:    category,
		},
		&cli.StringFlag{
			Name:        "secret-key",
			Usage:       "s3 secret key",
			EnvVars:     []string{"PLUGIN_SECRET_KEY", "S3_SECRET_KEY"},
			Destination: &settings.SecretKey,
			Required:    true,
			Category:    category,
		},
		&cli.BoolFlag{
			Name:        "path-style",
			Usage:       "enable path style for bucket paths",
			EnvVars:     []string{"PLUGIN_PATH_STYLE"},
			Destination: &settings.PathStyle,
			Category:    category,
		},
		&cli.StringFlag{
			Name:        "bucket",
			Usage:       "name of the bucket",
			EnvVars:     []string{"PLUGIN_BUCKET"},
			Destination: &settings.Bucket,
			Required:    true,
			Category:    category,
		},
		&cli.StringFlag{
			Name:        "region",
			Usage:       "s3 region",
			Value:       "us-east-1",
			EnvVars:     []string{"PLUGIN_REGION"},
			Destination: &settings.Region,
			Category:    category,
		},
		&cli.StringFlag{
			Name:        "source",
			Usage:       "upload source path",
			Value:       ".",
			EnvVars:     []string{"PLUGIN_SOURCE"},
			Destination: &settings.Source,
			Category:    category,
		},
		&cli.StringFlag{
			Name:        "target",
			Usage:       "upload target path",
			Value:       "/",
			EnvVars:     []string{"PLUGIN_TARGET"},
			Destination: &settings.Target,
			Category:    category,
		},
		&cli.BoolFlag{
			Name:        "delete",
			Usage:       "delete locally removed files from the target",
			EnvVars:     []string{"PLUGIN_DELETE"},
			Destination: &settings.Delete,
			Category:    category,
		},
		&cli.GenericFlag{
			Name:     "acl",
			Usage:    "access control list",
			EnvVars:  []string{"PLUGIN_ACL"},
			Value:    &types.StringMapFlag{},
			Category: category,
		},
		&cli.GenericFlag{
			Name:     "content-type",
			Usage:    "content-type settings for uploads",
			EnvVars:  []string{"PLUGIN_CONTENT_TYPE"},
			Value:    &types.StringMapFlag{},
			Category: category,
		},
		&cli.GenericFlag{
			Name:     "content-encoding",
			Usage:    "content-encoding settings for uploads",
			EnvVars:  []string{"PLUGIN_CONTENT_ENCODING"},
			Value:    &types.StringMapFlag{},
			Category: category,
		},
		&cli.GenericFlag{
			Name:     "cache-control",
			Usage:    "cache-control settings for uploads",
			EnvVars:  []string{"PLUGIN_CACHE_CONTROL"},
			Value:    &types.StringMapFlag{},
			Category: category,
		},
		&cli.GenericFlag{
			Name:     "metadata",
			Usage:    "additional metadata for uploads",
			EnvVars:  []string{"PLUGIN_METADATA"},
			Value:    &types.DeepStringMapFlag{},
			Category: category,
		},
		&cli.GenericFlag{
			Name:     "redirects",
			Usage:    "redirects to create",
			EnvVars:  []string{"PLUGIN_REDIRECTS"},
			Value:    &types.MapFlag{},
			Category: category,
		},
		&cli.StringFlag{
			Name:        "cloudfront-distribution",
			Usage:       "ID of cloudfront distribution to invalidate",
			EnvVars:     []string{"PLUGIN_CLOUDFRONT_DISTRIBUTION"},
			Destination: &settings.CloudFrontDistribution,
			Category:    category,
		},
		&cli.BoolFlag{
			Name:        "dry-run",
			Usage:       "dry run disables api calls",
			EnvVars:     []string{"DRY_RUN", "PLUGIN_DRY_RUN"},
			Destination: &settings.DryRun,
			Category:    category,
		},
		&cli.IntFlag{
			Name:  "max-concurrency",
			Usage: "customize number concurrent files to process",
			//nolint:mnd
			Value:       100,
			EnvVars:     []string{"PLUGIN_MAX_CONCURRENCY"},
			Destination: &settings.MaxConcurrency,
			Category:    category,
		},
	}
}
