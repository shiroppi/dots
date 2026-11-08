<div align="center">
    <img src="docs/icon_512x512.webp" alt="dots logo" width="256" height="256">
    <h1>dots</h1>
</div>

> A brief, declarative, user-defined dotfiles manager.

Your dotfiles, your environment.

A small, declarative dotfiles manager for people who want their configuration like directory structure to remain itself.

<img width="631" height="302" alt="image" src="https://github.com/user-attachments/assets/0ca23527-3514-472f-8124-54c54d4c527e" />

## Features

- Brief, declarative configuration
- User-defined dotfiles structure
- Cross-platform support (Windows, macOS, Linux, FreeBSD, OpenBSD)

Although other operating systems supported by [GOOS](https://pkg.go.dev/internal/platform#pkg-variables), except for AIX, iOS, JavaScript, WASI and Plan9 have not been tested with unit and e2e tests, they are expected to work in most cases, as defined in [config.go](https://github.com/shiroppi/dots/blob/dev/internal/config/config.go#L113-L118).

## Configuration

```toml
[ui]
fold_unchanged = true
fold_unchanged_keep = 2

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
```

* File placement
  * `auto` — Link each top-level item of `source` into `target` (`config/tmux` → `~/.config/tmux`).
  * `[os]` — OS-specific rules. (supported value: `windows`, `darwin`(macos), `linux`, `freebsd`, `openbsd`)
  * `ignore` — Top-level names to skip in an `auto` source. `*` is the only wildcard; case-sensitive.

* UI
  * `fold_unchanged` (boolean, default `true`) — Fold unchanged (`✓`) rows into one `… N unchanged` line.
  * `fold_unchanged_keep` (integer >= 0, default `2`) — How many unchanged rows stay visible when folding.

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

*Requires Go 1.26 or later.*

## Quick Start

1. Initialize a new configuration in your dotfiles repository:
   ```bash
   dots init
   ```
2. Edit the newly created `dots.toml` to define your environment and links referring to [Configuration](#configuration):
   ```bash
   vim dots.toml
   ```
3. Apply the configuration (creates symlinks):
   ```bash
   dots apply
   ```

   ```
     ✓ ~/.config/fish
     ✓ ~/.config/nvim
     … 47 unchanged
     + ~/.vimrc        ← vimrc
     ~ ~/.config/tmux  ← config/tmux

   2 created, 49 unchanged

   # `✓` unchanged, `+` created, `~` replaced, `!` problems
   ```

> [!IMPORTANT]
> On Windows, creating symlinks requires either `Developer Mode` to be enabled or the user who has the permission, `SeCreateSymbolicLinkPrivilege`.
