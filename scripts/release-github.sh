#!/usr/bin/env bash
# Drafts or refreshes the GitHub release with the notarized installer.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
version="$(jq -r .version mygo.json)"
tag="${1:-v$version}"
[[ "$tag" == "v$version" ]] || { echo "tag $tag does not match VoxtMini $version" >&2; exit 1; }
[[ -n "${GITHUB_TOKEN:-${GH_TOKEN:-}}" ]] || { echo "GH_TOKEN is required" >&2; exit 1; }
command -v gh >/dev/null || { echo "gh is required" >&2; exit 1; }
shopt -s nullglob
images=(build/darwin-arm64/*.dmg)
(( ${#images[@]} > 0 )) || { echo "no VoxtMini installer in build/darwin-arm64" >&2; exit 1; }

notes="$(mktemp)"
trap 'rm -f "$notes"' EXIT
awk -v version="$version" '
  $0 ~ "^## \\[" version "\\]" { found = 1; print; next }
  found && /^## / { exit }
  found { print }
' CHANGELOG.md >"$notes"
[[ -s "$notes" ]] || { echo "CHANGELOG.md has no section for $version" >&2; exit 1; }
min_macos="$(jq -r '.macos.minimumSystemVersion // "15.0"' mygo.json)"
{
  printf '**Download:** %s below (signed and notarized; macOS %s+, Apple silicon), or https://actnow.dev/voxtmini\n\n' "${images[0]##*/}" "$min_macos"
  cat "$notes"
} >"$notes.full"
mv "$notes.full" "$notes"

# Every VoxtMini release, betas included, is a full release marked latest:
# the repository's older v1.x releases are Voxt's, and GitHub shows its
# "latest" release (and /releases/latest) only among non-prereleases, so a
# prerelease would leave Voxt 1.15.0 as the latest. The beta is in the
# title and the version.
title="VoxtMini $version"
if gh release view "$tag" >/dev/null 2>&1; then
  gh release edit "$tag" --draft=true --title "$title" --notes-file "$notes" --prerelease=false --latest
  gh release upload "$tag" "${images[@]}" --clobber
else
  gh release create "$tag" --verify-tag --draft --title "$title" --notes-file "$notes" --latest "${images[@]}"
fi
printf 'Draft GitHub release %s with %d installer(s)\n' "$tag" "${#images[@]}"
