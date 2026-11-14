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

title="VoxtMini $version"
prerelease=()
[[ "$version" == *-* ]] && prerelease=(--prerelease)
if gh release view "$tag" >/dev/null 2>&1; then
  gh release edit "$tag" --draft=true --title "$title" --notes-file "$notes" "${prerelease[@]}"
  gh release upload "$tag" "${images[@]}" --clobber
else
  gh release create "$tag" --verify-tag --draft --title "$title" --notes-file "$notes" "${prerelease[@]}" "${images[@]}"
fi
printf 'Draft GitHub release %s with %d installer(s)\n' "$tag" "${#images[@]}"
