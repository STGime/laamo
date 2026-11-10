-- Brain allowance metering (issue #23).
--
-- 1 brain credit = €0.001 of model cost. The free plan gets 500 credits a
-- month; plans set their own allowance in billing (#54).
ALTER TABLE brains ALTER COLUMN allowance_credits SET DEFAULT 500;
UPDATE brains SET allowance_credits = 500 WHERE allowance_credits = 0;

-- add_brain_usage: count credits against a workspace's brain, atomically.
-- Starts a new period (used = 0) when the month has changed, and marks the
-- brain allowance_used once the included allowance is spent.
-- Returns the brain row.
CREATE FUNCTION add_brain_usage(ws uuid, credits numeric)
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  b brains;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'add_brain_usage is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;
  IF credits IS NULL OR credits < 0 THEN
    RAISE EXCEPTION 'credits must be zero or more' USING ERRCODE = 'invalid_parameter_value';
  END IF;

  SELECT * INTO b FROM brains WHERE workspace_id = ws FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'workspace % has no brain', ws USING ERRCODE = 'no_data_found';
  END IF;

  IF b.period_start < date_trunc('month', now()) THEN
    b.used_credits_this_period := 0;
    b.period_start := date_trunc('month', now());
  END IF;
  b.used_credits_this_period := b.used_credits_this_period + credits;

  UPDATE brains
     SET used_credits_this_period = b.used_credits_this_period,
         period_start = b.period_start,
         status = CASE
                    WHEN b.mode = 'included' AND b.used_credits_this_period >= b.allowance_credits THEN 'allowance_used'
                    WHEN b.status = 'allowance_used' THEN 'ok'
                    ELSE b.status
                  END
   WHERE id = b.id
  RETURNING * INTO b;

  RETURN to_jsonb(b);
END;
$$;
