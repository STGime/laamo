-- create_workspace: a new workspace with its owner membership and the
-- included brain, in one transaction (issue #13). Called by the API with the
-- secret key after it has verified the user's token.
--
-- Errors (mapped by Eurobase RPC to 400 with the SQLSTATE):
--   22023 invalid_parameter_value  bad name, timezone or locale
--   23503 foreign_key_violation    owner is not a user of this project
--   23514 check_violation          owner already has 5 workspaces
CREATE FUNCTION create_workspace(owner uuid, name text, timezone text DEFAULT 'Europe/Berlin', locale text DEFAULT 'en')
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
  IF timezone IS NULL OR NOT EXISTS (SELECT 1 FROM pg_timezone_names WHERE pg_timezone_names.name = timezone) THEN
    RAISE EXCEPTION 'unknown timezone %', timezone USING ERRCODE = 'invalid_parameter_value';
  END IF;
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
