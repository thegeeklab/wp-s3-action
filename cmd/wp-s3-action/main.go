package main

import (
	"github.com/thegeeklab/wp-s3-action/plugin"
)

//nolint:gochecknoglobals
var (
	BuildVersion = "devel"
	BuildDate    = "00000000"
)

func main() {
	plugin.New(nil, BuildVersion, BuildDate).Run()
}
