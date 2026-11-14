#!/usr/bin/env bash
# Runs the root package's tests in N processes at once, split by the last
# run's durations (internal/tools/testshard; build/test-times.json): its
# end-to-end tests wait in real time and run one after another, so one
# process takes about a minute where eight take about ten seconds. The
# other packages are quick, and go test's cache already skips the ones
# that did not change: test them with `go test -p 8 ./internal/...`.
#
#   scripts/test-main.sh             8 processes, every test
#   scripts/test-main.sh -n 4        4 processes (mac-air: memory)
#   scripts/test-main.sh -run 'TestSession|TestOverlay' -v
#   scripts/test-main.sh -race       built with -race (cgo)
#
# Exit status: 0 all passed, 1 a test failed or did not run, 2 the build.
set -euo pipefail
cd "$(dirname "$0")/.."
export CGO_ENABLED="${CGO_ENABLED:-0}"
mkdir -p build
go build -o build/testshard ./internal/tools/testshard
exec build/testshard "$@" .
