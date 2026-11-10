-- Tests for add_brain_usage (0009). Runs as tenant_func.
\set ON_ERROR_STOP on
\o /dev/null
SET search_path = tenant_test, public;

CREATE FUNCTION pg_temp.act(role text, uid uuid) RETURNS void LANGUAGE sql AS $$
  SELECT set_config('app.end_user_role', role, false),
         set_config('app.end_user_id', coalesce(uid::text, ''), false);
$$;
SELECT pg_temp.act('service', NULL);

DO $$
DECLARE ws jsonb; wid uuid; b jsonb;
BEGIN
  ws := create_workspace('bbbbbbbb-0000-0000-0000-000000000002', 'Brain test');
  wid := (ws->>'id')::uuid;
  ASSERT (SELECT allowance_credits FROM brains WHERE workspace_id = wid) = 500, 'new brains get 500 credits';

  b := add_brain_usage(wid, 120.5);
  ASSERT (b->>'used_credits_this_period')::numeric = 120.5 AND b->>'status' = 'ok', 'first usage: ' || b::text;
  b := add_brain_usage(wid, 400);
  ASSERT (b->>'used_credits_this_period')::numeric = 520.5 AND b->>'status' = 'allowance_used', 'over allowance: ' || b::text;

  -- A new month starts a new period.
  UPDATE brains SET period_start = date_trunc('month', now()) - interval '1 month' WHERE workspace_id = wid;
  b := add_brain_usage(wid, 1);
  ASSERT (b->>'used_credits_this_period')::numeric = 1 AND b->>'status' = 'ok', 'new period: ' || b::text;

  BEGIN PERFORM add_brain_usage(wid, -1); RAISE EXCEPTION 'negative credits accepted';
  EXCEPTION WHEN invalid_parameter_value THEN NULL; END;
  BEGIN PERFORM add_brain_usage(gen_random_uuid(), 1); RAISE EXCEPTION 'unknown workspace accepted';
  EXCEPTION WHEN no_data_found THEN NULL; END;
END $$;

SELECT pg_temp.act('authenticated', 'bbbbbbbb-0000-0000-0000-000000000002');
DO $$ BEGIN
  BEGIN PERFORM add_brain_usage(gen_random_uuid(), 1); RAISE EXCEPTION 'user called add_brain_usage';
  EXCEPTION WHEN insufficient_privilege THEN NULL; END;
END $$;

\echo 'Brain tests passed'
