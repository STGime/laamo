-- Learn from swipes (#76). When "Learn from my choices" is on, every card
-- the user kept or skipped on a Let me pick card is remembered here, per
-- helper: a short copy of what the card said, and the user's choice. Keep
-- the good ones shows the newest ones to the AI as labelled examples.
-- Forget deletes them all. A helper keeps its newest 200.
CREATE TABLE pick_examples (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id  uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  helper_id     uuid NOT NULL REFERENCES helpers(id) ON DELETE CASCADE,
  -- where the choice was made; no FK, so cleaning up old runs keeps what was learned
  step_id       uuid NOT NULL,
  card_id       text NOT NULL,
  kept          boolean NOT NULL,
  title         text NOT NULL DEFAULT '' CHECK (length(title) <= 300),
  site          text NOT NULL DEFAULT '' CHECK (length(site) <= 200),
  text          text NOT NULL DEFAULT '' CHECK (length(text) <= 600),
  created_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (step_id, card_id)
);
CREATE INDEX pick_examples_helper_idx ON pick_examples (helper_id, kept, created_at DESC);

ALTER TABLE pick_examples ENABLE ROW LEVEL SECURITY;
CREATE POLICY service_all ON pick_examples USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY member_read ON pick_examples FOR SELECT USING (is_workspace_member(workspace_id));

-- add_pick_examples: records a Let me pick decision. Rows already recorded
-- (same step and card, e.g. a retried job) are skipped. Afterwards each
-- helper keeps its newest 200. Returns the number of new rows.
CREATE FUNCTION add_pick_examples(rows jsonb)
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  added int;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'add_pick_examples is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;
  IF rows IS NULL OR jsonb_typeof(rows) <> 'array' THEN
    RAISE EXCEPTION 'rows must be a jsonb array' USING ERRCODE = 'invalid_parameter_value';
  END IF;

  WITH input AS (
    SELECT r.*
      FROM jsonb_array_elements(rows) AS e(elem),
           jsonb_to_record(e.elem) AS r(
             id uuid, workspace_id uuid, user_id uuid, helper_id uuid, step_id uuid,
             card_id text, kept boolean, title text, site text, text text)
  ), inserted AS (
    INSERT INTO pick_examples (id, workspace_id, user_id, helper_id, step_id, card_id, kept, title, site, text)
    SELECT coalesce(i.id, gen_random_uuid()), i.workspace_id, i.user_id, i.helper_id, i.step_id,
           i.card_id, i.kept, left(coalesce(i.title, ''), 300), left(coalesce(i.site, ''), 200),
           left(coalesce(i.text, ''), 600)
      FROM input i
    ON CONFLICT (step_id, card_id) DO NOTHING
    RETURNING helper_id
  )
  SELECT count(*) INTO added FROM inserted;

  DELETE FROM pick_examples p
   USING (
     SELECT id, row_number() OVER (PARTITION BY helper_id ORDER BY created_at DESC, id DESC) AS n
       FROM pick_examples
      WHERE helper_id IN (SELECT DISTINCT (e.elem->>'helper_id')::uuid FROM jsonb_array_elements(rows) AS e(elem))
   ) old
   WHERE p.id = old.id AND old.n > 200;

  RETURN to_jsonb(added);
END;
$$;

-- forget_pick_examples: deletes everything a helper learned from swipes.
-- Returns the number of deleted rows.
CREATE FUNCTION forget_pick_examples(helper uuid)
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  gone int;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'forget_pick_examples is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;
  DELETE FROM pick_examples WHERE helper_id = helper;
  GET DIAGNOSTICS gone = ROW_COUNT;
  RETURN to_jsonb(gone);
END;
$$;
