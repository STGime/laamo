-- Tests for extend_lease and finish_job (0007). Runs as tenant_func after queue_test.sql.
\set ON_ERROR_STOP on
\o /dev/null
SET search_path = tenant_test, public;

CREATE FUNCTION pg_temp.act(role text, uid uuid) RETURNS void LANGUAGE sql AS $$
  SELECT set_config('app.end_user_role', role, false),
         set_config('app.end_user_id', coalesce(uid::text, ''), false);
$$;
SELECT pg_temp.act('service', NULL);
DELETE FROM jobs;

-- ---- extend_lease ------------------------------------------------------------
DO $$
DECLARE r jsonb; jid uuid;
BEGIN
  INSERT INTO jobs (kind) VALUES ('extend');
  jid := (claim_jobs('w1', 1)->0->>'id')::uuid;

  r := extend_lease(jid, 'w1', 1, 600);
  ASSERT (r->>'applied')::boolean, 'holder extends: ' || r::text;
  ASSERT (SELECT lease_until > now() + interval '9 minutes' FROM jobs WHERE id = jid), 'lease pushed';

  r := extend_lease(jid, 'w2', 1, 600);
  ASSERT NOT (r->>'applied')::boolean, 'other worker cannot extend';
  r := extend_lease(jid, 'w1', 2, 600);
  ASSERT NOT (r->>'applied')::boolean, 'wrong attempt cannot extend';

  BEGIN
    PERFORM extend_lease(jid, 'w1', 1, 0);
    RAISE EXCEPTION 'seconds=0 accepted';
  EXCEPTION WHEN invalid_parameter_value THEN NULL;
  END;
  DELETE FROM jobs;
END $$;

-- ---- finish_job --------------------------------------------------------------
DO $$
DECLARE r jsonb; jid uuid; j record;
BEGIN
  -- done
  INSERT INTO jobs (kind) VALUES ('ok');
  jid := (claim_jobs('w1', 1)->0->>'id')::uuid;
  r := finish_job(jid, 'w1', 1, 'done');
  ASSERT r = '{"applied": true, "status": "done"}'::jsonb, 'done: ' || r::text;
  SELECT * INTO j FROM jobs WHERE id = jid;
  ASSERT j.status = 'done' AND j.lease_until IS NULL, 'done row';
  r := finish_job(jid, 'w1', 1, 'done');
  ASSERT NOT (r->>'applied')::boolean, 'finishing twice is a no-op';

  -- retry → queued with backoff, then failed when attempts are used up
  INSERT INTO jobs (kind, max_attempts) VALUES ('flaky', 2);
  jid := (claim_jobs('w1', 1)->0->>'id')::uuid;
  r := finish_job(jid, 'w1', 1, 'retry', 'timeout from Brave', 120);
  ASSERT r->>'status' = 'queued', 'first failure requeues: ' || r::text;
  SELECT * INTO j FROM jobs WHERE id = jid;
  ASSERT j.run_after > now() + interval '119 seconds', 'backoff applied';
  ASSERT j.last_error = 'timeout from Brave', 'error kept';
  ASSERT jsonb_array_length(claim_jobs('w2', 1)) = 0, 'not claimable during backoff';

  UPDATE jobs SET run_after = now() WHERE id = jid;
  ASSERT (claim_jobs('w2', 1)->0->>'attempts')::int = 2, 'second attempt claimed';
  r := finish_job(jid, 'w2', 2, 'retry', 'timeout again', 60);
  ASSERT r->>'status' = 'failed', 'out of attempts → failed: ' || r::text;
  ASSERT (SELECT last_error FROM jobs WHERE id = jid) = E'timeout from Brave\ntimeout again', 'errors appended';

  -- failed: permanent, no retry even with attempts left
  INSERT INTO jobs (kind) VALUES ('broken');
  jid := (claim_jobs('w1', 1)->0->>'id')::uuid;
  r := finish_job(jid, 'w1', 1, 'failed', 'unknown job kind');
  ASSERT r->>'status' = 'failed', 'permanent failure: ' || r::text;

  -- a worker whose lease expired and was reclaimed cannot finish the job
  INSERT INTO jobs (kind) VALUES ('slow');
  jid := (claim_jobs('w1', 1)->0->>'id')::uuid;
  UPDATE jobs SET lease_until = now() - interval '1 second' WHERE id = jid;
  ASSERT (claim_jobs('w2', 1)->0->>'claimed_by') = 'w2', 'w2 reclaims';
  r := finish_job(jid, 'w1', 1, 'done');
  ASSERT NOT (r->>'applied')::boolean, 'stale w1 cannot finish';
  r := extend_lease(jid, 'w1', 1, 60);
  ASSERT NOT (r->>'applied')::boolean, 'stale w1 cannot extend';
  ASSERT (SELECT status FROM jobs WHERE id = jid) = 'running', 'still running for w2';
  r := finish_job(jid, 'w2', 2, 'done');
  ASSERT (r->>'applied')::boolean, 'w2 finishes';

  BEGIN
    PERFORM finish_job(jid, 'w2', 2, 'maybe');
    RAISE EXCEPTION 'bad outcome accepted';
  EXCEPTION WHEN invalid_parameter_value THEN NULL;
  END;
  DELETE FROM jobs;
END $$;

-- ---- non-service callers are refused -------------------------------------------
SELECT pg_temp.act('authenticated', 'aaaaaaaa-0000-0000-0000-000000000001');
DO $$ BEGIN
  BEGIN PERFORM extend_lease(gen_random_uuid(), 'evil', 1, 60); RAISE EXCEPTION 'user extended a lease';
  EXCEPTION WHEN insufficient_privilege THEN NULL; END;
  BEGIN PERFORM finish_job(gen_random_uuid(), 'evil', 1, 'done'); RAISE EXCEPTION 'user finished a job';
  EXCEPTION WHEN insufficient_privilege THEN NULL; END;
END $$;

\echo 'Job finish tests passed'
