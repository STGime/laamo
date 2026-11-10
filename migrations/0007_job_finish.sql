-- Lease extension and job completion for workers (issue #9).
--
-- Eurobase REST can only PATCH a row by id, with no extra conditions. A worker
-- whose lease expired (and whose job another worker reclaimed) must not be
-- able to extend or finish it, so these are RPCs that check the claim:
-- status = 'running', claimed_by = worker and attempts = attempt.
-- Both return {"applied": false} when the worker no longer holds the job.

-- extend_lease: push lease_until to now() + seconds (1 … 3600).
-- Call: POST /v1/db/rpc/extend_lease {"job_id": …, "worker": "w1", "attempt": 2, "seconds": 300}
CREATE FUNCTION extend_lease(job_id uuid, worker text, attempt int, seconds int DEFAULT 300)
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  until timestamptz;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'extend_lease is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;
  IF seconds IS NULL OR seconds < 1 OR seconds > 3600 THEN
    RAISE EXCEPTION 'seconds must be between 1 and 3600' USING ERRCODE = 'invalid_parameter_value';
  END IF;

  UPDATE jobs j
     SET lease_until = now() + make_interval(secs => seconds)
   WHERE j.id = job_id
     AND j.status = 'running'
     AND j.claimed_by = worker
     AND j.attempts = attempt
  RETURNING j.lease_until INTO until;

  RETURN jsonb_build_object('applied', until IS NOT NULL, 'lease_until', until);
END;
$$;

-- finish_job: record the outcome of a claimed job.
--   outcome 'done'   → status done
--   outcome 'retry'  → back to queued with run_after = now() + retry_in seconds,
--                      or failed when no attempts are left
--   outcome 'failed' → status failed (permanent error, no retry)
-- error is appended to last_error for 'retry' and 'failed'.
-- Returns {"applied": bool, "status": final status or null}.
CREATE FUNCTION finish_job(job_id uuid, worker text, attempt int, outcome text,
                           error text DEFAULT NULL, retry_in int DEFAULT 60)
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  final text;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'finish_job is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;
  IF outcome IS NULL OR outcome NOT IN ('done', 'retry', 'failed') THEN
    RAISE EXCEPTION 'outcome must be done, retry or failed' USING ERRCODE = 'invalid_parameter_value';
  END IF;
  IF outcome = 'retry' AND (retry_in IS NULL OR retry_in < 0 OR retry_in > 86400) THEN
    RAISE EXCEPTION 'retry_in must be between 0 and 86400 seconds' USING ERRCODE = 'invalid_parameter_value';
  END IF;

  UPDATE jobs j
     SET status = CASE
                    WHEN outcome = 'done' THEN 'done'
                    WHEN outcome = 'retry' AND j.attempts < j.max_attempts THEN 'queued'
                    ELSE 'failed'
                  END,
         run_after = CASE
                       WHEN outcome = 'retry' AND j.attempts < j.max_attempts
                       THEN now() + make_interval(secs => retry_in)
                       ELSE j.run_after
                     END,
         last_error = CASE
                        WHEN outcome = 'done' OR error IS NULL THEN j.last_error
                        ELSE right(coalesce(j.last_error || E'\n', '') || error, 4000) -- keep the newest
                      END,
         lease_until = NULL
   WHERE j.id = job_id
     AND j.status = 'running'
     AND j.claimed_by = worker
     AND j.attempts = attempt
  RETURNING j.status INTO final;

  RETURN jsonb_build_object('applied', final IS NOT NULL, 'status', final);
END;
$$;
