package main

import (
	"os"

	"github.com/aeen/v2ray-scrapper/internal/cli"
)

var buildVersion = "dev"

func main() {
	cli.Version = buildVersion
	os.Exit(cli.RunCLI(os.Args))
}
