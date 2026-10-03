<div align="center">
    <img src="docs/icon.png" alt="dots logo" width="256" height="256">
    <h1>dots</h1>
</div>

> A breif, declarative, flexible, cross-platform dotfiles manager.

Your dotfiles, your structure.

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

### Priority

```
dots.[os] > dots > auto.[os] > auto
```

When multiple rules target the same path, the highest-priority rule is used. Rules with the same priority result in an error.

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
