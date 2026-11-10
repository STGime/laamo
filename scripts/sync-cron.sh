#!/usr/bin/env bash
# Creates or updates the Eurobase cron jobs listed in migrations/cron.json.
# Safe to re-run. Needs EUROBASE_URL and EUROBASE_PRIVATE_KEY (task loads .env).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
url="${EUROBASE_URL:?EUROBASE_URL not set}"
: "${EUROBASE_PRIVATE_KEY:?EUROBASE_PRIVATE_KEY not set}"

# Pass the key via a header file so it never shows up in the process list.
hdr="$(mktemp)"; trap 'rm -f "$hdr"' EXIT
printf 'apikey: %s\nContent-Type: application/json\n' "$EUROBASE_PRIVATE_KEY" > "$hdr"

api() { # method path [body] → prints body, returns HTTP status in $status
	local out; out="$(mktemp)"
	status=$(curl -sS -o "$out" -w '%{http_code}' -X "$1" -H @"$hdr" "$url/v1/schedules$2" ${3:+--data "$3"})
	cat "$out"; rm -f "$out"
}

count=$(jq length "$root/migrations/cron.json")
for i in $(seq 0 $((count - 1))); do
	job=$(jq -c ".[$i]" "$root/migrations/cron.json")
	name=$(jq -r .name <<<"$job")

	body=$(api POST "" "$job")
	if [[ "$status" == "201" ]]; then
		echo "✓ created $name"
	elif [[ "$status" == "409" ]]; then
		body=$(api PATCH "/$name" "$(jq -c 'del(.name)' <<<"$job")")
		[[ "$status" == 2* ]] || { echo "✗ update $name failed ($status): $body" >&2; exit 1; }
		echo "✓ updated $name"
	else
		echo "✗ create $name failed ($status): $body" >&2
		exit 1
	fi
done
