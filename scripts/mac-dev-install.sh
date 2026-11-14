#!/usr/bin/env bash
# A development build of VoxtMini installed into /Applications on the Mac,
# signed with the Mac's development identity (scripts/mac-dev-identity.sh)
# so macOS keeps its permissions from one install to the next
# (docs/development.md, "Signing during development").
#
#   scripts/mac-dev-install.sh [--runtimes DIR] [--skip-tests] [--dry-run]
#   MAC_DIR=code/VoxtMini-test scripts/mac.sh scripts/mac-dev-install.sh
#
# Run from a checkout or a git archive of the project (a file .rev there
# names its commit). In order: the helper's Swift tests, the helper built,
# the language and speech runtimes copied from DIR (default
# ~/voxtmini-runtime: DIR/llm, DIR/omni/bin; APFS clones), go tool mygo
# build (ad hoc, no disk image), the app signed again with the identity
# without the hardened runtime, its signature checked (designated
# requirements of the app, the helper and voxtmini-llm name the
# certificate, not a cdhash), then a staged install: a copy to
# /Applications/.VoxtMini.new.app, a running VoxtMini asked to quit
# (SIGTERM), the copy swapped in. It never opens the app. The build trees
# go afterwards, whatever happens.
#
# Output: "rev <commit>" first, INSTALL_DONE or BUILD_FAILED last.
# VOXTMINI_MIN_FREE (default 30): the least memory free, in percent
# (memory_pressure), to start with; 0 skips the check.
# --dry-run prints the steps without running them (on Linux too).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
RUNTIMES="$HOME/voxtmini-runtime" TESTS=1 DRY=0
while (($#)); do
  case "$1" in
    --runtimes) RUNTIMES="${2:?--runtimes needs a directory}"; shift 2 ;;
    --skip-tests) TESTS=0; shift ;;
    --dry-run) DRY=1; shift ;;
    -h | --help) sed -n '2,/^set -euo/{/^set -euo/d;s/^# \{0,1\}//;p;}' "$0"; exit 0 ;;
    *) echo "mac-dev-install: unknown argument $1 (see --help)" >&2; exit 2 ;;
  esac
done

APPS="${VOXTMINI_APPLICATIONS:-/Applications}"
TARGET="$APPS/VoxtMini.app"
NEW="$APPS/.VoxtMini.new.app"
OLD="$APPS/.VoxtMini.old.app"
BUILT="build/darwin-arm64/VoxtMini.app"
RES="resources/darwin-arm64"

if [[ -f .rev ]]; then
  rev="$(tr -d '[:space:]' <.rev)"
else
  rev="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
fi
echo "rev $rev"

run() {
  if ((DRY)); then
    printf '+'
    printf ' %q' "$@"
    printf '\n'
  else
    "$@"
  fi
}

cleanup() {
  run rm -rf build "$RES/llm" "$RES/omni" helper/.build "$NEW" "$OLD"
}
finish() {
  local status=$?
  trap - EXIT
  cleanup 2>/dev/null || true
  if ((status == 0)); then
    echo INSTALL_DONE
  else
    echo BUILD_FAILED
  fi
  exit "$status"
}
trap finish EXIT

if ((DRY == 0)); then
  [[ "$(uname -s)" == Darwin && "$(uname -m)" == arm64 ]] || { echo "run it on the Mac (Apple silicon), or with --dry-run" >&2; exit 1; }
  export PATH="/opt/homebrew/bin:$PATH"
  min="${VOXTMINI_MIN_FREE:-30}"
  if ((min > 0)); then
    free="$(memory_pressure 2>/dev/null | sed -n 's/^System-wide memory free percentage: \([0-9]*\)%.*/\1/p')"
    sysctl vm.swapusage
    if [[ -n "$free" ]] && ((free < min)); then
      echo "only $free% memory free (VOXTMINI_MIN_FREE=$min): try later" >&2
      exit 1
    fi
  fi
  for d in "$RUNTIMES/llm" "$RUNTIMES/omni/bin"; do
    [[ -d "$d" ]] || { echo "no $d (--runtimes)" >&2; exit 1; }
  done
fi

echo "== signing identity"
run scripts/mac-dev-identity.sh --print

if ((TESTS)); then
  echo "== helper tests"
  (cd helper && run swift test)
fi

echo "== runtimes from $RUNTIMES"
run rm -rf "$RES/llm" "$RES/omni"
run mkdir -p "$RES/omni"
run cp -Rc "$RUNTIMES/llm" "$RES/llm"
run cp -Rc "$RUNTIMES/omni/bin" "$RES/omni/bin"

echo "== helper"
run scripts/build-helper.sh

echo "== app"
run go tool mygo build -platform darwin/arm64 -skip-dmg

echo "== signing"
run scripts/mac-dev-identity.sh --sign "$BUILT"
run scripts/mac-dev-identity.sh --check "$BUILT"

echo "== install into $TARGET"
run rm -rf "$NEW" "$OLD"
run ditto "$BUILT" "$NEW"
if ((DRY)); then
  echo "+ pkill -TERM -f '^$TARGET/Contents/MacOS/' and wait up to 20 s"
elif pkill -TERM -f "^$TARGET/Contents/MacOS/"; then
  echo "asked the running VoxtMini to quit"
  for _ in $(seq 40); do
    pgrep -f "^$TARGET/Contents/MacOS/" >/dev/null || break
    sleep 0.5
  done
  if pgrep -f "^$TARGET/Contents/MacOS/" >/dev/null; then
    echo "warning: VoxtMini is still running; it keeps the old files until it quits" >&2
  fi
fi
if ((DRY)) || [[ -e "$TARGET" ]]; then
  run mv "$TARGET" "$OLD"
fi
run mv "$NEW" "$TARGET"
echo "installed $TARGET ($rev); not opened"
