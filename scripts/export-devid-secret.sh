#!/usr/bin/env bash
# Puts the Developer ID Application certificate that signs releases into
# the repository secrets MACOS_CERTIFICATE (base64 .p12) and
# MACOS_CERTIFICATE_PASSWORD, which .github/workflows/release.yml reads
# (docs/release.md, "GitHub Actions setup"). Run it on the Mac that holds
# the certificate, in Terminal (not over ssh: the export needs the login
# keychain and asks for its password):
#
#   bash scripts/export-devid-secret.sh [TEAMID] [OWNER/REPO]
#
# TEAMID defaults to 779M533DL5, OWNER/REPO to hehehai/VoxtMini.
#
# 1. It lists the team's Developer ID Application certificates with their
#    expiry dates and opens Keychain Access.
# 2. You export the newest one (right click → Export → .p12, a password of
#    your choice) to ~/Desktop/devid.p12 and press Return here.
# 3. It asks for that password, checks that the file holds a Developer ID
#    Application certificate of the team with its private key, and sets
#    both secrets with gh. Without a working `gh auth`, it leaves them in
#    ~/.config/voxtmini-release/ (mode 600) for someone with access to set,
#    who must then delete them.
# 4. The .p12 is deleted in every case.
set -euo pipefail
team="${1:-779M533DL5}"
repo="${2:-hehehai/VoxtMini}"
p12="$HOME/Desktop/devid.p12"
export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"

[[ "$(uname -s)" == Darwin ]] || { echo "run it on the Mac" >&2; exit 1; }
command -v openssl >/dev/null || { echo "openssl is missing (brew install openssl)" >&2; exit 1; }
trap 'rm -P "$p12" 2>/dev/null || rm -f "$p12"' EXIT

echo "Developer ID Application certificates of team $team in your keychains:"
found=0
while IFS= read -r -d $'\0' pem; do
  subject="$(printf '%s\n' "$pem" | openssl x509 -noout -subject 2>/dev/null || true)"
  [[ "$subject" == *"Developer ID Application"* && "$subject" == *"$team"* ]] || continue
  end="$(printf '%s\n' "$pem" | openssl x509 -noout -enddate | cut -d= -f2)"
  echo "  - expires $end"
  found=1
done < <(security find-certificate -a -c "Developer ID Application" -p 2>/dev/null |
  perl -0777 -ne 'print "$1\n\0" while /(-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----)/sg')
((found)) || { echo "  none: is the certificate in this Mac's login keychain?" >&2; exit 1; }

cat <<EOF

In Keychain Access (opening now): login keychain → My Certificates →
"Developer ID Application: … ($team)". If there are several, pick the one
that expires last. Right click → Export → File Format "Personal Information
Exchange (.p12)" → save as:  $p12
Choose a password for the file when asked; macOS then asks for your login
password to allow the export.
EOF
open -a "Keychain Access" || true
read -r -p "Press Return once $p12 is saved… "
[[ -s "$p12" ]] || { echo "no $p12" >&2; exit 1; }
read -r -s -p "The .p12's password: " pass
echo

# macOS writes .p12 files with algorithms OpenSSL 3 calls legacy.
info="$(openssl pkcs12 -in "$p12" -passin "pass:$pass" -nokeys -clcerts 2>/dev/null ||
  openssl pkcs12 -legacy -in "$p12" -passin "pass:$pass" -nokeys -clcerts 2>/dev/null || true)"
[[ -n "$info" ]] || { echo "cannot open the .p12 (wrong password?)" >&2; exit 1; }
subject="$(printf '%s\n' "$info" | openssl x509 -noout -subject)"
[[ "$subject" == *"Developer ID Application"* && "$subject" == *"$team"* ]] ||
  { echo "the .p12 holds $subject, not a Developer ID Application of $team" >&2; exit 1; }
keys="$(openssl pkcs12 -in "$p12" -passin "pass:$pass" -nocerts -nodes 2>/dev/null ||
  openssl pkcs12 -legacy -in "$p12" -passin "pass:$pass" -nocerts -nodes 2>/dev/null || true)"
[[ "$keys" == *"PRIVATE KEY"* ]] || { echo "the .p12 has no private key: export the identity, not only the certificate" >&2; exit 1; }
unset keys
echo "ok: $subject, with its private key"

if command -v gh >/dev/null && gh auth status >/dev/null 2>&1; then
  base64 -i "$p12" | tr -d '\n' | gh secret set MACOS_CERTIFICATE -R "$repo"
  printf '%s' "$pass" | gh secret set MACOS_CERTIFICATE_PASSWORD -R "$repo"
  echo "set MACOS_CERTIFICATE and MACOS_CERTIFICATE_PASSWORD on $repo"
else
  dir="$HOME/.config/voxtmini-release"
  mkdir -p "$dir" && chmod 700 "$dir"
  (umask 077 && base64 -i "$p12" | tr -d '\n' >"$dir/macos-certificate.b64" && printf '%s' "$pass" >"$dir/macos-certificate.pass")
  echo "gh is not logged in here: left the two values in $dir (mode 600)."
  echo "Whoever sets the secrets must delete that folder afterwards."
fi
unset pass
