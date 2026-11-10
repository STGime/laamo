-- Tests for the queue and scheduler functions (0005). Runs as tenant_func after rls_test.sql.
\set ON_ERROR_STOP on
\o /dev/null
SET search_path = tenant_test, public;

CREATE FUNCTION pg_temp.act(role text, uid uuid) RETURNS void LANGUAGE sql AS $$
  SELECT set_config('app.end_user_role', role, false),
         set_config('app.end_user_id', coalesce(uid::text, ''), false);
$$;

SELECT pg_temp.act('service', NULL);
DELETE FROM jobs;

-- ---- next_schedule_time ----------------------------------------------------
DO $$
DECLARE r timestamptz;
BEGIN
  -- daily 08:00 Berlin, from Tue 2026-11-10 06:00 UTC (07:00 local) → same day 07:00 UTC
  r := next_schedule_time('{"freq":"daily","time":"08:00"}', 'Europe/Berlin', '2026-11-10 06:00+00');
  ASSERT r = '2026-11-10 07:00+00', 'daily same day: ' || r;
  -- exactly at the slot → next day (strictly after)
  r := next_schedule_time('{"freq":"daily","time":"08:00"}', 'Europe/Berlin', '2026-11-10 07:00+00');
  ASSERT r = '2026-11-11 07:00+00', 'daily strictly after: ' || r;
  -- weekdays from Fri evening → Monday
  r := next_schedule_time('{"freq":"weekdays","time":"07:30"}', 'Europe/Berlin', '2026-11-13 18:00+00');
  ASSERT r = '2026-11-16 06:30+00', 'weekdays skips weekend: ' || r;
  -- weekly Mon 08:00 from Tue → next Monday
  r := next_schedule_time('{"freq":"weekly","days":["mon"],"time":"08:00"}', 'Europe/Berlin', '2026-11-10 12:00+00');
  ASSERT r = '2026-11-16 07:00+00', 'weekly next monday: ' || r;
  -- weekly Tue+Fri 10:00 from Tue 12:00 local → Fri
  r := next_schedule_time('{"freq":"weekly","days":["tue","fri"],"time":"10:00"}', 'Europe/Berlin', '2026-11-10 11:00+00');
  ASSERT r = '2026-11-13 09:00+00', 'weekly two days: ' || r;
  -- DST: Berlin switches to summer time on 2027-03-28; 07:30 local is 06:30 UTC before, 05:30 UTC after
  r := next_schedule_time('{"freq":"daily","time":"07:30"}', 'Europe/Berlin', '2027-03-27 07:00+00');
  ASSERT r = '2027-03-28 05:30+00', 'DST spring forward: ' || r;
  -- weekly with no days never fires
  r := next_schedule_time('{"freq":"weekly","time":"08:00"}', 'Europe/Berlin', now());
  ASSERT r IS NULL, 'weekly without days should be NULL';
  -- unknown freq is an error
  BEGIN
    PERFORM next_schedule_time('{"freq":"hourly"}', 'UTC', now());
    RAISE EXCEPTION 'unknown freq accepted';
  EXCEPTION WHEN invalid_parameter_value THEN NULL;
  END;
END $$;

-- ---- enqueue_due_triggers ----------------------------------------------------
-- Workspace A (from rls_test): helper A gets a current version and three triggers.
UPDATE helpers SET status = 'active', current_version_id = 'aaaaaaaa-3333-0000-0000-000000000001'
 WHERE id = 'aaaaaaaa-2222-0000-0000-000000000001';
UPDATE helpers SET status = 'paused', current_version_id = 'bbbbbbbb-3333-0000-0000-000000000002'
 WHERE id = 'bbbbbbbb-2222-0000-0000-000000000002';

INSERT INTO triggers (id, workspace_id, user_id, helper_id, type, config, next_run_at) VALUES
  -- due, active helper → job
  ('aaaaaaaa-5555-0000-0000-000000000001', 'aaaaaaaa-1111-0000-0000-000000000001', 'aaaaaaaa-0000-0000-0000-000000000001',
   'aaaaaaaa-2222-0000-0000-000000000001', 'schedule', '{"freq":"daily","time":"08:00"}', now() - interval '3 hours'),
  -- not due yet → untouched
  ('aaaaaaaa-5555-0000-0000-000000000002', 'aaaaaaaa-1111-0000-0000-000000000001', 'aaaaaaaa-0000-0000-0000-000000000001',
   'aaaaaaaa-2222-0000-0000-000000000001', 'schedule', '{"freq":"daily","time":"08:00"}', now() + interval '1 hour'),
  -- due, paused helper → no job, but advanced
  ('bbbbbbbb-5555-0000-0000-000000000003', 'bbbbbbbb-1111-0000-0000-000000000002', 'bbbbbbbb-0000-0000-0000-000000000002',
   'bbbbbbbb-2222-0000-0000-000000000002', 'schedule', '{"freq":"daily","time":"08:00"}', now() - interval '1 minute'),
  -- manual trigger with a past next_run_at is ignored
  ('aaaaaaaa-5555-0000-0000-000000000004', 'aaaaaaaa-1111-0000-0000-000000000001', 'aaaaaaaa-0000-0000-0000-000000000001',
   'aaaaaaaa-2222-0000-0000-000000000001', 'manual', '{}', now() - interval '1 day');

DO $$
DECLARE res jsonb; j record;
BEGIN
  res := enqueue_due_triggers();
  ASSERT res = '{"enqueued": 1, "skipped": 1}'::jsonb, 'enqueue result: ' || res;

  SELECT * INTO j FROM jobs;
  ASSERT j.kind = 'run' AND j.status = 'queued', 'one queued run job';
  ASSERT j.workspace_id = 'aaaaaaaa-1111-0000-0000-000000000001', 'job has workspace';
  ASSERT j.user_id = '00000000-0000-0000-0000-000000000000', 'job owner is the system id';
  ASSERT j.payload->>'trigger_id' = 'aaaaaaaa-5555-0000-0000-000000000001', 'payload trigger id';
  ASSERT j.payload->>'version_id' = 'aaaaaaaa-3333-0000-0000-000000000001', 'payload version id';

  ASSERT (SELECT next_run_at > now() FROM triggers WHERE id = 'aaaaaaaa-5555-0000-0000-000000000001'),
    'due trigger advanced into the future';
  ASSERT (SELECT next_run_at > now() FROM triggers WHERE id = 'bbbbbbbb-5555-0000-0000-000000000003'),
    'paused trigger advanced too';
  ASSERT (SELECT next_run_at < now() FROM triggers WHERE id = 'aaaaaaaa-5555-0000-0000-000000000004'),
    'manual trigger untouched';

  -- second call in the same minute does nothing
  res := enqueue_due_triggers();
  ASSERT res = '{"enqueued": 0, "skipped": 0}'::jsonb, 'idempotent within the minute: ' || res;
  ASSERT (SELECT count(*) FROM jobs) = 1, 'still one job';
END $$;

-- ---- claim_jobs -------------------------------------------------------------
DO $$
DECLARE c jsonb;
BEGIN
  INSERT INTO jobs (kind, run_after) VALUES ('later', now() + interval '1 hour');

  c := claim_jobs('w1', 10);
  ASSERT jsonb_array_length(c) = 1, 'claims only the due job: ' || c;
  ASSERT c->0->>'status' = 'running' AND c->0->>'claimed_by' = 'w1', 'claimed by w1';
  ASSERT (c->0->>'attempts')::int = 1, 'attempts incremented';
  ASSERT (c->0->>'lease_until')::timestamptz > now(), 'lease set';

  ASSERT jsonb_array_length(claim_jobs('w2', 10)) = 0, 'nothing left for w2';

  -- worker w1 crashes: lease expires → w2 reclaims
  UPDATE jobs SET lease_until = now() - interval '1 second' WHERE claimed_by = 'w1';
  c := claim_jobs('w2', 10);
  ASSERT jsonb_array_length(c) = 1 AND c->0->>'claimed_by' = 'w2', 'expired lease reclaimed';
  ASSERT (c->0->>'attempts')::int = 2, 'attempts counted on reclaim';

  -- last attempt expires → failed, not reclaimed
  UPDATE jobs SET lease_until = now() - interval '1 second', attempts = max_attempts WHERE claimed_by = 'w2';
  ASSERT jsonb_array_length(claim_jobs('w3', 10)) = 0, 'exhausted job not reclaimed';
  ASSERT (SELECT status FROM jobs WHERE kind = 'run') = 'failed', 'exhausted job marked failed';

  BEGIN
    PERFORM claim_jobs('w1', 0);
    RAISE EXCEPTION 'n=0 accepted';
  EXCEPTION WHEN invalid_parameter_value THEN NULL;
  END;
END $$;

-- ---- insert_cards ------------------------------------------------------------
INSERT INTO run_steps (id, workspace_id, user_id, run_id, brick_id, brick_type, position) VALUES
  ('aaaaaaaa-6666-0000-0000-000000000001', 'aaaaaaaa-1111-0000-0000-000000000001', 'aaaaaaaa-0000-0000-0000-000000000001',
   'aaaaaaaa-4444-0000-0000-000000000001', 'b2', 'look.search', 1);

DO $$
DECLARE res jsonb;
BEGIN
  res := insert_cards(jsonb_build_array(
    jsonb_build_object('workspace_id', 'aaaaaaaa-1111-0000-0000-000000000001', 'user_id', 'aaaaaaaa-0000-0000-0000-000000000001',
      'run_id', 'aaaaaaaa-4444-0000-0000-000000000001', 'step_id', 'aaaaaaaa-6666-0000-0000-000000000001',
      'card_id', 'cccccccc-0000-0000-0000-000000000001', 'kind', 'web_result',
      'data', '{"title": "Bakery news", "url": "https://example.com"}'::jsonb, 'trust', '{"title": "untrusted"}'::jsonb),
    jsonb_build_object('workspace_id', 'aaaaaaaa-1111-0000-0000-000000000001', 'user_id', 'aaaaaaaa-0000-0000-0000-000000000001',
      'run_id', 'aaaaaaaa-4444-0000-0000-000000000001', 'step_id', 'aaaaaaaa-6666-0000-0000-000000000001',
      'card_id', 'cccccccc-0000-0000-0000-000000000002', 'kind', 'web_result')
  ));
  ASSERT jsonb_array_length(res) = 2, 'two cards inserted: ' || res;
  ASSERT res->0->>'card_id' = 'cccccccc-0000-0000-0000-000000000001', 'input order kept';
  ASSERT (SELECT array_agg("position" ORDER BY "position") FROM cards) = ARRAY[0, 1], 'positions default to input order';
  ASSERT (SELECT data->>'title' FROM cards WHERE card_id = 'cccccccc-0000-0000-0000-000000000001') = 'Bakery news', 'data stored';

  ASSERT insert_cards('[]'::jsonb) = '[]'::jsonb, 'empty pile ok';

  -- all-or-nothing: a bad row rolls back the whole call
  BEGIN
    PERFORM insert_cards(jsonb_build_array(
      jsonb_build_object('workspace_id', 'aaaaaaaa-1111-0000-0000-000000000001', 'user_id', 'aaaaaaaa-0000-0000-0000-000000000001',
        'run_id', 'aaaaaaaa-4444-0000-0000-000000000001', 'step_id', 'aaaaaaaa-6666-0000-0000-000000000001',
        'card_id', 'cccccccc-0000-0000-0000-000000000003', 'kind', 'web_result'),
      jsonb_build_object('card_id', 'cccccccc-0000-0000-0000-000000000004') -- missing required fields
    ));
    RAISE EXCEPTION 'bad pile accepted';
  EXCEPTION WHEN not_null_violation THEN NULL;
  END;
  ASSERT (SELECT count(*) FROM cards) = 2, 'bad pile rolled back';
END $$;

-- ---- non-service callers are refused ------------------------------------------
SELECT pg_temp.act('authenticated', 'aaaaaaaa-0000-0000-0000-000000000001');
DO $$ BEGIN
  BEGIN PERFORM claim_jobs('evil', 10); RAISE EXCEPTION 'user claimed jobs';
  EXCEPTION WHEN insufficient_privilege THEN NULL; END;
  BEGIN PERFORM enqueue_due_triggers(); RAISE EXCEPTION 'user enqueued triggers';
  EXCEPTION WHEN insufficient_privilege THEN NULL; END;
  BEGIN PERFORM insert_cards('[]'::jsonb); RAISE EXCEPTION 'user inserted cards';
  EXCEPTION WHEN insufficient_privilege THEN NULL; END;
  -- next_schedule_time is a pure helper and may be called by anyone
  PERFORM next_schedule_time('{"freq":"daily","time":"08:00"}', 'UTC', now());
END $$;
SELECT pg_temp.act('anon', NULL);
DO $$ BEGIN
  BEGIN PERFORM claim_jobs('evil', 10); RAISE EXCEPTION 'anon claimed jobs';
  EXCEPTION WHEN insufficient_privilege THEN NULL; END;
END $$;

\echo 'Queue tests passed'
