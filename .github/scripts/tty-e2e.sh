#!/usr/bin/env bash
# TTY E2E for the interactive prompts (Linux/macOS, needs `expect`).
# Usage: tty-e2e.sh <path-to-dots-binary>
# Never touches the real home: HOME/XDG_* are redirected into a sandbox.
#
# TERM must not be "dumb": huh switches to its accessible (numbered) prompt
# there. With a real TERM bubbletea sends OSC 11 / CPR queries that expect's
# pty never answers (issue #4); termenv gives up after a few seconds, so keys
# are only sent once the prompt text has appeared. NO_COLOR keeps the TUI but
# drops colours; cursor/OSC escapes are stripped from the capture below.
set -euo pipefail

BIN="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
HERE="$(cd "$(dirname "$0")" && pwd)"
EXP="$HERE/tty-e2e.exp"
command -v expect >/dev/null || { echo "FAIL: expect is not installed" >&2; exit 1; }

WORK="$(mktemp -d "${TMPDIR:-/tmp}/dots-tty.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

REPO="$WORK/repo"
FAKEHOME="$WORK/home"
mkdir -p "$REPO" "$FAKEHOME"
echo "set nocp" > "$REPO/vimrc"
cat > "$REPO/dots.toml" <<'TOML'
[dots]
"~/.vimrc" = "vimrc"
TOML

export HOME="$FAKEHOME"
export XDG_CONFIG_HOME="$FAKEHOME/.config"
export XDG_DATA_HOME="$FAKEHOME/.local/share"
export TERM="${TTY_E2E_TERM:-xterm-256color}"
export NO_COLOR=1
case "$(uname -s)" in
  Darwin) BACKUPS="$FAKEHOME/Library/Application Support/dots/backups" ;;
  *)      BACKUPS="$XDG_DATA_HOME/dots/backups" ;;
esac
cd "$REPO"

OUT="$WORK/out.txt"
FAILED=0

fail() { echo "FAIL: $*" >&2; FAILED=1; }
pass() { echo "ok: $*"; }

# drive <steps> <dots args...>: run dots under expect; sets $RC and $OUT
# (CRs removed). Prints the output on a nonzero expect failure (99/98).
drive() {
  local steps=$1; shift
  RC=0
  expect -f "$EXP" "$steps" "$BIN" "$@" > "$OUT.raw" 2>&1 || RC=$?
  # Strip CR, OSC (ESC ] ... BEL|ESC \), CSI (ESC [ ... final) and other ESC
  # sequences. perl is used because sed differs between GNU and BSD.
  perl -pe 's/\r//g; s/\e\][^\a\e]*(?:\a|\e\\)//g; s/\e\[[0-9;?<>=!]*[ -\/]*[@-~]//g; s/\e[@-_]//g' "$OUT.raw" > "$OUT"
}
dump() { echo "----- captured output -----" >&2; cat "$OUT" >&2; echo "---------------------------" >&2; }
expect_rc() { # expect_rc <label> <want>
  if [ "$RC" -eq "$2" ]; then pass "$1 exited $RC"; else fail "$1 exited $RC, want $2"; dump; fi
}
expect_out() { # expect_out <label> <fixed string>
  if grep -qF -- "$2" "$OUT"; then pass "$1 output has '$2'"; else fail "$1 output lacks '$2'"; dump; fi
}

# a. Conflict + replace: Enter selects "Yes - back up and replace", the
# second Enter submits the form (the "process the rest" box stays unchecked).
echo "old content" > "$FAKEHOME/.vimrc"
drive '{Replace it} {\r} {Process the} {\r}' apply
expect_rc "apply (conflict, replace)" 0
if [ -L "$FAKEHOME/.vimrc" ]; then
  t="$(readlink "$FAKEHOME/.vimrc")"
  case "$t" in
    /*) fail "link target $t is not relative" ;;
    *)  pass ".vimrc -> $t" ;;
  esac
else
  fail ".vimrc is not a symlink"; dump
fi
ARCHIVE="$(ls "$BACKUPS"/*.tar.gz 2>/dev/null | head -n 1 || true)"
if [ -n "$ARCHIVE" ]; then pass "backup archive: $ARCHIVE"; else fail "no backup archive in $BACKUPS"; dump; fi
if grep -qE '~ +~/\.vimrc' "$OUT"; then pass "output has the replace (~) line"; else fail "output lacks the replace (~) line"; dump; fi

# b. Restore over the existing link: first choice is "Back up current
# content and restore".
if [ -n "$ARCHIVE" ]; then
  drive '{over it} {\r}' restore "$ARCHIVE"
  expect_rc "restore" 0
  if [ ! -L "$FAKEHOME/.vimrc" ] && [ "$(cat "$FAKEHOME/.vimrc" 2>/dev/null)" = "old content" ]; then
    pass "restored content is back"
  else
    fail "restored content is wrong"; ls -la "$FAKEHOME" >&2; dump
  fi
fi

# c. Abort: Ctrl-C at the conflict prompt -> exit 130, nothing processed.
rm -f "$FAKEHOME/.vimrc"
echo "keep me" > "$FAKEHOME/.vimrc"
drive '{Replace it} {\003}' apply
expect_rc "apply (Ctrl-C)" 130
expect_out "apply (Ctrl-C)" "not processed"
if [ ! -L "$FAKEHOME/.vimrc" ] && [ "$(cat "$FAKEHOME/.vimrc")" = "keep me" ]; then
  pass "aborted apply left the file untouched"
else
  fail "aborted apply changed the file"
fi

if [ "$FAILED" -ne 0 ]; then echo "tty-e2e FAILED" >&2; exit 1; fi
echo "tty-e2e OK"
