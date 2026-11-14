#!/usr/bin/env bash
# Formatting and dead code checks: gofmt clean, nothing unused (staticcheck's U1000) and no function
# unreachable even from the tests (deadcode -test). Both must print
# nothing; CI runs this (.github/workflows/ci.yml). The tools are pinned
# here, run with go run (not in go.mod); the first run downloads them.
#
#   scripts/lint.sh
#
# Code only the tests reach is allowed (deadcode without -test lists it:
# the deliberate seams, permission.Fake, settings.NewMemory, the
# protocols' constructors for the fixture tests…); move the rest into
# _test.go files rather than exporting it for tests.
set -euo pipefail
cd "$(dirname "$0")/.."
export CGO_ENABLED="${CGO_ENABLED:-0}"

staticcheck="honnef.co/go/tools/cmd/staticcheck@v0.8.2" # 2026.2.2
deadcode="golang.org/x/tools/cmd/deadcode@v0.52.0"

status=0
echo "== gofmt"
unformatted="$(gofmt -l $(git ls-files '*.go' | grep -v -e '^third_party/' -e '^sglang_omni_mlx/'))"
if [ -n "$unformatted" ]; then
	echo "$unformatted"
	status=1
fi
echo "== staticcheck -checks U1000"
go run "$staticcheck" -checks U1000 ./... || status=1
echo "== deadcode -test"
out="$(go run "$deadcode" -test ./...)" || status=1
if [ -n "$out" ]; then
	echo "$out"
	status=1
fi
[ "$status" -eq 0 ] && echo "lint: clean"
exit "$status"
