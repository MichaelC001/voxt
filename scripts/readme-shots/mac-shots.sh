#!/usr/bin/env bash
# The README's screenshots on a Mac (app/readme_shots.go): builds VoxtMini
# with -tags readmeshots as a small bundle of its own (another bundle ID,
# a temporary library, family directory and model library, made-up
# history and dictionary, no helper: not the installed VoxtMini), and
# captures its windows by their number, with macOS's own shadow and
# corners (screencapture -l): Home, History, Dictionary, Models, an
# editor window with made-up notes, and the overlay recording, a
# translation's card and a Rewrite conversation; in English and Chinese.
# VOXTMINI_SHOTS_BIN names a binary built elsewhere (a cross-build on
# Linux), so that the Mac only runs it; it stops when memory_pressure
# reports less than VOXTMINI_MIN_FREE percent free (40).
# No pointer, no keys. It takes the front for about half a minute so that
# the windows look active: it starts only when nobody has touched the Mac
# for VOXTMINI_SHOTS_IDLE seconds (300) and gives the front back to the
# app that had it. VOXTMINI_SHOTS_DRY=1 stays in the background (the
# windows look inactive; any time). compose.py puts the captures on the
# wallpaper (scripts/readme-shots/README.md).
#
#   MAC_DIR=code/VoxtMini-shots scripts/mac.sh scripts/readme-shots/mac-shots.sh
#   MAC_DIR=code/VoxtMini-shots scripts/mac.sh --pull build/readme-shots
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "run it on a Mac: MAC_DIR=code/VoxtMini-shots scripts/mac.sh scripts/readme-shots/mac-shots.sh" >&2
  exit 1
fi
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
export GOMAXPROCS=2 GOFLAGS=-p=2

idle() { echo $(( $(ioreg -c IOHIDSystem | awk '/HIDIdleTime/ {print $NF; exit}') / 1000000000 )); }
dry="${VOXTMINI_SHOTS_DRY:-}"
if [[ -z "$dry" ]] && (( $(idle) < ${VOXTMINI_SHOTS_IDLE:-300} )); then
  echo "someone used the Mac $(idle) s ago: not coming to the front (VOXTMINI_SHOTS_IDLE=${VOXTMINI_SHOTS_IDLE:-300})" >&2
  exit 4
fi

free="$(memory_pressure -Q | sed -n 's/.*percentage: \([0-9]*\)%.*/\1/p')"
if (( ${free:-0} < ${VOXTMINI_MIN_FREE:-40} )); then
  echo "less than ${VOXTMINI_MIN_FREE:-40}% of memory is free (${free}%): not starting" >&2
  exit 3
fi

out="$ROOT/build/readme-shots"
id=dev.hehehai.voxtmini.readmeshots
app="$out/VoxtMiniReadmeShots.app"
rm -rf "$out"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
if [[ -n "${VOXTMINI_SHOTS_BIN:-}" ]]; then
  # Built elsewhere (VoxtMini is cgo-free: CGO_ENABLED=0 GOOS=darwin
  # GOARCH=arm64 go build -tags readmeshots ./app on Linux): no build here.
  cp "$VOXTMINI_SHOTS_BIN" "$app/Contents/MacOS/VoxtMiniReadmeShots"
else
  echo "== build"
  nice -n 10 go build -tags readmeshots -o "$app/Contents/MacOS/VoxtMiniReadmeShots" ./app
fi
# The store's SQLite, which mygo build puts among an app's resources: from
# a built one.
for lib in "$HOME"/code/VoxtMini*/build/darwin-arm64/VoxtMini.app/Contents/Resources/libSqlitecore.dylib \
  /Applications/VoxtMini.app/Contents/Resources/libSqlitecore.dylib; do
  [[ -f "$lib" ]] && { cp "$lib" "$app/Contents/Resources/"; break; }
done
cat >"$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleIdentifier</key><string>$id</string>
  <key>CFBundleName</key><string>VoxtMini</string>
  <key>CFBundleExecutable</key><string>VoxtMiniReadmeShots</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>0.1.0</string>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST
codesign -s - --force "$app" >/dev/null 2>&1 || true

# Away from the user's family directory and model library, whatever reads
# the defaults.
scratch="$(mktemp -d /tmp/voxtmini-readme-shots.XXXXXX)"
export ACTNOW_FAMILY_DIR="$scratch/ActNow" HF_HUB_CACHE="$scratch/hub"
unset HF_HOME

front() { lsappinfo info -only bundleid "$(lsappinfo front)" 2>/dev/null | sed 's/.*="\(.*\)"/\1/'; }
before="$(front)"
status=0
for lang in en zh-Hans; do
  shots="$out/$lang"
  mkdir -p "$shots"
  echo "== $lang (in front: $before)"
  extra=(--front 1)
  [[ -n "$dry" ]] && extra=()
  open ${dry:+-g} -n -W --env ACTNOW_FAMILY_DIR="$ACTNOW_FAMILY_DIR" --env HF_HUB_CACHE="$HF_HUB_CACHE" \
    --stdout "$out/run-$lang.log" --stderr "$out/run-$lang.log" \
    "$app" --args --readme-shots "$shots" --lang "$lang" "${extra[@]}" &
  opener=$!
  n=0
  deadline=$((SECONDS + 120))
  while [[ ! -f "$shots/finished" ]] && ((SECONDS < deadline)); do
    if [[ -f "$shots/step-$n.ready" ]]; then
      read -r wid name <"$shots/step-$n.ready"
      # The window with its shadow (macOS's, on a transparent ground).
      screencapture -x -l"$wid" "$shots/$name.png" || status=$?
      touch "$shots/step-$n.done"
      n=$((n + 1))
      continue
    fi
    # The next step may skip a number when its window was missing.
    [[ -f "$shots/step-$((n + 1)).ready" ]] && { n=$((n + 1)); continue; }
    sleep 0.1
  done
  [[ -f "$shots/finished" ]] || { echo "the run did not finish" >&2; cat "$out/run-$lang.log" >&2; status=1; }
  wait "$opener" || true
  rm -f "$shots"/step-*
  [[ -n "$before" && -z "$dry" ]] && open -b "$before" || true
  ((status == 0)) || break
done
# What the run left besides the shots: its bundle, the scratch library,
# MyGo's and macOS's folders of the bundle.
rm -rf "$app" "$scratch" \
  "$HOME/Library/Application Support/VoxtMini README Shots" "$HOME/Library/Caches/VoxtMini README Shots" \
  "$HOME/Library/Logs/VoxtMini README Shots" "$HOME/Library/Saved Application State/$id.savedState" \
  "$HOME/Library/Caches/$id" "$HOME/Library/HTTPStorages/$id" "$HOME/Library/WebKit/$id"
defaults delete "$id" >/dev/null 2>&1 || true
ls -la "$out"/*/
exit $status
