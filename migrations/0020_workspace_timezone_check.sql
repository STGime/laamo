-- create_workspace: check the time zone without pg_timezone_names.
--
-- Eurobase is closing the system catalogs to project SQL (Free and Pro), so
-- the view 0008 looked the name up in won't be readable anymore. Now the
-- name has to look like an IANA zone ("Europe/Berlin",
-- "America/Argentina/Buenos_Aires") or be "UTC", and PostgreSQL has to know
-- it: AT TIME ZONE raises invalid_parameter_value for a name it doesn't.
--
-- The pattern comes first because AT TIME ZONE alone would also take
-- abbreviations and POSIX strings ("CET", "EST5EDT", "UTC+3"). Those were
-- partly accepted before (pg_timezone_names lists CET); they're refused
-- now: a workspace's zone is a place, so schedules follow its summer time.
-- The app sends the browser's zone, which is a place (see timezone.ts for
-- the few browsers that report something else).
--
-- Everything else is 0008 unchanged.
--
-- Errors (mapped by Eurobase RPC to 400 with the SQLSTATE):
--   22023 invalid_parameter_value  bad name, timezone or locale
--   23503 foreign_key_violation    owner is not a user of this project
--   23514 check_violation          owner already has 5 workspaces
CREATE OR REPLACE FUNCTION create_workspace(owner uuid, name text, timezone text DEFAULT 'Europe/Berlin', locale text DEFAULT 'en')
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  ws workspaces;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'create_workspace is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;
  name := btrim(coalesce(name, ''));
  IF length(name) < 1 OR length(name) > 80 THEN
    RAISE EXCEPTION 'name must be 1 to 80 characters' USING ERRCODE = 'invalid_parameter_value';
  END IF;
  IF locale IS NULL OR locale NOT IN ('en', 'de') THEN
    RAISE EXCEPTION 'locale must be en or de' USING ERRCODE = 'invalid_parameter_value';
  END IF;
  IF timezone IS NULL OR timezone !~ '^(UTC|[A-Za-z_]+(/[A-Za-z0-9_+-]+)+)$' THEN
    RAISE EXCEPTION 'unknown timezone %', timezone USING ERRCODE = 'invalid_parameter_value';
  END IF;
  BEGIN
    PERFORM now() AT TIME ZONE timezone;
  EXCEPTION WHEN invalid_parameter_value THEN
    RAISE EXCEPTION 'unknown timezone %', timezone USING ERRCODE = 'invalid_parameter_value';
  END;
  IF (SELECT count(*) FROM workspaces w WHERE w.user_id = owner) >= 5 THEN
    RAISE EXCEPTION 'you already have 5 workspaces' USING ERRCODE = 'check_violation';
  END IF;

  INSERT INTO workspaces (user_id, name, timezone, locale)
  VALUES (owner, name, timezone, locale)
  RETURNING * INTO ws;

  INSERT INTO members (workspace_id, user_id, role) VALUES (ws.id, owner, 'owner');
  INSERT INTO brains (workspace_id, user_id, mode) VALUES (ws.id, owner, 'included');

  RETURN to_jsonb(ws);
END;
$$;
