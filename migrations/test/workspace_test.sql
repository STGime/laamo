-- Tests for create_workspace (0008, time zone check from 0020). Runs as tenant_func.
\set ON_ERROR_STOP on
\o /dev/null
SET search_path = tenant_test, public;

CREATE FUNCTION pg_temp.act(role text, uid uuid) RETURNS void LANGUAGE sql AS $$
  SELECT set_config('app.end_user_role', role, false),
         set_config('app.end_user_id', coalesce(uid::text, ''), false);
$$;
SELECT pg_temp.act('service', NULL);

DO $$
DECLARE ws jsonb; wid uuid; owner uuid := 'aaaaaaaa-0000-0000-0000-000000000001';
BEGIN
  ws := create_workspace(owner, '  Bäckerei Krause  ', 'Europe/Vienna', 'de');
  wid := (ws->>'id')::uuid;
  ASSERT ws->>'name' = 'Bäckerei Krause', 'name trimmed: ' || ws::text;
  ASSERT ws->>'timezone' = 'Europe/Vienna' AND ws->>'locale' = 'de' AND ws->>'plan' = 'free', 'fields: ' || ws::text;
  ASSERT (SELECT role FROM members WHERE workspace_id = wid AND user_id = owner) = 'owner', 'owner member';
  ASSERT (SELECT mode FROM brains WHERE workspace_id = wid) = 'included', 'included brain';

  -- defaults
  ws := create_workspace(owner, 'Second');
  ASSERT ws->>'timezone' = 'Europe/Berlin' AND ws->>'locale' = 'en', 'defaults: ' || ws::text;

  BEGIN PERFORM create_workspace(owner, '', 'UTC', 'en'); RAISE EXCEPTION 'empty name accepted';
  EXCEPTION WHEN invalid_parameter_value THEN NULL; END;
  BEGIN PERFORM create_workspace(owner, 'x', 'Mars/Olympus', 'en'); RAISE EXCEPTION 'bad timezone accepted';
  EXCEPTION WHEN invalid_parameter_value THEN NULL; END;
  BEGIN PERFORM create_workspace(owner, 'x', 'UTC', 'fr'); RAISE EXCEPTION 'bad locale accepted';
  EXCEPTION WHEN invalid_parameter_value THEN NULL; END;
  BEGIN PERFORM create_workspace('cccccccc-0000-0000-0000-000000000009', 'x'); RAISE EXCEPTION 'unknown owner accepted';
  EXCEPTION WHEN foreign_key_violation THEN NULL; END;

  -- A failure leaves nothing behind (one transaction).
  ASSERT (SELECT count(*) FROM workspaces WHERE name = 'x') = 0, 'no partial workspace';

  -- At most 5 per owner (rls_test already gave this owner one).
  WHILE (SELECT count(*) FROM workspaces WHERE user_id = owner) < 5 LOOP
    PERFORM create_workspace(owner, 'filler');
  END LOOP;
  BEGIN PERFORM create_workspace(owner, 'sixth'); RAISE EXCEPTION 'sixth workspace accepted';
  EXCEPTION WHEN check_violation THEN NULL; END;
END $$;

SELECT pg_temp.act('authenticated', 'aaaaaaaa-0000-0000-0000-000000000001');
DO $$ BEGIN
  BEGIN PERFORM create_workspace('aaaaaaaa-0000-0000-0000-000000000001', 'mine'); RAISE EXCEPTION 'user called create_workspace';
  EXCEPTION WHEN insufficient_privilege THEN NULL; END;
END $$;

-- needs (0012): known values, at most 3.
SELECT pg_temp.act('service', NULL);
DO $$
DECLARE wid uuid;
BEGIN
  wid := (create_workspace('bbbbbbbb-0000-0000-0000-000000000002', 'Needs test')->>'id')::uuid;
  ASSERT (SELECT needs FROM workspaces WHERE id = wid) = '{}'::text[], 'needs default to empty';
  UPDATE workspaces SET needs = ARRAY['find_customers', 'plan_events'] WHERE id = wid;
  BEGIN
    UPDATE workspaces SET needs = ARRAY['world_domination'] WHERE id = wid;
    RAISE EXCEPTION 'unknown need accepted';
  EXCEPTION WHEN check_violation THEN NULL;
  END;
  BEGIN
    UPDATE workspaces SET needs = ARRAY['find_customers', 'plan_events', 'sort_emails', 'book_meetings'] WHERE id = wid;
    RAISE EXCEPTION '4 needs accepted';
  EXCEPTION WHEN check_violation THEN NULL;
  END;
  DELETE FROM workspaces WHERE id = wid;
END $$;

\echo 'Workspace tests passed'

-- Time zones (0020): a place PostgreSQL knows, or UTC. Checked without the
-- system catalogs, which Eurobase closes to project SQL.
SELECT pg_temp.act('service', NULL);
DO $$
DECLARE
  wid uuid; tz text; owner uuid := 'bbbbbbbb-0000-0000-0000-000000000002';
BEGIN
  FOREACH tz IN ARRAY ARRAY['Europe/Berlin', 'America/Argentina/Buenos_Aires', 'UTC', 'Etc/GMT+3', 'Asia/Ho_Chi_Minh'] LOOP
    wid := (create_workspace(owner, 'tz ok', tz)->>'id')::uuid;
    ASSERT (SELECT timezone FROM workspaces WHERE id = wid) = tz, 'kept ' || tz;
    DELETE FROM workspaces WHERE id = wid; -- stay under the 5 per owner
  END LOOP;

  -- Refused: places that don't exist, abbreviations and POSIX strings (AT
  -- TIME ZONE alone would take those), and anything that isn't a name.
  FOREACH tz IN ARRAY ARRAY['Mars/Base', 'Europe/Atlantis', 'CET', 'EST5EDT', 'UTC+3', 'utc', 'GMT', 'Europe/', '/Berlin',
                            'Europe/Berlin; DROP TABLE workspaces', 'Europe/Ber lin', '+02:00', ''] LOOP
    BEGIN
      PERFORM create_workspace(owner, 'tz bad', tz);
      RAISE EXCEPTION 'time zone % accepted', tz USING ERRCODE = 'assert_failure';
    EXCEPTION WHEN invalid_parameter_value THEN NULL; END;
  END LOOP;
  BEGIN
    PERFORM create_workspace(owner, 'tz null', NULL);
    RAISE EXCEPTION 'NULL time zone accepted' USING ERRCODE = 'assert_failure';
  EXCEPTION WHEN invalid_parameter_value THEN NULL; END;
  ASSERT (SELECT count(*) FROM workspaces WHERE name LIKE 'tz %') = 0, 'nothing left behind';
END $$;

-- The function itself no longer reads a system catalog.
DO $$ BEGIN
  ASSERT (SELECT count(*) FROM pg_proc p WHERE p.proname = 'create_workspace' AND p.prosrc ~* 'pg_timezone|pg_catalog|information_schema') = 0,
    'create_workspace still reads a system catalog';
END $$;
