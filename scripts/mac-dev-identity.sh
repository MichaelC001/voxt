#!/usr/bin/env bash
# The Mac's development signing identity (docs/development.md, "Signing
# during development"): a self-signed code signing certificate, "VoxtMini
# Dev", valid for ten years, in a keychain of its own. A build signed with
# it keeps one designated requirement (its certificate, not its cdhash), so
# macOS keeps the Microphone and Accessibility grants across rebuilds and
# installs; an ad hoc build looks like a new app every time.
#
#   scripts/mac-dev-identity.sh                 # create it, once (idempotent)
#   scripts/mac-dev-identity.sh --print         # its SHA-1 (fails if missing)
#   scripts/mac-dev-identity.sh --sign APP      # sign APP and its nested code with it
#   scripts/mac-dev-identity.sh --check APP     # APP's requirements are the identity's
#   scripts/mac.sh scripts/mac-dev-identity.sh  # from Linux
#
# Works over ssh, without the GUI: the keychain
# (~/Library/Keychains/voxtmini-dev.keychain-db) has a random password of
# its own, kept in ~/.config/voxtmini-dev/sign.env (0600), and its key's
# partition list lets codesign use it without asking. The login keychain
# and other keychains are never touched. The keychain joins the user's
# search list only while --sign runs (a locked keychain on the list can
# make other apps ask for its password after a restart), and the list is
# put back afterwards.
#
# --sign is for development builds only: no hardened runtime and no
# timestamp. A self-signed certificate has no Team ID, so the hardened
# runtime's library validation would refuse the app's libraries (SQLite,
# llama.framework, the Omni runtime's). Releases are signed by
# scripts/release.sh with the Developer ID, as before.
#
# Environment (for tests): VOXTMINI_DEV_KEYCHAIN, VOXTMINI_DEV_SIGN_ENV.
set -euo pipefail

NAME="VoxtMini Dev"
KC="${VOXTMINI_DEV_KEYCHAIN:-$HOME/Library/Keychains/voxtmini-dev.keychain-db}"
ENVFILE="${VOXTMINI_DEV_SIGN_ENV:-$HOME/.config/voxtmini-dev/sign.env}"

die() { echo "mac-dev-identity: $*" >&2; exit 1; }
usage() { sed -n '2,/^set -euo/{/^set -euo/d;s/^# \{0,1\}//;p;}' "$0"; }

[[ "$(uname -s)" == Darwin ]] || die "run it on the Mac: scripts/mac.sh scripts/mac-dev-identity.sh"
export PATH="/opt/homebrew/bin:$PATH"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# identity prints the identity's SHA-1, or nothing.
identity() {
  [[ -f "$KC" ]] || return 0
  security find-identity -p codesigning "$KC" 2>/dev/null |
    sed -nE "s/^ *[0-9]+\) ([0-9A-F]{40}) \"$NAME\".*/\1/p" | head -1
}

unlock() {
  [[ -f "$ENVFILE" ]] || die "$KC exists but $ENVFILE, its password, does not: delete the keychain (security delete-keychain $KC) and create it again"
  local VOXTMINI_DEV_KEYCHAIN_PASSWORD=""
  # shellcheck disable=SC1090
  source "$ENVFILE"
  [[ -n "$VOXTMINI_DEV_KEYCHAIN_PASSWORD" ]] || die "$ENVFILE sets no VOXTMINI_DEV_KEYCHAIN_PASSWORD"
  security unlock-keychain -p "$VOXTMINI_DEV_KEYCHAIN_PASSWORD" "$KC"
}

create() {
  if [[ -n "$(identity)" ]]; then
    unlock
    echo "$NAME: $(identity) in $KC (already there)"
    return
  fi
  [[ ! -e "$KC" ]] || die "$KC exists without the identity \"$NAME\": look at it (security find-identity -p codesigning $KC) before deleting it"
  local work pass legacy=()
  work="$(mktemp -d)"
  # shellcheck disable=SC2064
  trap "rm -rf '$work'" EXIT
  umask 077
  pass="$(openssl rand -hex 24)"
  mkdir -p "$(dirname "$ENVFILE")"
  printf 'VOXTMINI_DEV_KEYCHAIN_PASSWORD=%s\n' "$pass" >"$ENVFILE"
  chmod 600 "$ENVFILE"
  openssl req -x509 -newkey rsa:2048 -nodes -days 3650 -subj "/CN=$NAME" \
    -addext "keyUsage=critical,digitalSignature" -addext "extendedKeyUsage=critical,codeSigning" \
    -addext "basicConstraints=critical,CA:false" -keyout "$work/key.pem" -out "$work/cert.pem" 2>/dev/null
  openssl version | grep -qE '^OpenSSL [3-9]' && legacy=(-legacy)
  openssl pkcs12 -export ${legacy[@]+"${legacy[@]}"} -inkey "$work/key.pem" -in "$work/cert.pem" \
    -name "$NAME" -out "$work/id.p12" -passout "pass:$pass"
  security create-keychain -p "$pass" "$KC"
  # No lock on sleep or after a while: unlocked until the next restart.
  security set-keychain-settings "$KC"
  security unlock-keychain -p "$pass" "$KC"
  security import "$work/id.p12" -k "$KC" -P "$pass" -T /usr/bin/codesign >/dev/null
  security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$pass" "$KC" >/dev/null
  [[ -n "$(identity)" ]] || die "the identity is not in $KC after the import"
  echo "$NAME: $(identity) in $KC (created; password in $ENVFILE)"
}

# nested prints the code inside APP to sign before it, deepest first:
# every Mach-O file and every bundle that holds one, as mygo build's
# signNestedCode finds them (the app's own executable is signed with it).
nested() {
  local app="$1" f b machos=() bundles=()
  while IFS= read -r -d '' f; do
    case "$(head -c 4 "$f" | od -An -tx1 | tr -d ' \n')" in
      cffaedfe | cefaedfe | cafebabe | bebafeca) machos+=("$f") ;;
    esac
  done < <(find "$app/Contents" -type f ! -path "$app/Contents/MacOS/*" -print0)
  while IFS= read -r -d '' b; do
    for f in ${machos[@]+"${machos[@]}"}; do
      if [[ "$f" == "$b/"* ]]; then
        bundles+=("$b")
        break
      fi
    done
  done < <(find "$app/Contents" -mindepth 1 -type d \( -name '*.framework' -o -name '*.app' -o -name '*.bundle' \
    -o -name '*.xpc' -o -name '*.appex' -o -name '*.plugin' \) -print0)
  for f in ${machos[@]+"${machos[@]}"} ${bundles[@]+"${bundles[@]}"}; do
    printf '%d\t%s\n' "$(tr -cd / <<<"$f" | wc -c)" "$f"
  done | sort -s -t$'\t' -k1,1nr | cut -f2-
}

SEARCH=()
restore_search() {
  ((${#SEARCH[@]} == 0)) || security list-keychains -d user -s "${SEARCH[@]}"
}

sign() {
  local app="${1:?--sign needs an app}" id f n=0 out
  [[ -d "$app/Contents" ]] || die "$app is not an app bundle"
  id="$(identity)"
  [[ -n "$id" ]] || die "no \"$NAME\" identity in $KC: run scripts/mac-dev-identity.sh first"
  unlock
  read -r -a SEARCH <<<"$(security list-keychains -d user | tr -d '"' | xargs)"
  trap restore_search EXIT
  security list-keychains -d user -s "$KC" "${SEARCH[@]}"
  # Each piece keeps its identifier and entitlements; its flags (the
  # hardened runtime of an ad hoc helper) go.
  local args=(--force --sign "$id" --keychain "$KC" --timestamp=none)
  while IFS= read -r f; do
    out="$(codesign "${args[@]}" --preserve-metadata=identifier,entitlements "$f" 2>&1)" || die "codesign ${f#"$app/"}: $out"
    n=$((n + 1))
  done < <(nested "$app")
  out="$(codesign "${args[@]}" --entitlements "$ROOT/helper/Support/app.entitlements" "$app" 2>&1)" || die "codesign $app: $out"
  echo "signed $app and $n pieces of code inside as $NAME ($id), without the hardened runtime"
}

# check: the app verifies, and the designated requirements of the app, the
# helper and voxtmini-llm name the certificate (an ad hoc one names the
# cdhash, which every build changes), with no hardened runtime.
check() {
  local app="${1:?--check needs an app}" p dr status=0
  codesign --verify --deep --strict "$app" || die "$app does not verify"
  for p in "$app" "$app/Contents/Resources/helper/voxtmini-helper" "$app/Contents/Resources/llm/voxtmini-llm"; do
    [[ -e "$p" ]] || die "no $p"
    dr="$(codesign -d -r- "$p" 2>&1 | sed -n 's/^designated => //p')"
    if [[ "$dr" == *certificate* && "$dr" != *cdhash* ]]; then
      echo "ok   ${p#"$app/"}: $dr"
    else
      echo "FAIL ${p#"$app/"}: designated requirement ${dr:-missing}, not a certificate's" >&2
      status=1
    fi
    if codesign -d -v "$p" 2>&1 | grep -q '^CodeDirectory.*flags=.*runtime'; then
      echo "FAIL ${p#"$app/"}: the hardened runtime is on (library validation refuses self-signed libraries)" >&2
      status=1
    fi
  done
  return "$status"
}

case "${1:-}" in
  "") create ;;
  --print)
    id="$(identity)"
    [[ -n "$id" ]] || die "no \"$NAME\" identity in $KC: run scripts/mac-dev-identity.sh first"
    echo "$id"
    ;;
  --sign) sign "${2:-}" ;;
  --check) check "${2:-}" ;;
  -h | --help) usage ;;
  *) usage >&2; exit 2 ;;
esac
