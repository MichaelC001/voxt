#!/usr/bin/env bash
# Builds the native Qwen3-ASR runtime into a self-contained directory.
# Note (Jiaxin Deng): MLX comes from the pinned mlx wheel, the same build the
# Python reference server uses.
set -euo pipefail

output="${1:?usage: build_runtime.sh <output-directory> [<work-directory>]}"
work="${2:-$output/.build}"
native_dir="$(cd "$(dirname "$0")/.." && pwd)"

test "$(uname -m)" = arm64 || { echo "Apple Silicon is required" >&2; exit 1; }
command -v uv >/dev/null || { echo "uv is required" >&2; exit 1; }

# The oldest macOS the runtime runs on (VoxtMini: 15.0). MLX publishes a
# wheel per macOS (14, 15, 26): left alone, uv takes the build host's, and
# on macOS 26 its mlx.metallib (Metal 4) and libraries do not load on
# macOS 15. --python-platform with MACOSX_DEPLOYMENT_TARGET picks the wheel
# for the target (reinstalled, in case an older venv has another), and
# CMake builds the servers for it too.
export MACOSX_DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-15.0}"

mkdir -p "$work"
uv venv -q -p 3.12 --allow-existing "$work/venv"
VIRTUAL_ENV="$work/venv" uv pip install -q --python-platform aarch64-apple-darwin \
  --reinstall-package mlx --reinstall-package mlx-metal \
  -r "$native_dir/scripts/build-tools.lock"
mlx_dir="$("$work/venv/bin/python" -c 'import mlx.core, os; print(os.path.join(os.path.dirname(mlx.core.__file__), "share/cmake/MLX"))')"
export PATH="$work/venv/bin:$PATH"
cmake -S "$native_dir" -B "$work/cmake" -G Ninja -DCMAKE_BUILD_TYPE=Release \
  -DMLX_DIR="$mlx_dir" -DCMAKE_INSTALL_PREFIX="$output" \
  -DCMAKE_OSX_DEPLOYMENT_TARGET="$MACOSX_DEPLOYMENT_TARGET"
cmake --build "$work/cmake"
cmake --install "$work/cmake"
echo "VOXT_OMNI_RUNTIME=$output/bin/qwen3_asr_server"
