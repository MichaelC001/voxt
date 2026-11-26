#!/usr/bin/env bash
# Builds voxtmini-ctl (cmd/voxtmini-ctl, lightweight Go CLI + MCP stdio
# bridge, no UI/MyGo/SQLite) and stages it where mygo ships it:
# resources/darwin-arm64/ctl/voxtmini-ctl ->
# VoxtMini.app/Contents/Resources/ctl/voxtmini-ctl.
#
#   scripts/build-ctl.sh              # stage it for the app (darwin/arm64)
#   scripts/build-ctl.sh --out DIR    # build it into DIR instead
#
# On macOS it is signed with the hardened runtime, ad hoc unless
# VOXTMINI_SIGN_IDENTITY names an identity.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/resources/darwin-arm64/ctl"
if [[ "${1:-}" == "--out" ]]; then
  [[ -n "${2:-}" ]] || { echo "--out needs a directory" >&2; exit 1; }
  OUT="$(mkdir -p "$2" && cd "$2" && pwd)"
fi
IDENTITY="${VOXTMINI_SIGN_IDENTITY:--}"

mkdir -p "$OUT"
cd "$ROOT"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o "$OUT/voxtmini-ctl" ./cmd/voxtmini-ctl

if [[ "$(uname -s)" == "Darwin" ]]; then
  codesign --force --sign "$IDENTITY" --options runtime "$OUT/voxtmini-ctl"
  echo "built $OUT/voxtmini-ctl (darwin/arm64, signed ${IDENTITY/#-/ad hoc})"
else
  echo "built $OUT/voxtmini-ctl (darwin/arm64, unsigned on $(uname -s))"
fi
