-- Only new rows (#50): Read my list remembers which rows a helper has used,
-- so a lead never gets two drafts and a digest never repeats. A row is used
-- by the run that read it; a retry of that run gets the same rows again.
-- Test runs read without marking.
CREATE TABLE list_reads (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id  uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  helper_id     uuid NOT NULL REFERENCES helpers(id) ON DELETE CASCADE,
  row_id        uuid NOT NULL REFERENCES list_rows(id) ON DELETE CASCADE,
  run_id        uuid NOT NULL, -- no FK: cleaning up old runs mustn't make rows new again
  created_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (helper_id, row_id)
);
CREATE INDEX list_reads_run_idx ON list_reads (run_id);

ALTER TABLE list_reads ENABLE ROW LEVEL SECURITY;
CREATE POLICY service_all ON list_reads USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY member_read ON list_reads FOR SELECT USING (is_workspace_member(workspace_id));

-- take_list_rows: up to p_limit rows of a list this helper hasn't used,
-- oldest first, and (with p_mark) marks them used by p_run. Rows this run
-- already took come back first, so a retried job sees the same pile.
-- Returns the rows as a jsonb array.
CREATE FUNCTION take_list_rows(p_list uuid, p_helper uuid, p_run uuid, p_limit int, p_mark boolean)
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  picked jsonb;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'take_list_rows is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;
  IF p_limit IS NULL OR p_limit < 1 OR p_limit > 500 THEN
    RAISE EXCEPTION 'limit must be 1 to 500' USING ERRCODE = 'invalid_parameter_value';
  END IF;

  WITH mine AS (
    SELECT r.* FROM list_rows r
      JOIN list_reads x ON x.row_id = r.id AND x.helper_id = p_helper AND x.run_id = p_run
     WHERE r.list_id = p_list
  ), fresh AS (
    SELECT r.* FROM list_rows r
     WHERE r.list_id = p_list
       AND NOT EXISTS (SELECT 1 FROM list_reads x WHERE x.row_id = r.id AND x.helper_id = p_helper)
     ORDER BY r.created_at, r.id
     LIMIT greatest(p_limit - (SELECT count(*) FROM mine), 0)
     FOR UPDATE OF r SKIP LOCKED
  ), picked_rows AS (
    SELECT * FROM mine UNION ALL SELECT * FROM fresh
  ), marked AS (
    INSERT INTO list_reads (workspace_id, user_id, helper_id, row_id, run_id)
    SELECT f.workspace_id, f.user_id, p_helper, f.id, p_run FROM fresh f WHERE p_mark
    ON CONFLICT (helper_id, row_id) DO NOTHING
    RETURNING row_id
  )
  SELECT coalesce(jsonb_agg(to_jsonb(p) ORDER BY p.created_at, p.id), '[]'::jsonb)
    INTO picked FROM picked_rows p;
  RETURN picked;
END;
$$;
