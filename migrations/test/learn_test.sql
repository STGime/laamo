-- Tests for pick_examples (0014). Runs as tenant_func after rls_test.sql
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
  step uuid := gen_random_uuid();
  row  jsonb;
  rows jsonb := '[]';
  res  jsonb;
BEGIN
  FOR i IN 1..3 LOOP
    rows := rows || jsonb_build_object('workspace_id', ws, 'user_id', usr, 'helper_id', hlp, 'step_id', step,
      'card_id', 'c' || i, 'kept', i = 1, 'title', repeat('x', 400), 'text', 'Bakery news ' || i);
  END LOOP;
  res := add_pick_examples(rows);
  ASSERT res::int = 3, 'added: ' || res::text;
  ASSERT (SELECT length(title) FROM pick_examples WHERE card_id = 'c1') = 300, 'long titles are cut';
  -- A retried job adds nothing twice.
  res := add_pick_examples(rows);
  ASSERT res::int = 0, 'retry: ' || res::text;
  ASSERT (SELECT count(*) FROM pick_examples WHERE helper_id = hlp AND kept) = 1, 'one kept';

  -- At most 200 per helper: the oldest go.
  rows := '[]';
  FOR i IN 1..205 LOOP
    rows := rows || jsonb_build_object('workspace_id', ws, 'user_id', usr, 'helper_id', hlp,
      'step_id', gen_random_uuid(), 'card_id', 'n' || i, 'kept', false, 'title', 'n' || i);
  END LOOP;
  UPDATE pick_examples SET created_at = now() - interval '1 day' WHERE helper_id = hlp;
  PERFORM add_pick_examples(rows);
  ASSERT (SELECT count(*) FROM pick_examples WHERE helper_id = hlp) = 200, 'capped at 200';
  ASSERT NOT EXISTS (SELECT 1 FROM pick_examples WHERE card_id = 'c1'), 'the oldest went first';

  res := forget_pick_examples(hlp);
  ASSERT res::int = 200, 'forgot: ' || res::text;
  ASSERT NOT EXISTS (SELECT 1 FROM pick_examples WHERE helper_id = hlp), 'nothing left';

  BEGIN PERFORM add_pick_examples('{}'); RAISE EXCEPTION 'non-array accepted';
  EXCEPTION WHEN invalid_parameter_value THEN NULL; END;
END $$;

SELECT pg_temp.act('authenticated', 'aaaaaaaa-0000-0000-0000-000000000001');
DO $$ BEGIN
  BEGIN PERFORM forget_pick_examples('aaaaaaaa-2222-0000-0000-000000000001'); RAISE EXCEPTION 'user called forget_pick_examples';
  EXCEPTION WHEN insufficient_privilege THEN NULL; END;
  BEGIN PERFORM add_pick_examples('[]'); RAISE EXCEPTION 'user called add_pick_examples';
  EXCEPTION WHEN insufficient_privilege THEN NULL; END;
END $$;
