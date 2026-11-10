# Migrations

SQL migrations for the Laamo Eurobase project, applied with the Eurobase CLI
(`eurobase migrations up`). Files are `<version>_<name>.sql` and apply in order,
each in one transaction inside the project schema. The platform tracks applied
versions and checksums, so **never edit a migration that has been applied**; add a
new one instead.

```sh
task test:migrations   # apply to a throwaway local Postgres + run RLS tests (Docker)
task migrate           # apply pending migrations + sync cron jobs to the laamo Eurobase project
task migrate:status    # local files vs applied versions
```

## Rules for every table

- `id uuid PRIMARY KEY DEFAULT gen_random_uuid()` (Eurobase REST is wired to `id`; `uuid_generate_v4()` is not available).
- `workspace_id` for RLS by membership (`is_workspace_member(workspace_id)`).
- **`user_id uuid NOT NULL`** = workspace owner. Eurobase realtime does not evaluate RLS; it delivers a row to the signed-in user whose id is in the owner column (first of `user_id`/`owner_id`/`created_by`/`uploaded_by`). Rows without an owner (or with NULL) reach only secret-key connections, so the app would get no live updates for them. Service-only tables that end users must never receive (e.g. `jobs`) use the all-zero system id, enforced by a CHECK. The RLS test fails if a table is missing this.
  - Shared team workspaces (post-MVP): other members won't get realtime events for the owner's rows, since the filter knows no workspace column. They must re-read via the API or poll; policy-aware realtime is not scheduled.
- `ENABLE ROW LEVEL SECURITY` plus:
  - `service_all`: `USING (public.is_service_role()) WITH CHECK (public.is_service_role())`. The secret key does **not** bypass RLS by itself.
  - `member_read`: `FOR SELECT USING (is_workspace_member(workspace_id))` for tables the app may read.
  - Tables holding secrets or tokens (`brains`, `places`, `calendar_feeds`, `jobs`) get no end-user policy.
    Realtime can still deliver `brains`/`places` rows to their owner; secrets there are envelope-encrypted with a server-side key, so only ciphertext could reach the client.
- End users only read. All writes go through the Go services with the secret key.
- Use `auth_uid()` (unqualified) for the signed-in user. Only `public.is_service_role()`, `public.current_end_user_id()` may be schema-qualified.
- Not allowed by Eurobase: `SECURITY DEFINER`, `GRANT`/`REVOKE`, `SET search_path`/`SET ROLE`, `CREATE EXTENSION`/`SCHEMA`, `COPY`, transaction control.
- Realtime events fire only for REST writes; rows the UI watches live (`runs`, `run_steps`, `feed_cards`) must be written via REST, not RPC.
- RPC functions return a single value; return `jsonb` instead of `SETOF`.

`test/harness.sql` mimics a Eurobase tenant locally (helper functions, schema, non-owner API role). It is never applied to Eurobase.

## Cron jobs

Eurobase cron jobs are platform config, not SQL, so they live in `cron.json` and are
created or updated by `scripts/sync-cron.sh` (part of `task migrate`). Removing a job from `cron.json` does not delete it on Eurobase; set `"enabled": false` instead. Current jobs:

- `enqueue-due-triggers`: every minute, calls `enqueue_due_triggers()` as the service role.

## Queue functions (0005, fixes in 0006, worker side in 0007)

REST PATCH updates a row by id only, so anything that must check "do I still hold this job" is an RPC.

| Function | Called by | Returns |
| --- | --- | --- |
| `claim_jobs(worker, n)` | worker, ~1/s when idle | jsonb array of claimed jobs (SKIP LOCKED, 5 min lease, reclaims expired leases, fails jobs with no attempts left, never claims past `max_attempts`) |
| `insert_cards(rows)` | worker, per brick | `[{id, card_id}]`; one transaction per call, keep under 1 MB |
| `extend_lease(job_id, worker, attempt, seconds)` | worker, every ~100s while a job runs | `{applied, lease_until}`; only while the worker still holds the job (0007) |
| `finish_job(job_id, worker, attempt, outcome, error, retry_in)` | worker, when a job ends | `{applied, status}`; outcome `done`, `retry` (queued after `retry_in` s, or `failed` when out of attempts) or `failed`. Refused for a worker that lost the lease (0007) |
| `enqueue_due_triggers()` | cron, every minute | `{enqueued, skipped, errored, errors}`; queues `run` jobs and advances `next_run_at`. A trigger whose config or workspace timezone fails is switched off (`next_run_at = NULL`) and listed in `errors`, so it can't block the others |
| `next_schedule_time(config, tz, from_ts)` | API when saving a trigger, and the scheduler | next run strictly after `from_ts`, or NULL |

Schedule trigger config: `{"freq": "daily" | "weekdays" | "weekly", "time": "HH:MM", "days": ["mon", …]}` in the workspace timezone.
