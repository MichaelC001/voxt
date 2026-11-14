#!/usr/bin/env bash
# Builds the Omni native runtime (sglang_omni_mlx/native, C++ on MLX) on a
# Mac and stages what VoxtMini runs of it where mygo ships it:
# resources/darwin-arm64/omni/bin/ becomes
# VoxtMini.app/Contents/Resources/omni/bin/ (go tool mygo build, mygo dev):
# qwen3_asr_server (Qwen3-ASR and, with --model-kind silero_vad, Silero
# VAD), whisper_server (Whisper), MLX's libraries and mlx.metallib, which
# MLX loads from beside its library. The build makes the other families'
# servers too; VoxtMini has no model for them (its catalogue keeps only
# models that speak English, Chinese and Japanese), so they stay out.
# mygo build signs the binaries and libraries with the app (they
# keep the hardened runtime of a real identity); the metallib is a
# resource the app's signature seals.
#
#   scripts/build-omni.sh               # build and stage it for the app
#   scripts/build-omni.sh --out DIR     # stage it in DIR instead: a runtime
#                                       # outside the app, for VOXTMINI_OMNI_RUNTIME
#   scripts/build-omni.sh --clean       # delete the build tree afterwards (~1 GB)
#   scripts/mac.sh 'scripts/build-omni.sh && go tool mygo build'
#
# It reuses the pinned sglang_omni_mlx/native/scripts/build_runtime.sh (uv,
# the pinned MLX wheel, CMake, Ninja). Without uv on PATH a private one is
# installed into the build tree (python3 -m venv).
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" || "$(uname -m)" != arm64 ]]; then
  echo "the Omni runtime builds on an Apple silicon Mac: scripts/mac.sh scripts/build-omni.sh" >&2
  exit 1
fi
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/resources/darwin-arm64/omni"
CLEAN=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --out) [[ -n "${2:-}" ]] || { echo "--out needs a directory" >&2; exit 1; }
      OUT="$(mkdir -p "$2" && cd "$2" && pwd)"; shift 2 ;;
    --clean) CLEAN=1; shift ;;
    *) echo "unknown argument $1" >&2; exit 1 ;;
  esac
done

NATIVE="$ROOT/sglang_omni_mlx/native"
[[ -f "$NATIVE/scripts/build_runtime.sh" ]] || {
  echo "the pinned Omni runtime source is missing at $NATIVE" >&2; exit 1; }

WORK="$ROOT/build/omni"
mkdir -p "$WORK"
if ! command -v uv >/dev/null; then
  if [[ ! -x "$WORK/tools/bin/uv" ]]; then
    python3 -m venv "$WORK/tools"
    "$WORK/tools/bin/pip" install -q uv
  fi
  export PATH="$WORK/tools/bin:$PATH"
fi

log="$WORK/build.log"
echo "building the Omni runtime from $NATIVE (log: $log)"
if ! bash "$NATIVE/scripts/build_runtime.sh" "$WORK/runtime" "$WORK/work" >"$log" 2>&1; then
  tail -40 "$log" >&2
  echo "build_runtime.sh failed" >&2
  exit 1
fi

# Only what VoxtMini runs: the servers (not the parity tools), and MLX
# beside them.
SERVERS=(qwen3_asr_server whisper_server)
rm -rf "$OUT/bin"
mkdir -p "$OUT/bin"
for s in "${SERVERS[@]}"; do
  cp -f "$WORK/runtime/bin/$s" "$OUT/bin/"
  chmod 0755 "$OUT/bin/$s"
done
cp -f "$WORK/runtime/bin/"*.dylib "$WORK/runtime/bin/mlx.metallib" "$OUT/bin/"
# Unsigned code does not run on Apple silicon: sign it ad hoc for a runtime
# used outside the app (mygo build signs the app's copy again).
for f in "${SERVERS[@]/#/$OUT/bin/}" "$OUT/bin/"*.dylib; do
  codesign --force --sign - "$f" >/dev/null 2>&1
done
# Each loads (its libraries resolve) when it gets as far as refusing a
# kind ("--model-kind must be …", "only --model-kind whisper is served").
for s in "${SERVERS[@]}"; do
  probe="$("$OUT/bin/$s" --model-kind none 2>&1 || true)"
  if [[ "$probe" != *"--model-kind"* ]]; then
    echo "the staged $s does not start: $probe" >&2
    exit 1
  fi
  du -h "$OUT/bin/$s" | sed "s|^|  |"
done
du -sh "$OUT/bin" | sed "s|^|staged |"
echo "VOXTMINI_OMNI_RUNTIME=$OUT"

if [[ $CLEAN == 1 ]]; then
  rm -rf "$WORK"
  echo "deleted $WORK"
fi
