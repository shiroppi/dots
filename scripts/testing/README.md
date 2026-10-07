# Manual-testing sandbox

Scenarios for trying dots by hand, and a script that unpacks one into a temporary directory. Your real home directory is never touched.

```sh
scripts/testing/sandbox.sh --list                 # list scenarios
scripts/testing/sandbox.sh basic                  # unpack basic and open $SHELL in it
scripts/testing/sandbox.sh --keep conflicts       # keep the temporary directory after exit
scripts/testing/sandbox.sh basic -- dots doctor   # run a command instead of opening a shell
SHELL=bash scripts/testing/sandbox.sh conflicts   # open a different shell
```

Without a scenario name, basic is used.

## What sandbox.sh does

1. Builds dots from this checkout (`go build ./cmd/dots`).
2. Creates `${TMPDIR:-/tmp}/dots-testing.XXXXXX`, copies the scenario's `repo/` into `repo/`, and creates an empty `home/`.
3. Points `HOME`, `USERPROFILE`, `LOCALAPPDATA` and `XDG_{CONFIG,DATA,STATE,CACHE}_HOME` at the fake home, unsets `DOTS_REPO`, and puts the freshly built dots first in `PATH`.
4. Runs the scenario's `setup.sh`, if any, to create what git cannot hold (symlinks, FIFOs, items already in the home).
5. Opens `$SHELL` in `repo/` and prints the scenario's `about.txt`. The temporary directory is removed when the shell exits.

Because the shell starts with the fake home, your own shell configuration (such as `~/.config/fish`) is not loaded, and the shell may write its own history or config files into the fake home.

Every run rebuilds dots and starts from a clean state, so after changing the code, just run the script again.

## Scenarios

| Name | What it covers |
| --- | --- |
| basic | Happy path. Four rules producing one target (overrides), every kind of ignore pattern (exact name, `*` as prefix, suffix and in the middle, case sensitivity), os_only, a second auto rule, a name with a space, a hidden item, and a target inside the repository |
| conflicts | The basic repository with existing items in the fake home: a regular file, a directory, a symlink to elsewhere, a broken symlink, an old link from an overridden rule (six conflicts), and one correct link. For the multi-select prompt, backups and restore |
| syntax-error | A TOML syntax error |
| config-errors | Valid TOML that breaks almost every validation rule; checks that all problems are reported at once |
| resolve-errors | Errors while expanding rules: same-priority duplicates (including three auto rules producing one target), parent/child conflicts, `~` and `.` as targets, os_only nested too deep, and bad auto sources (missing, a file, broken symlink, symlink loop, FIFO) |
| plan-errors | Rules resolve, but the disk state stops apply: missing, broken or FIFO sources, a parent that is a file or a symlink, a FIFO at the target, and overlapping source and target |
| empty | An empty directory, to try `dots init` |

Steps to try and the expected results for each scenario are in `scenarios/<name>/about.txt`, which is also printed when the sandbox opens.

The scenarios are written for Linux (WSL included). On macOS the `[dots.darwin]` and other darwin rules apply instead, so some expected results in about.txt differ. `setup.sh` uses `ln -s` and `mkfifo`, so it does not work on Windows.

## Adding a scenario

Create `scenarios/<name>/` with:

| File | Required | Contents |
| --- | --- | --- |
| about.txt | yes | The first line is the summary shown by `--list`, followed by steps to try and expected results |
| repo/ | no | Copied into the repository (including dots.toml) |
| base | no | The name of another scenario on one line. Its repo/ is copied first, then this scenario's repo/ on top |
| setup.sh | no | Run with bash in the repository, with `HOME` and `REPO` set |

dots links nothing when the configuration has any error, so put error cases in their own scenarios instead of adding them to a working one.
