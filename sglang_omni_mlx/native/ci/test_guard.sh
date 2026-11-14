#!/usr/bin/env bash
# Builds tools/guard_server (common/guard on the pinned civetweb, no MLX) with
# the system's compilers and runs ci/test_guard.py against it. Runs on Linux
# and macOS.
#
#   ci/test_guard.sh [<work-directory>]    # default: a temporary directory
#   CIVETWEB_SRC=<checkout> ci/test_guard.sh   # a civetweb checkout to use
set -euo pipefail

native="$(cd "$(dirname "$0")/.." && pwd)"
work="${1:-$(mktemp -d)}"
mkdir -p "$work"
commit="$(sed -n 's/^ *GIT_TAG \([0-9a-f]\{40\}\) # v1\.16.*/\1/p' "$native/CMakeLists.txt")"
[[ -n "$commit" ]] || { echo "no civetweb commit in CMakeLists.txt" >&2; exit 1; }
src="${CIVETWEB_SRC:-$work/civetweb}"
if [[ ! -d "$src/.git" ]]; then
  git clone -q https://github.com/civetweb/civetweb.git "$src"
fi
git -C "$src" -c advice.detachedHead=false checkout -q "$commit"

cc -O1 -c "$src/src/civetweb.c" -I"$src/include" -DUSE_WEBSOCKET -DNO_SSL \
  -o "$work/civetweb.o"
c++ -std=c++20 -O1 -Wall -Wextra -I"$native/src" -I"$src/include" \
  "$native/src/common/guard.cpp" "$native/tools/guard_server.cpp" \
  "$work/civetweb.o" -lpthread -ldl -o "$work/guard_server" 2>&1 |
  { grep -v '^$' || true; }
[[ -x "$work/guard_server" ]]
python3 -I "$native/ci/test_guard.py" "$work/guard_server"
