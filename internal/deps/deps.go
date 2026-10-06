//go:build tools

// Package deps pins module dependencies that are required by the v0.1.0
// design before every package imports them. It is excluded from normal builds.
package deps

import (
	_ "github.com/charmbracelet/huh"
	_ "github.com/go-git/go-billy/v5/memfs"
	_ "github.com/pelletier/go-toml/v2"
	_ "github.com/spf13/cobra"
	_ "golang.org/x/term"
)
