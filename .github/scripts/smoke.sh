#!/usr/bin/env bash
# Real-binary smoke test. Usage: smoke.sh <path-to-dots-binary>
# Never touches the real home: HOME/USERPROFILE/LOCALAPPDATA are overridden.
# POSIX-friendly: runs on Linux, macOS, Windows (Git Bash), FreeBSD and OpenBSD.
set -euo pipefail

BIN="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
# Explicit template: bare `mktemp -d` is not portable across BSD variants.
WORK="$(mktemp -d "${TMPDIR:-/tmp}/dots-smoke.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

REPO="$WORK/repo"
FAKEHOME="$WORK/home"
mkdir -p "$REPO/config/sub" "$FAKEHOME/AppData/Local"
echo "set nocp" > "$REPO/vimrc"
echo "a=1" > "$REPO/config/sub/a.conf"
cat > "$REPO/dots.toml" <<'TOML'
[dots]
"~/.vimrc" = "vimrc"

[[auto]]
source = "config"
target = "~/.config"
TOML

export HOME="$FAKEHOME"
export USERPROFILE="$FAKEHOME"
export LOCALAPPDATA="$FAKEHOME/AppData/Local"
export XDG_CONFIG_HOME="$FAKEHOME/.config"
export XDG_STATE_HOME="$FAKEHOME/.local/state"
cd "$REPO"

run() { # run <expected-exit> args...
  local want=$1; shift
  local got=0
  "$BIN" "$@" || got=$?
  if [ "$got" -ne "$want" ]; then
    echo "FAIL: dots $* exited $got, want $want" >&2
    exit 1
  fi
  echo "ok: dots $* -> $got"
}

run 0 --version
run 1 status
run 0 apply
run 0 status

# Verify created link is a symlink pointing at a relative path.
check_link() {
  local p=$1
  if [ ! -L "$p" ]; then echo "FAIL: $p is not a symlink" >&2; ls -la "$(dirname "$p")" >&2; exit 1; fi
  local t; t="$(readlink "$p")"
  case "$t" in
    /*|[A-Za-z]:*) echo "FAIL: $p -> $t is not relative" >&2; exit 1 ;;
  esac
  echo "ok: $p -> $t"
}
check_link "$FAKEHOME/.vimrc"
[ "$(cat "$FAKEHOME/.vimrc")" = "set nocp" ] || { echo "FAIL: link content" >&2; exit 1; }
# auto links the first-level item (the directory), not the files inside it.
check_link "$FAKEHOME/.config/sub"
[ "$(cat "$FAKEHOME/.config/sub/a.conf")" = "a=1" ] || { echo "FAIL: auto link content" >&2; exit 1; }

# Idempotency.
run 0 apply
run 0 status
check_link "$FAKEHOME/.vimrc"
check_link "$FAKEHOME/.config/sub"

# Stale links: remove a rule from dots.toml. status reports its link and
# exits 1, clean removes it, and status is clean again.
cat > "$REPO/dots.toml" <<'TOML'
[[auto]]
source = "config"
target = "~/.config"
TOML
check_link "$FAKEHOME/.vimrc"
run 1 status
"$BIN" status > "$WORK/status.txt" || true
grep -q "stale" "$WORK/status.txt" || { echo "FAIL: status does not report the stale link" >&2; cat "$WORK/status.txt" >&2; exit 1; }
run 0 clean --dry-run
[ -L "$FAKEHOME/.vimrc" ] || { echo "FAIL: --dry-run removed the link" >&2; exit 1; }
run 0 clean --no-confirm
[ ! -e "$FAKEHOME/.vimrc" ] && [ ! -L "$FAKEHOME/.vimrc" ] || { echo "FAIL: stale link was not removed" >&2; exit 1; }
[ -f "$REPO/vimrc" ] || { echo "FAIL: source file was removed" >&2; exit 1; }
check_link "$FAKEHOME/.config/sub"
run 0 status
"$BIN" status > "$WORK/status.txt"
if grep -q "stale" "$WORK/status.txt"; then echo "FAIL: status still reports stale links" >&2; cat "$WORK/status.txt" >&2; exit 1; fi
echo "smoke OK"
