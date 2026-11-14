#!/usr/bin/env bash
# Reports the known vulnerabilities reachable from VoxtMini's code
# (govulncheck, the Go vulnerability database) for the release target,
# darwin/arm64, and fails when there is one. Run it before a release
# (docs/release.md) and after changing go.mod's Go version or a module.
# Needs the network (vuln.go.dev); any OS works: govulncheck analyses
# the darwin build without compiling it.
#
#   scripts/vulncheck.sh            # the app and every internal package
#   scripts/vulncheck.sh -show verbose
set -euo pipefail
cd "$(dirname "$0")/.."

# Pinned so the check is the same everywhere; bump it now and then.
GOVULNCHECK=golang.org/x/vuln/cmd/govulncheck@v1.9.0

# Built for this machine, then run on the darwin/arm64 build of the code.
bin="$(mktemp -d)"
trap 'rm -rf "$bin"' EXIT
GOBIN="$bin" go install "$GOVULNCHECK"
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 "$bin/govulncheck" "$@" ./...
