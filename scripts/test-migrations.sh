#!/usr/bin/env bash
# Applies migrations/*.sql to a throwaway Postgres that mimics a Eurobase tenant,
# then runs the RLS tests in migrations/test/rls_test.sql.
# Needs Docker. Usage: scripts/test-migrations.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
name="laamo-migtest-$$"
image="${PG_IMAGE:-postgres:16-alpine}"

cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$name" -e POSTGRES_PASSWORD=test "$image" >/dev/null
for _ in $(seq 1 30); do
	docker exec "$name" pg_isready -U postgres >/dev/null 2>&1 && break
	sleep 1
done
sleep 1


run() { docker exec -i "$name" psql -v ON_ERROR_STOP=1 -q -U postgres -d postgres "$@"; }

run < "$root/migrations/test/harness.sql"

# Each migration runs in one transaction inside the tenant schema, like Eurobase.
for f in "$root"/migrations/[0-9]*_*.sql; do
	echo "→ $(basename "$f")"
	{ echo "BEGIN; SET LOCAL search_path = tenant_test;"; cat "$f"; echo "COMMIT;"; } | run
done

# Let the API role use the tables, like Eurobase's default privileges.
run -c "GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA tenant_test TO tenant_func;"

{ echo "SET ROLE tenant_func;"; cat "$root/migrations/test/rls_test.sql"; } | run
