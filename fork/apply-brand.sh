#!/usr/bin/env bash
# Rebrand the checkout as Spark (fork/brand.env) before an image build.
# It edits the working tree only; never commit the result. Every edit checks it
# matched, so an upstream change it no longer fits fails the build loudly.
#
#   fork/apply-brand.sh            # from the repository root
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck disable=SC1091
. fork/brand.env
export APP_NAME THEME_COLOR

if [ -f fork/.applied ]; then
  echo "brand already applied"
  exit 0
fi

# edit <file> <perl substitution using $ENV{...}>: fails when nothing changed.
edit() {
  local file=$1 code=$2
  perl -0pi -e "BEGIN { \$n = 0 } \$n += ($code); END { exit(\$n ? 0 : 3) }" "$file" \
    || { echo "apply-brand: '$code' matched nothing in $file" >&2; exit 1; }
}

# Web app: every user-facing "Silo" names the product. Only the bare word
# changes: hyphenated forms are protocol names (X-Silo-Device-Id) or
# repository names (Silo-Server), identifiers such as SiloBrand do not match,
# and the files generated from the API contract are left alone.
mapfile -t web_files < <(grep -rlw --include='*.ts' --include='*.tsx' 'Silo' web/src \
  | grep -v -e '\.test\.' -e '^web/src/api/v2/schema\.ts$' -e '^web/src/api/v2/operations\.ts$')
[ "${#web_files[@]}" -gt 0 ] || { echo "apply-brand: no web strings found" >&2; exit 1; }
perl -pi -e 's/(?<![-\w])Silo(?![-\w])/$ENV{APP_NAME}/g' "${web_files[@]}"
edit web/index.html 's{<title>Silo</title>}{<title>$ENV{APP_NAME}</title>}'
edit web/public/site.webmanifest 's/"(name|short_name)": "Silo"/"$1": "$ENV{APP_NAME}"/g'
edit web/public/site.webmanifest 's/"(theme_color|background_color)": "#[0-9a-fA-F]{6}"/"$1": "$ENV{THEME_COLOR}"/g'

# Server defaults: the name a fresh server shows, Jellyfin clients see and
# emails come from. A name an administrator saved is kept.
edit internal/config/admin_settings.go 's/("(?:branding\.server_name|jellyfin_compat\.server_name|email\.from_name)":\s*)"Silo"/$1"$ENV{APP_NAME}"/g'
edit internal/config/db_loader.go 's/("jellyfin_compat\.server_name", )"Silo"/$1"$ENV{APP_NAME}"/'
edit internal/config/config.go 's/(ServerName:\s*)"Silo"/$1"$ENV{APP_NAME}"/'
edit internal/mail/mail.go 's/(cfg\.fromName = )"Silo"/$1"$ENV{APP_NAME}"/'
edit internal/invitations/email.go 's/(product = )"Silo"/$1"$ENV{APP_NAME}"/'

# Icons, favicon, mark and sidebar wordmark.
cp -R fork/overlay/. .

touch fork/.applied
echo "applied brand $APP_NAME"
