<div align="center">
    <img src="docs/icon_512x512.webp" alt="dots logo" width="256" height="256">
    <h1>dots</h1>
</div>

> A brief, declarative, flexible, cross-platform dotfiles manager.

Your dotfiles, your environment.

A small, declarative dotfiles manager for people who want their configuration to remain itself.

<img width="631" height="302" alt="image" src="https://github.com/user-attachments/assets/0ca23527-3514-472f-8124-54c54d4c527e" />

## Features

- Brief, declarative configuration
- Flexible dotfiles structure
- Cross-platform support (Windows, macOS, Linux, FreeBSD, OpenBSD)
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
ignore = ["README.md", ".DS_Store"]

[[auto.darwin]]
source = "config/darwin"
target = "~/.config"
os_only = true
```

* `dots` — Link individual files or directories.
* `auto` — Link each top-level item of `source` into `target` (`config/tmux` → `~/.config/tmux`).
* `[os]` — OS-specific rules. (supported value: `windows`, `darwin`(macos), `linux`, `freebsd`, `openbsd`)
* `os_only` — `auto.[os]` only. Excludes the rule's `source` from the common `auto` rules, so the contents of `config/darwin` are linked only on macOS.
* `ignore` — Top-level names to skip in an `auto` source. `*` is the only wildcard; case-sensitive.

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

*Requires Go 1.24 or later.*

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
