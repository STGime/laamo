#!/usr/bin/env bash
# Applies migrations/*.sql to a throwaway Postgres that mimics a Eurobase tenant,
# then runs the RLS, queue and concurrency tests (migrations/test/).
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
{ echo "SET ROLE tenant_func;"; cat "$root/migrations/test/queue_test.sql"; } | run
{ echo "SET ROLE tenant_func;"; cat "$root/migrations/test/job_finish_test.sql"; } | run
{ echo "SET ROLE tenant_func;"; cat "$root/migrations/test/workspace_test.sql"; } | run
{ echo "SET ROLE tenant_func;"; cat "$root/migrations/test/brain_test.sql"; } | run

# Concurrency: several workers claim from 300 jobs in parallel; no job may be claimed twice.
run <<'SQL'
SET search_path = tenant_test, public;
SELECT set_config('app.end_user_role', 'service', false) \gset
DELETE FROM jobs;
INSERT INTO jobs (kind) SELECT 'load' FROM generate_series(1, 300);
CREATE TABLE public.claim_log (job_id uuid, worker text);
GRANT INSERT ON public.claim_log TO tenant_func;
SQL
worker() {
	run -q <<SQL
SET ROLE tenant_func;
SET search_path = tenant_test, public;
SELECT set_config('app.end_user_role', 'service', false) \gset
DO \$\$
DECLARE c jsonb;
BEGIN
  LOOP
    c := claim_jobs('$1', 7);
    EXIT WHEN jsonb_array_length(c) = 0;
    INSERT INTO public.claim_log SELECT (e->>'id')::uuid, '$1' FROM jsonb_array_elements(c) e;
    COMMIT;
  END LOOP;
END \$\$;
SQL
}
pids=()
for w in w1 w2 w3 w4 w5 w6; do worker "$w" & pids+=($!); done
for p in "${pids[@]}"; do wait "$p"; done
run -t -A <<'SQL'
DO $$
DECLARE total int; distinct_jobs int; workers int;
BEGIN
  SELECT count(*), count(DISTINCT job_id), count(DISTINCT worker)
    INTO total, distinct_jobs, workers FROM public.claim_log;
  ASSERT total = 300 AND distinct_jobs = 300,
    format('double or missing claims: %s claims, %s distinct jobs', total, distinct_jobs);
  ASSERT workers > 1, 'only one worker got jobs; test did not run concurrently';
  RAISE NOTICE 'Concurrency test passed (% workers, 300 jobs, no double claims)', workers;
END $$;
SQL
