#!/usr/bin/env bash
# Creates or updates the Eurobase cron jobs listed in migrations/cron.json.
# Safe to re-run. Needs EUROBASE_PRIVATE_KEY (task loads .env); EUROBASE_URL defaults to laamo.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
url="${EUROBASE_URL:-https://laamo.eurobase.app}"
: "${EUROBASE_PRIVATE_KEY:?EUROBASE_PRIVATE_KEY not set}"

# Pass the key via a header file so it never shows up in the process list.
hdr="$(mktemp)"; trap 'rm -f "$hdr"' EXIT
printf 'apikey: %s\nContent-Type: application/json\n' "$EUROBASE_PRIVATE_KEY" > "$hdr"

body_file="$(mktemp)"; trap 'rm -f "$hdr" "$body_file"' EXIT

api() { # method path [body] → prints the HTTP status; response body in $body_file
	curl -sS --connect-timeout 10 --max-time 30 -o "$body_file" -w '%{http_code}' \
		-X "$1" -H @"$hdr" "$url/v1/schedules$2" ${3:+--data "$3"}
}

# Jobs removed from cron.json are not deleted on Eurobase; set "enabled": false instead.
# One job per line; an empty list runs no iterations (BSD seq 0 -1 would count down).
while IFS= read -r job; do
	name=$(jq -r .name <<<"$job")

	status=$(api POST "" "$job")
	if [[ "$status" == "201" ]]; then
		echo "✓ created $name"
	elif [[ "$status" == "409" ]]; then
		status=$(api PATCH "/$name" "$(jq -c 'del(.name)' <<<"$job")")
		[[ "$status" == 2* ]] || { echo "✗ update $name failed ($status): $(cat "$body_file")" >&2; exit 1; }
		echo "✓ updated $name"
	else
		echo "✗ create $name failed ($status): $(cat "$body_file")" >&2
		exit 1
	fi
done < <(jq -c '.[]' "$root/migrations/cron.json")
