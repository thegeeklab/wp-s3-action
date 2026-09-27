package main

import (
	"github.com/thegeeklab/wp-s3-action/plugin"
)

//nolint:gochecknoglobals
var (
	// BuildVersion is the semantic version injected at build time.
	BuildVersion = "devel"
	// BuildDate is the build date injected at build time.
	BuildDate = "00000000"
)

func main() {
	plugin.New(nil, BuildVersion, BuildDate).Run()
}
