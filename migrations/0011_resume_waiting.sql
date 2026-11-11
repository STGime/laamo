-- Resume runs that wait for the user (#20). A run pauses at an Ask me brick
-- with status 'waiting' and waiting_on_card_id set. Once its Feed card is
-- decided, or has expired, a 'run' job brings it back; the engine applies
-- the decision. The API calls this for one run right after a decision; the
-- cron job sweeps every minute for expired cards and anything missed.

-- Finds the queued/running job of a run quickly.
CREATE INDEX jobs_run_id_idx ON jobs ((payload->>'run_id'))
  WHERE kind = 'run' AND status IN ('queued', 'running');

CREATE OR REPLACE FUNCTION resume_waiting_runs(p_run uuid DEFAULT NULL)
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  enqueued int;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'resume_waiting_runs is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;

  WITH due AS (
    SELECT r.id, r.workspace_id
      FROM runs r
      JOIN feed_cards f ON f.id = r.waiting_on_card_id
     WHERE r.status = 'waiting'
       AND (p_run IS NULL OR r.id = p_run)
       AND (f.state <> 'open' OR f.expires_at <= now())
       AND NOT EXISTS (
             SELECT 1 FROM jobs j
              WHERE j.kind = 'run'
                AND j.status IN ('queued', 'running')
                AND j.payload->>'run_id' = r.id::text)
     ORDER BY r.created_at
     LIMIT 500
     FOR UPDATE OF r SKIP LOCKED
  ), ins AS (
    INSERT INTO jobs (workspace_id, kind, payload)
    SELECT workspace_id, 'run', jsonb_build_object('run_id', id) FROM due
    RETURNING 1
  )
  SELECT count(*) INTO enqueued FROM ins;

  RETURN jsonb_build_object('enqueued', enqueued);
END;
$$;
