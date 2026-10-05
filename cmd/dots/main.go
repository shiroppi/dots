package main

import (
	"os"

	"github.com/shiroppi/dots/internal/cli"
)

var version = "dev" // overridden by -ldflags "-X main.version=v0.1.0"

func main() { os.Exit(cli.Main(version)) }
