#!/usr/bin/env bash
# Fails if the web app references server-only secrets.
# The Eurobase secret key must only ever be used by the Go services.
#
# 1. Source check: no server-only variable names in apps/web.
# 2. Bundle check (optional): if EUROBASE_PRIVATE_KEY is set (CI secret),
#    make sure its value does not appear in the built bundle. The value is never printed.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
web="$root/apps/web"
fail=0

forbidden='EUROBASE_PRIVATE_KEY|EUROBASE_SECRET|EUROBASE_MCP_TOKEN|EUROBASE_PAT|BRAVE_SEARCH_API_KEY|BRAVESEARCH_|eb_ptk_'
if grep -rnE "$forbidden" "$web" \
	--exclude-dir=node_modules --exclude-dir=.svelte-kit --exclude-dir=build; then
	echo "✗ Server-only secret referenced in apps/web (see above)." >&2
	fail=1
fi

if [[ -n "${EUROBASE_PRIVATE_KEY:-}" ]]; then
	if [[ ! -d "$web/build" ]]; then
		echo "✗ apps/web/build missing; build the web app before the bundle check." >&2
		fail=1
	elif grep -rqF -- "$EUROBASE_PRIVATE_KEY" "$web/build"; then
		echo "✗ Eurobase secret key value found in the web bundle." >&2
		fail=1
	fi
else
	echo "• EUROBASE_PRIVATE_KEY not set, bundle check skipped."
fi

if [[ $fail -eq 0 ]]; then echo "✓ No server-only secrets in the web app."; fi
exit $fail
