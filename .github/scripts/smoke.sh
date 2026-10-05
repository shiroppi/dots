#!/usr/bin/env bash
# Real-binary smoke test. Usage: smoke.sh <path-to-dots-binary>
# Never touches the real home: HOME/USERPROFILE/LOCALAPPDATA are overridden.
set -euo pipefail

BIN="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
WORK="$(mktemp -d)"
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
run 1 doctor
run 0 apply
run 0 doctor

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

# Idempotency.
run 0 apply
run 0 doctor
check_link "$FAKEHOME/.vimrc"
echo "smoke OK"
