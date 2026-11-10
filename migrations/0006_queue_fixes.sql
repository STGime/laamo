-- Review fixes for 0005 (PR #79). 0005 is already applied, so the functions are replaced here.
--
-- enqueue_due_triggers: one trigger with a bad config or workspace timezone used to raise
-- and abort the whole call, every minute, stopping all schedules. Now each trigger runs in
-- its own sub-transaction; a failing trigger is switched off (next_run_at = NULL) and
-- reported in the result, and the rest are queued as normal.
--
-- claim_jobs: a job put back to 'queued' after its final attempt was claimed again past
-- max_attempts. Such jobs are now marked failed, and both pick branches require
-- attempts < max_attempts.

CREATE OR REPLACE FUNCTION enqueue_due_triggers()
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  t         record;
  next_at   timestamptz;
  enqueued  int := 0;
  skipped   int := 0;
  errored   jsonb := '[]'::jsonb;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'enqueue_due_triggers is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;

  FOR t IN
    SELECT tr.id, tr.helper_id, tr.workspace_id, tr.config, tr.next_run_at,
           w.timezone, h.status AS helper_status, h.current_version_id
      FROM triggers tr
      JOIN helpers h    ON h.id = tr.helper_id
      JOIN workspaces w ON w.id = tr.workspace_id
     WHERE tr.type = 'schedule'
       AND tr.next_run_at <= now()
     ORDER BY tr.next_run_at
     LIMIT 500
     FOR UPDATE OF tr SKIP LOCKED
  LOOP
    BEGIN
      -- Compute first: if the config is bad, nothing is queued for this trigger.
      next_at := next_schedule_time(t.config, t.timezone, now());

      IF t.helper_status = 'active' AND t.current_version_id IS NOT NULL THEN
        INSERT INTO jobs (workspace_id, kind, payload)
        VALUES (t.workspace_id, 'run', jsonb_build_object(
          'trigger_id',    t.id,
          'helper_id',     t.helper_id,
          'version_id',    t.current_version_id,
          'trigger_type',  'schedule',
          'scheduled_for', t.next_run_at
        ));
        enqueued := enqueued + 1;
      ELSE
        skipped := skipped + 1;
      END IF;

      UPDATE triggers SET next_run_at = next_at WHERE id = t.id;
    EXCEPTION WHEN OTHERS THEN
      -- Switch the trigger off so it can't block the scheduler; the API re-arms it on save.
      UPDATE triggers SET next_run_at = NULL WHERE id = t.id;
      errored := errored || jsonb_build_object('trigger_id', t.id, 'error', SQLERRM);
    END;
  END LOOP;

  RETURN jsonb_build_object(
    'enqueued', enqueued,
    'skipped',  skipped,
    'errored',  jsonb_array_length(errored),
    'errors',   errored
  );
END;
$$;

CREATE OR REPLACE FUNCTION claim_jobs(worker text, n int DEFAULT 10)
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  claimed jsonb;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'claim_jobs is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;
  IF n IS NULL OR n < 1 OR n > 100 THEN
    RAISE EXCEPTION 'n must be between 1 and 100' USING ERRCODE = 'invalid_parameter_value';
  END IF;

  -- Out of attempts: an expired lease on the last attempt, or re-queued after it.
  UPDATE jobs
     SET status = 'failed',
         last_error = coalesce(last_error || E'\n', '') ||
                      CASE status WHEN 'running' THEN 'lease expired after final attempt'
                                  ELSE 'no attempts left' END,
         lease_until = NULL
   WHERE attempts >= max_attempts
     AND ((status = 'running' AND lease_until < now())
       OR (status = 'queued'  AND run_after  <= now()));

  WITH picked AS (
    SELECT id
      FROM jobs
     WHERE attempts < max_attempts
       AND ((status = 'queued'  AND run_after   <= now())
         OR (status = 'running' AND lease_until <  now()))
     ORDER BY run_after
     LIMIT n
     FOR UPDATE SKIP LOCKED
  ), updated AS (
    UPDATE jobs j
       SET status      = 'running',
           claimed_by  = worker,
           claimed_at  = now(),
           lease_until = now() + interval '5 minutes',
           attempts    = j.attempts + 1
      FROM picked
     WHERE j.id = picked.id
    RETURNING j.*
  )
  SELECT coalesce(jsonb_agg(to_jsonb(updated) ORDER BY updated.run_after), '[]'::jsonb)
    INTO claimed
    FROM updated;

  RETURN claimed;
END;
$$;
