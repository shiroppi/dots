#!/usr/bin/env bash
# TTY E2E for the interactive prompts (Linux/macOS, needs `expect`).
# Usage: tty-e2e.sh <path-to-dots-binary>
# Never touches the real home: HOME/XDG_* are redirected into a sandbox.
#
# The prompt is a bubbletea v2 program: it does not query the terminal, so
# keys can be sent as soon as the prompt text has appeared (issue #4). NO_COLOR
# drops colours; cursor/OSC escapes are stripped from the capture below.
# Keys: "\033\[A" Up, "\033\[B" Down, "\040" Space, "\r" Enter, "\003" Ctrl-C.
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
export XDG_STATE_HOME="$FAKEHOME/.local/state"
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

# a. Conflict + replace: the cursor starts on "No - skip", Up moves to
# "Yes - back up and replace", Enter confirms (the checkbox stays off).
echo "old content" > "$FAKEHOME/.vimrc"
drive '{Replace it} {\033\[A} {} {\r}' apply
expect_rc "apply (conflict, replace)" 0
expect_out "apply (conflict, replace)" "? ~/.vimrc  Yes – back up and replace"
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

# b. Restore over the existing link.
if [ -n "$ARCHIVE" ]; then
  # The cursor starts on "Skip": Up moves to "Back up current content and restore".
  drive '{over it} {\033\[A} {} {\r}' restore "$ARCHIVE"
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

# d. Enter on the initial cursor changes nothing (No - skip).
rm -f "$FAKEHOME/.vimrc"
echo "keep me" > "$FAKEHOME/.vimrc"
drive '{Replace it} {\r}' apply
expect_rc "apply (Enter on default)" 0
expect_out "apply (Enter on default)" "? ~/.vimrc  No – skip"
if [ ! -L "$FAKEHOME/.vimrc" ] && [ "$(cat "$FAKEHOME/.vimrc")" = "keep me" ]; then
  pass "default answer left the file untouched"
else
  fail "default answer changed the file"
fi

# e. Checkbox: Down reaches "Process the rest", Space ticks it, Up twice
# moves to Yes, Enter confirms -> Yes to All: one prompt for two conflicts.
echo "set gui" > "$REPO/gvimrc"
cat >> "$REPO/dots.toml" <<'TOML'
"~/.gvimrc" = "gvimrc"
TOML
rm -f "$FAKEHOME/.vimrc"
echo "old vimrc" > "$FAKEHOME/.vimrc"
echo "old gvimrc" > "$FAKEHOME/.gvimrc"
drive '{Replace it} {\033\[B} {} {\040} {} {\033\[A} {} {\033\[A} {} {\r}' apply
expect_rc "apply (Yes to All)" 0
expect_out "apply (Yes to All)" "[x]"
expect_out "apply (Yes to All)" "(and the rest)"
if [ "$(grep -c 'Replace it' "$OUT")" -eq 1 ]; then pass "asked only once"; else fail "asked $(grep -c 'Replace it' "$OUT") times, want 1"; dump; fi
if [ -L "$FAKEHOME/.vimrc" ] && [ -L "$FAKEHOME/.gvimrc" ]; then
  pass "both conflicts replaced"
else
  fail "not both links were created"; dump
fi

# f. clean: drop the ~/.gvimrc rule (its link was recorded by the
# apply in e). Ctrl-C at the prompt -> exit 130 and nothing is removed; Up
# moves to "Yes - remove them" and Enter removes the link.
cat > "$REPO/dots.toml" <<'TOML'
[dots]
"~/.vimrc" = "vimrc"
TOML
drive '{Remove 1 stale} {\003}' clean
expect_rc "clean (Ctrl-C)" 130
expect_out "clean (Ctrl-C)" "~/.gvimrc"
if [ -L "$FAKEHOME/.gvimrc" ]; then pass "aborted clean left the link"; else fail "aborted clean removed the link"; dump; fi
drive '{Remove 1 stale} {\033\[A} {} {\r}' clean
expect_rc "clean (Yes)" 0
expect_out "clean (Yes)" "? stale links  Yes – remove them"
if [ ! -e "$FAKEHOME/.gvimrc" ] && [ ! -L "$FAKEHOME/.gvimrc" ] && [ -L "$FAKEHOME/.vimrc" ]; then
  pass "stale link removed, current link kept"
else
  fail "unexpected state after clean"; dump
fi

if [ "$FAILED" -ne 0 ]; then echo "tty-e2e FAILED" >&2; exit 1; fi
echo "tty-e2e OK"
