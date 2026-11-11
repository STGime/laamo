-- Tests for resume_waiting_runs (0011). Runs as tenant_func after rls_test.sql
-- (reuses its workspace, helper and version A).
\set ON_ERROR_STOP on
\o /dev/null
SET search_path = tenant_test, public;

CREATE FUNCTION pg_temp.act(role text, uid uuid) RETURNS void LANGUAGE sql AS $$
  SELECT set_config('app.end_user_role', role, false),
         set_config('app.end_user_id', coalesce(uid::text, ''), false);
$$;
SELECT pg_temp.act('service', NULL);
DELETE FROM jobs;

DO $$
DECLARE
  ws  uuid := 'aaaaaaaa-1111-0000-0000-000000000001';
  usr uuid := 'aaaaaaaa-0000-0000-0000-000000000001';
  hlp uuid := 'aaaaaaaa-2222-0000-0000-000000000001';
  ver uuid := 'aaaaaaaa-3333-0000-0000-000000000001';
  r_open uuid; r_decided uuid; r_expired uuid; r_other uuid;
  fc uuid;
  res jsonb;
BEGIN
  -- Four waiting runs: card open, decided, expired; and one for the single-run call.
  FOR i IN 1..4 LOOP
    INSERT INTO runs (workspace_id, user_id, helper_id, version_id, trigger_type, status)
      VALUES (ws, usr, hlp, ver, 'manual', 'waiting') RETURNING id INTO fc;
    IF i = 1 THEN r_open := fc; ELSIF i = 2 THEN r_decided := fc; ELSIF i = 3 THEN r_expired := fc; ELSE r_other := fc; END IF;
  END LOOP;
  INSERT INTO feed_cards (workspace_id, user_id, run_id, type, title, state, expires_at)
    VALUES (ws, usr, r_open, 'approval', 'open', 'open', now() + interval '1 day') RETURNING id INTO fc;
  UPDATE runs SET waiting_on_card_id = fc WHERE id = r_open;
  INSERT INTO feed_cards (workspace_id, user_id, run_id, type, title, state)
    VALUES (ws, usr, r_decided, 'approval', 'decided', 'approved') RETURNING id INTO fc;
  UPDATE runs SET waiting_on_card_id = fc WHERE id = r_decided;
  INSERT INTO feed_cards (workspace_id, user_id, run_id, type, title, state, expires_at)
    VALUES (ws, usr, r_expired, 'approval', 'expired', 'open', now() - interval '1 minute') RETURNING id INTO fc;
  UPDATE runs SET waiting_on_card_id = fc WHERE id = r_expired;
  INSERT INTO feed_cards (workspace_id, user_id, run_id, type, title, state)
    VALUES (ws, usr, r_other, 'question', 'other', 'answered') RETURNING id INTO fc;
  UPDATE runs SET waiting_on_card_id = fc WHERE id = r_other;

  -- One run only.
  res := resume_waiting_runs(r_other);
  ASSERT (res->>'enqueued')::int = 1, 'single run: ' || res::text;
  ASSERT (SELECT count(*) FROM jobs WHERE payload->>'run_id' = r_other::text) = 1, 'job for that run';

  -- The sweep: decided and expired, not open; r_other already has a job.
  res := resume_waiting_runs();
  ASSERT (res->>'enqueued')::int = 2, 'sweep: ' || res::text;
  ASSERT (SELECT count(*) FROM jobs WHERE kind = 'run' AND payload->>'run_id' IN (r_decided::text, r_expired::text)) = 2, 'decided + expired';
  ASSERT NOT EXISTS (SELECT 1 FROM jobs WHERE payload->>'run_id' = r_open::text), 'open card keeps waiting';

  -- Running it again queues nothing new while those jobs are queued.
  res := resume_waiting_runs();
  ASSERT (res->>'enqueued')::int = 0, 'no duplicates: ' || res::text;

  -- A finished job doesn't block a later resume (e.g. a second Ask me brick).
  UPDATE jobs SET status = 'done' WHERE payload->>'run_id' = r_decided::text;
  res := resume_waiting_runs(r_decided);
  ASSERT (res->>'enqueued')::int = 1, 'after the job finished: ' || res::text;

  -- A run that isn't waiting is left alone.
  UPDATE runs SET status = 'done' WHERE id = r_open;
  UPDATE feed_cards SET state = 'approved' WHERE run_id = r_open;
  ASSERT (resume_waiting_runs(r_open)->>'enqueued')::int = 0, 'finished run';

  DELETE FROM jobs;
END $$;

-- End users can't call it.
SELECT pg_temp.act('authenticated', 'aaaaaaaa-0000-0000-0000-000000000001');
DO $$
BEGIN
  PERFORM resume_waiting_runs();
  RAISE EXCEPTION 'end user resumed runs';
EXCEPTION WHEN insufficient_privilege THEN NULL;
END $$;

\o
\echo Resume tests passed
