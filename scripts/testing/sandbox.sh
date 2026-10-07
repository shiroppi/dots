#!/usr/bin/env bash
# Manual-testing sandbox for dots.
#
# Usage:
#   scripts/testing/sandbox.sh [--keep] [<scenario>] [-- <command>...]
#   scripts/testing/sandbox.sh --list
#
# Builds dots from this checkout, copies a scenario into a temporary
# directory, points HOME (and the XDG/Windows variables dots reads) at a fake
# home inside it, and opens $SHELL in the scenario's repository. With
# "-- <command>...", runs that command there instead of a shell.
#
# Your real home is never touched. The temporary directory is removed when the
# shell or command exits unless --keep is given.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
SCENARIOS="$HERE/scenarios"

list() {
  for d in "$SCENARIOS"/*/; do
    name="$(basename "$d")"
    printf '  %-16s %s\n' "$name" "$(head -n 1 "$d/about.txt")"
  done
}

keep=0
scenario=basic
while [ $# -gt 0 ]; do
  case "$1" in
    --list) echo "Scenarios:"; list; exit 0 ;;
    --keep) keep=1; shift ;;
    -h|--help) sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    --) shift; break ;;
    -*) echo "sandbox: unknown option $1" >&2; exit 2 ;;
    *) scenario="$1"; shift ;;
  esac
done

T="$SCENARIOS/$scenario"
if [ ! -f "$T/about.txt" ]; then
  echo "sandbox: unknown scenario \"$scenario\"" >&2
  echo "Scenarios:" >&2; list >&2
  exit 2
fi

WORK="$(mktemp -d "${TMPDIR:-/tmp}/dots-testing.XXXXXX")"
if [ "$keep" -eq 0 ]; then
  trap 'rm -rf "$WORK"' EXIT
fi
export REPO="$WORK/repo"
FAKEHOME="$WORK/home"
mkdir -p "$WORK/bin" "$REPO" "$FAKEHOME"

(cd "$ROOT" && go build -o "$WORK/bin/dots" ./cmd/dots)

# A scenario may reuse another scenario's repository ("base" names it) and
# add or override files with its own repo/.
if [ -f "$T/base" ]; then
  cp -R "$SCENARIOS/$(cat "$T/base")/repo/." "$REPO/"
fi
if [ -d "$T/repo" ]; then
  cp -R "$T/repo/." "$REPO/"
fi

export HOME="$FAKEHOME"
export USERPROFILE="$FAKEHOME"
export LOCALAPPDATA="$FAKEHOME/AppData/Local"
export XDG_CONFIG_HOME="$FAKEHOME/.config"
export XDG_DATA_HOME="$FAKEHOME/.local/share"
export XDG_STATE_HOME="$FAKEHOME/.local/state"
export XDG_CACHE_HOME="$FAKEHOME/.cache"
export PATH="$WORK/bin:$PATH"
export DOTS_SANDBOX="$scenario"
unset DOTS_REPO

# setup.sh prepares what plain files in git cannot hold: symlinks, FIFOs and
# pre-existing items in the fake home. It runs with HOME and REPO set.
if [ -f "$T/setup.sh" ]; then
  (cd "$REPO" && bash "$T/setup.sh")
fi

cd "$REPO"
if [ $# -gt 0 ]; then
  "$@"
  exit
fi

cat <<EOF
dots sandbox: $scenario
  repo: $REPO
  home: $HOME
  dots: $WORK/bin/dots
$( [ "$keep" -eq 1 ] && echo "  (kept after exit: rm -rf $WORK)" || echo "  (removed on exit; use --keep to inspect it afterwards)" )

EOF
cat "$T/about.txt"
echo
echo "Type \"exit\" to leave the sandbox."
"${SHELL:-bash}" || true
