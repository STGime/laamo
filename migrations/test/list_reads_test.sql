-- Tests for take_list_rows (0015). Runs as tenant_func after rls_test.sql
-- (reuses its workspace and helper).
\set ON_ERROR_STOP on
\o /dev/null
SET search_path = tenant_test, public;

CREATE FUNCTION pg_temp.act(role text, uid uuid) RETURNS void LANGUAGE sql AS $$
  SELECT set_config('app.end_user_role', role, false),
         set_config('app.end_user_id', coalesce(uid::text, ''), false);
$$;
SELECT pg_temp.act('service', NULL);

DO $$
DECLARE
  ws   uuid := 'aaaaaaaa-1111-0000-0000-000000000001';
  usr  uuid := 'aaaaaaaa-0000-0000-0000-000000000001';
  hlp  uuid := 'aaaaaaaa-2222-0000-0000-000000000001';
  lst  uuid;
  run1 uuid := gen_random_uuid();
  run2 uuid := gen_random_uuid();
  got  jsonb;
BEGIN
  INSERT INTO lists (workspace_id, user_id, name) VALUES (ws, usr, 'Leads (reads test)') RETURNING id INTO lst;
  FOR i IN 1..5 LOOP
    INSERT INTO list_rows (workspace_id, user_id, list_id, data, created_at)
      VALUES (ws, usr, lst, jsonb_build_object('n', i), now() - make_interval(mins => 10 - i));
  END LOOP;

  -- A test run looks without using anything.
  got := take_list_rows(lst, hlp, gen_random_uuid(), 2, false);
  ASSERT jsonb_array_length(got) = 2 AND (got->0->'data'->>'n') = '1', 'test peek: ' || got::text;
  ASSERT NOT EXISTS (SELECT 1 FROM list_reads WHERE helper_id = hlp), 'a test marked rows';

  -- Run 1 takes the oldest two; its retry gets the same two.
  got := take_list_rows(lst, hlp, run1, 2, true);
  ASSERT (got->0->'data'->>'n') = '1' AND (got->1->'data'->>'n') = '2', 'run 1: ' || got::text;
  got := take_list_rows(lst, hlp, run1, 2, true);
  ASSERT jsonb_array_length(got) = 2 AND (got->0->'data'->>'n') = '1', 'run 1 retry: ' || got::text;

  -- Run 2 gets what's left.
  got := take_list_rows(lst, hlp, run2, 10, true);
  ASSERT jsonb_array_length(got) = 3 AND (got->0->'data'->>'n') = '3', 'run 2: ' || got::text;
  got := take_list_rows(lst, hlp, gen_random_uuid(), 10, true);
  ASSERT jsonb_array_length(got) = 0, 'nothing new: ' || got::text;

  -- A deleted row is gone from the reads too.
  DELETE FROM list_rows WHERE list_id = lst AND data->>'n' = '1';
  ASSERT (SELECT count(*) FROM list_reads WHERE helper_id = hlp) = 4, 'reads cascade';

  BEGIN PERFORM take_list_rows(lst, hlp, run1, 0, true); RAISE EXCEPTION 'limit 0 accepted';
  EXCEPTION WHEN invalid_parameter_value THEN NULL; END;
  DELETE FROM lists WHERE id = lst;
END $$;

SELECT pg_temp.act('authenticated', 'aaaaaaaa-0000-0000-0000-000000000001');
DO $$ BEGIN
  BEGIN PERFORM take_list_rows(gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), 1, false); RAISE EXCEPTION 'user called take_list_rows';
  EXCEPTION WHEN insufficient_privilege THEN NULL; END;
END $$;
