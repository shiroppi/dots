<div align="center">
    <img src="docs/icon.png" alt="dots logo" width="256" height="256">
    <h1>dots</h1>
</div>

> A brief, declarative, flexible, cross-platform dotfiles manager.

Your dotfiles, your environment.

A small, declarative dotfiles manager for people who want their configuration to stay simple.

> [!NOTE]
> Although this software is under construction, it currently works largely in accordance with the v0.1 architecture document.

## Features

- Brief, declarative configuration
- Flexible dotfiles structure
- Cross-platform support (Windows, macOS, Linux, FreeBSD)
- Beautiful, modern CLI UX

## Configuration

```toml
[dots]
"~/.vimrc" = "vimrc"
"~/.config/nvim" = "nvim"

[dots.windows]
"~/AppData/Local/nvim" = "nvim"

[[auto]]
source = "config"
target = "~/.config"
ignore = ["README.md", "**/.DS_Store"]

[[auto.darwin]]
source = "config/darwin"
target = "~/.config"
```

* `dots` — Map individual files or directories.
* `auto` — Recursively map a directory while preserving its structure.
* `[os]` — Define OS-specific configuration. (supported value: `windows`, `darwin`(macos), `linux`, `freebsd`)
* `ignore` — Skip paths inside an `auto` source. Patterns are relative to `source`, use `/` on every OS, and are case-sensitive.
  * `README.md` matches only the top-level `README.md`.
  * `*` matches within a single path element and never crosses `/`.
  * `**` matches zero or more directories and must be a whole path element, so `**/.DS_Store` matches `
.DS_Store` at any depth, including the top level.

### Directory structure

```
.
├── dots.toml
├── vimrc
├── nvim/
│   ├── init.lua
│   └── lua/
│       └── ...
└── config/
    ├── README.md
    ├── starship.toml
    ├── tmux/
    │   └── tmux.conf
    └── darwin/
        ├── karabiner/
        │   └── karabiner.json
        └── yabai/
            └── yabairc
```

This is just an example. Use any directory structure you like.

### Priority

```
dots.[os] > dots > auto.[os] > auto
```

When multiple rules target the same path, the highest-priority rule is used. Rules with the same priority result in an error.

## Installation

```bash
go install github.com/shiroppi/dots/cmd/dots@latest
```

*Requires Go 1.22 or later.*

## Quick Start

1. Initialize a new configuration in your dotfiles repository:
   ```bash
   dots init
   ```
2. Edit the newly created `dots.toml` to define your environment and links referring to [Configuration](#configuration):
   ```bash
   dots edit
   # Use this instead of "dots edit" if you don't set a $EDITOR environment variable.
   vim ~/your-dotfiles-path/dots.toml
   ```
3. Apply the configuration (creates symlinks):
   ```bash
   dots apply
   ```

> [!IMPORTANT]
> On Windows, creating symlinks requires either `Developer Mode` to be enabled or the user who has the `SeCreateSymbolicLinkPrivilege` (usually Administrator).
