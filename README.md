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

*Requires Go 1.26 or later.*

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

   Each created (`+`) or replaced (`~`) link is shown with its source, relative to the repository root:
   ```
     + ~/.vimrc                  ← vimrc
     ~ ~/.config/tmux            ← config/tmux
     ✓ ~/.config/starship.toml
   ```

### Stale links

`dots apply` records the links it creates in a state file (`$XDG_STATE_HOME/dots/state.json`, `~/.local/state/dots/state.json` by default; `~/Library/Application Support/dots/state.json` on macOS; `%LOCALAPPDATA%\dots\state.json` on Windows). When you remove a rule from `dots.toml`, its link stays in place and becomes *stale*. `dots doctor` lists stale links (and exits with 1), and `dots apply` mentions how many there are, but neither removes them:

```
  ! stale ~/.config/tmux  ← config/tmux
    not in dots.toml; created by dots on 2026-10-01
  ! stale ~/.zshrc
    changed since dots created it; dots will not remove it

Run "dots unlink --stale" to remove stale links.
```

`dots unlink --stale` lists them and asks before removing (`--yes` skips the question, `--dry-run` only shows what would happen). It removes only symbolic links whose link text is still the one dots recorded; a stale path that has since become a regular file, a directory or a different link is left alone and just forgotten, and parent directories are never removed.

Limitations:

* Links created by dots v0.1 and removed from `dots.toml` before any v0.2 `dots apply` are not detected, because nothing recorded them.
* Moving the repository to another path loses the detection of records made before the move.

> [!IMPORTANT]
> On Windows, creating symlinks requires either `Developer Mode` to be enabled or the user who has the `SeCreateSymbolicLinkPrivilege` (usually Administrator).
