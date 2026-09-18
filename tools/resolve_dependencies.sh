#!/usr/bin/env bash
# Bootstrap a missing lockfile on macOS, then enforce the resolved graph.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
LOCK="Voxt.xcodeproj/project.xcworkspace/xcshareddata/swiftpm/Package.resolved"
command -v xcodebuild >/dev/null || { echo "Requires Xcode on macOS" >&2; exit 1; }
xcodebuild -version
xcrun swift --version
OPTIONS=()
if [[ -f "$LOCK" ]]; then
  OPTIONS+=(-onlyUsePackageVersionsFromResolvedFile)
else
  echo "Bootstrapping Package.resolved; review and commit the generated lockfile before release." >&2
fi
xcodebuild -resolvePackageDependencies \
  -project Voxt.xcodeproj -scheme Voxt \
  "${OPTIONS[@]}" "$@"
python3 tools/audit_model_stack.py --resolved "$LOCK"
