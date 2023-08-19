package plugin

import (
	wp "github.com/thegeeklab/wp-plugin-go/plugin"
	"github.com/thegeeklab/wp-plugin-go/types"
)

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
	ACL                    types.StringMapFlag
	CacheControl           types.StringMapFlag
	ContentType            types.StringMapFlag
	ContentEncoding        types.StringMapFlag
	Metadata               types.DeepStringMapFlag
	Redirects              types.StringMapFlag
	CloudFrontDistribution string
	DryRun                 bool
	PathStyle              bool
	Client                 AWS
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

func New(options wp.Options, settings *Settings) *Plugin {
	p := &Plugin{}

	options.Execute = p.run

	p.Plugin = wp.New(options)
	p.Settings = settings

	return p
}
