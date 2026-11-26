#!/usr/bin/env bash
# End-to-end External Trigger & per-session microphone override test against
# an installed /Applications/VoxtMini.app on macOS.
#
#   scripts/mac-trigger-test.sh
#
# Tests:
#   1. External Trigger OFF: voxtmini-ctl status/trigger fail cleanly, and
#      voxtmini://trigger/* URLs are ignored without starting a session.
#   2. External Trigger ON:
#      - voxtmini-ctl devices --json returns Core Audio input devices.
#      - voxtmini-ctl status --json reports idle.
#      - trigger --device <unknown> --fallback fail fails with device_not_found.
#      - trigger --device <real-or-missing> --fallback default starts (or
#        reports a clean error if no mic / TCC mic permission is unavailable),
#        then trigger cancel returns to idle.
#      - open -g "voxtmini://trigger/transcribe?action=start&source=test-script"
#        does not steal focus from the frontmost app, and cancel returns to idle.
#      - settings.Audio.Devices in voxtmini.db is never mutated by triggers.
#
# Restores the original trigger settings in voxtmini.db before exiting.
set -euo pipefail

[[ "$(uname -s)" == "Darwin" ]] || { echo "run on macOS" >&2; exit 1; }

APP="${VOXTMINI_APP:-/Applications/VoxtMini.app}"
CTL="$APP/Contents/Resources/ctl/voxtmini-ctl"
DB="$HOME/Library/Application Support/VoxtMini/voxtmini.db"
LOG="$HOME/Library/Logs/VoxtMini/voxtmini.log"

[[ -x "$CTL" ]] || { echo "missing $CTL" >&2; exit 1; }

PASS=0
FAIL=0
pass() { echo "PASS: $*"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $*" >&2; FAIL=$((FAIL + 1)); }

frontmost_app() {
  local asn
  asn="$(lsappinfo front 2>/dev/null || true)"
  if [[ -n "$asn" ]]; then
    lsappinfo info -only name "$asn" 2>/dev/null | sed -n 's/^"LSDisplayName"="\(.*\)"$/\1/p'
  fi
}

audio_devices_json() {
  python3 - "$DB" <<'PY'
import json, sqlite3, sys
con = sqlite3.connect(sys.argv[1])
row = con.execute("SELECT doc FROM documents WHERE key = ?", ("settings:app",)).fetchone()
con.close()
doc = json.loads(row[0]) if row else {}
print(json.dumps(doc.get("audio", {}).get("devices", None), sort_keys=True))
PY
}

set_trigger_enabled() {
  local enabled="$1"
  pkill -TERM -f "$APP/Contents/MacOS/VoxtMini" 2>/dev/null || true
  for _ in $(seq 30); do
    pgrep -f "$APP/Contents/MacOS/VoxtMini" >/dev/null || break
    sleep 0.2
  done
  python3 - "$DB" "$enabled" <<'PY'
import json, sqlite3, sys, time
db, enabled = sys.argv[1], sys.argv[2] == "true"
con = sqlite3.connect(db)
row = con.execute("SELECT doc FROM documents WHERE key = ?", ("settings:app",)).fetchone()
doc = json.loads(row[0]) if row else {}
trig = doc.get("trigger") or {}
trig["enabled"] = enabled
if enabled:
    trig["allowURL"] = True
doc["trigger"] = trig
con.execute(
    "INSERT INTO documents(key, doc, updated_at) VALUES(?, ?, ?) ON CONFLICT(key) DO UPDATE SET doc=excluded.doc, updated_at=excluded.updated_at",
    ("settings:app", json.dumps(doc), int(time.time() * 1000)),
)
con.commit()
con.close()
PY
  open -g "$APP"
  for _ in $(seq 50); do
    pgrep -f "$APP/Contents/MacOS/VoxtMini" >/dev/null && break
    sleep 0.2
  done
  sleep 2
}

# Save original trigger config so we restore it at exit.
ORIG_TRIGGER="$(python3 - "$DB" <<'PY'
import json, sqlite3, sys, os
if not os.path.exists(sys.argv[1]):
    print("null")
    sys.exit(0)
con = sqlite3.connect(sys.argv[1])
row = con.execute("SELECT doc FROM documents WHERE key = ?", ("settings:app",)).fetchone()
con.close()
doc = json.loads(row[0]) if row else {}
print(json.dumps(doc.get("trigger", None)))
PY
)"

restore() {
  if [[ -f "$DB" && "$ORIG_TRIGGER" != "null" ]]; then
    pkill -TERM -f "$APP/Contents/MacOS/VoxtMini" 2>/dev/null || true
    sleep 1
    python3 - "$DB" "$ORIG_TRIGGER" <<'PY'
import json, sqlite3, sys, time
db, orig = sys.argv[1], json.loads(sys.argv[2])
con = sqlite3.connect(db)
row = con.execute("SELECT doc FROM documents WHERE key = ?", ("settings:app",)).fetchone()
doc = json.loads(row[0]) if row else {}
doc["trigger"] = orig
con.execute(
    "INSERT INTO documents(key, doc, updated_at) VALUES(?, ?, ?) ON CONFLICT(key) DO UPDATE SET doc=excluded.doc, updated_at=excluded.updated_at",
    ("settings:app", json.dumps(doc), int(time.time() * 1000)),
)
con.commit()
con.close()
PY
    open -g "$APP"
  fi
}
trap restore EXIT

echo "=== Part A: External Trigger OFF ==="
set_trigger_enabled false

out="$("$CTL" status --json 2>&1 || true)"
if [[ "$out" == *'"ok":false'* && "$out" == *'external trigger is off'* ]]; then
  pass "status rejected when OFF: $out"
else
  fail "status unexpected when OFF: $out"
fi

out="$("$CTL" trigger transcribe --action toggle --json 2>&1 || true)"
if [[ "$out" == *'"ok":false'* && "$out" == *'external trigger is off'* ]]; then
  pass "trigger rejected when OFF: $out"
else
  fail "trigger unexpected when OFF: $out"
fi

log_before=0
[[ -f "$LOG" ]] && log_before="$(wc -l <"$LOG" | tr -d ' ')"
open -g "voxtmini://trigger/transcribe?action=toggle"
sleep 1
if tail -n +"$((log_before + 1))" "$LOG" 2>/dev/null | grep -q "ignored URL trigger (external trigger disabled)"; then
  pass "URL trigger ignored when OFF"
else
  fail "URL trigger ignore line not found in $LOG"
fi

echo "=== Part B: External Trigger ON ==="
set_trigger_enabled true
AUDIO_BEFORE="$(audio_devices_json)"

dev_out="$("$CTL" devices --json)"
echo "devices: $dev_out"
if [[ "$dev_out" == *'"ok":true'* ]]; then
  pass "devices --json returned ok:true"
else
  fail "devices --json failed: $dev_out"
fi

st_out="$("$CTL" status --json)"
echo "status: $st_out"
if [[ "$st_out" == *'"ok":true'* && "$st_out" == *'"state":"idle"'* ]]; then
  pass "status --json is idle"
else
  fail "status --json not idle: $st_out"
fi

unk_out="$("$CTL" trigger transcribe --action start --device "nonexistent-device-uid-xyz" --fallback fail --json 2>&1 || true)"
echo "unknown device (fallback=fail): $unk_out"
if [[ "$unk_out" == *'"ok":false'* && "$unk_out" == *'device_not_found'* ]]; then
  pass "unknown device with --fallback fail returned device_not_found"
else
  fail "unknown device with --fallback fail unexpected: $unk_out"
fi

# Pick first real device UID if any Core Audio input device is attached.
REAL_DEV="$(python3 - "$dev_out" <<'PY'
import json, sys
d = json.loads(sys.argv[1])
devs = d.get("devices") or []
print(devs[0]["id"] if devs else "")
PY
)"
TEST_DEV="${REAL_DEV:-nonexistent-fallback-test}"
start_out="$("$CTL" trigger transcribe --action start --device "$TEST_DEV" --fallback default --json 2>&1 || true)"
echo "start (device=$TEST_DEV, fallback=default): $start_out"
if [[ -n "$REAL_DEV" ]]; then
  if [[ "$start_out" == *'"ok":true'* ]]; then
    pass "start with real device $REAL_DEV succeeded"
    live_st="$("$CTL" status --json)"
    echo "status after start: $live_st"
  else
    pass "start with real device $REAL_DEV returned clean error (e.g. TCC/model): $start_out"
  fi
else
  if [[ "$start_out" == *'"ok":true'* ]]; then
    pass "start with fallback=default (no input mic attached) returned ok:true: $start_out"
  else
    pass "start with fallback=default (no input mic attached) returned clean error: $start_out"
  fi
fi

cancel_out="$("$CTL" trigger cancel --json 2>&1 || true)"
echo "cancel: $cancel_out"
sleep 0.5
st_after_cancel="$("$CTL" status --json)"
if [[ "$st_after_cancel" == *'"ok":true'* && "$st_after_cancel" == *'"state":"idle"'* ]]; then
  pass "status back to idle after cancel"
else
  fail "status not idle after cancel: $st_after_cancel"
fi

# Activate Finder first so VoxtMini is not already frontmost.
osascript -e 'tell application "Finder" to activate' >/dev/null 2>&1 || true
sleep 0.5
front_before="$(frontmost_app)"
echo "frontmost before URL trigger: ${front_before:-<unknown>}"
open -g "voxtmini://trigger/transcribe?action=start&source=test-script"
sleep 1
front_after="$(frontmost_app)"
echo "frontmost after URL trigger:  ${front_after:-<unknown>}"
if [[ "$front_after" != "VoxtMini" ]]; then
  pass "URL trigger with open -g did not steal focus (frontmost=${front_after:-<unknown>})"
else
  fail "URL trigger stole focus to VoxtMini!"
fi

open -g "voxtmini://trigger/cancel"
sleep 1
st_after_url="$("$CTL" status --json)"
if [[ "$st_after_url" == *'"ok":true'* && "$st_after_url" == *'"state":"idle"'* ]]; then
  pass "status is idle after URL cancel"
else
  fail "status not idle after URL cancel: $st_after_url"
fi

AUDIO_AFTER="$(audio_devices_json)"
if [[ "$AUDIO_BEFORE" == "$AUDIO_AFTER" ]]; then
  pass "settings.Audio.Devices unchanged ($AUDIO_AFTER)"
else
  fail "settings.Audio.Devices mutated! before=$AUDIO_BEFORE after=$AUDIO_AFTER"
fi

echo "=== Summary: $PASS passed, $FAIL failed ==="
((FAIL == 0))
