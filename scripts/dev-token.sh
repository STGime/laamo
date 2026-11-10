#!/usr/bin/env bash
# Prints a Eurobase access token for a Laamo test user, for calling the API:
#
#   TOKEN=$(scripts/dev-token.sh you@example.com)
#   curl -H "Authorization: Bearer $TOKEN" localhost:8787/api/me
#
# --signup creates the user first. The password comes from LAAMO_TEST_PASSWORD
# or is asked for (not echoed). Uses the public key from .env; tokens last 1h.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
if [[ -f "$root/.env" ]]; then set -a; . "$root/.env"; set +a; fi
url="${EUROBASE_URL:-https://laamo.eurobase.app}"
: "${EUROBASE_PUBLIC_KEY:?EUROBASE_PUBLIC_KEY not set (see .env.example)}"

action=signin
if [[ "${1:-}" == "--signup" ]]; then action=signup; shift; fi
email="${1:?usage: scripts/dev-token.sh [--signup] email}"
password="${LAAMO_TEST_PASSWORD:-}"
if [[ -z "$password" ]]; then
	read -r -s -p "Password for $email: " password </dev/tty
	echo >&2
fi

body=$(jq -n --arg e "$email" --arg p "$password" '{email: $e, password: $p}')
resp=$(curl -sS --max-time 15 -X POST "$url/v1/auth/$action" \
	-H "apikey: $EUROBASE_PUBLIC_KEY" -H "Content-Type: application/json" --data "$body")

token=$(jq -r '.access_token // empty' <<<"$resp")
if [[ -z "$token" ]]; then
	if [[ "$(jq -r '.confirmation_required // false' <<<"$resp")" == "true" ]]; then
		echo "Check $email for the confirmation link, then run without --signup." >&2
	else
		echo "Sign-in failed: $(jq -r '.error // .' <<<"$resp")" >&2
	fi
	exit 1
fi
echo "$token"
