-- Runs, steps, card piles, Feed, outbound messages, lists and files.

CREATE TABLE runs (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id        uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  helper_id           uuid NOT NULL REFERENCES helpers(id) ON DELETE CASCADE,
  version_id          uuid NOT NULL REFERENCES helper_versions(id),
  trigger_type        text NOT NULL CHECK (trigger_type IN ('schedule', 'manual', 'email', 'watch', 'webhook', 'test')),
  trigger_data        jsonb NOT NULL DEFAULT '{}'::jsonb, -- Tap Go answer, inbound email id, …
  test_mode           boolean NOT NULL DEFAULT false, -- max 3 cards, Do bricks rerouted to the user
  status              text NOT NULL DEFAULT 'queued'
                      CHECK (status IN ('queued', 'running', 'waiting', 'done', 'failed', 'stopped')),
  next_brick_index    int NOT NULL DEFAULT 0, -- where to resume after an Ask me brick
  waiting_on_card_id  uuid, -- FK added below
  started_at          timestamptz,
  finished_at         timestamptz,
  cost_eur            numeric(12, 6) NOT NULL DEFAULT 0,
  tokens_in           int NOT NULL DEFAULT 0,
  tokens_out          int NOT NULL DEFAULT 0,
  error               text,
  created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX runs_helper_idx ON runs (helper_id, created_at DESC);
CREATE INDEX runs_waiting_idx ON runs (status) WHERE status = 'waiting';

CREATE TABLE run_steps (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  run_id       uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  brick_id     text NOT NULL, -- id inside the stack, e.g. "b3"
  brick_type   text NOT NULL, -- catalog id, e.g. "look.read_page"
  position     int NOT NULL,
  attempt      int NOT NULL DEFAULT 1,
  status       text NOT NULL DEFAULT 'running'
               CHECK (status IN ('running', 'waiting', 'done', 'failed', 'skipped')),
  cards_in     int NOT NULL DEFAULT 0,
  cards_out    int NOT NULL DEFAULT 0,
  started_at   timestamptz NOT NULL DEFAULT now(),
  finished_at  timestamptz,
  cost_eur     numeric(12, 6) NOT NULL DEFAULT 0,
  log          jsonb NOT NULL DEFAULT '[]'::jsonb,
  UNIQUE (run_id, brick_id, attempt)
);

-- cards: the pile each step produced. card_id is the logical card identity,
-- kept across Enrich bricks; each step stores its own snapshot row.
CREATE TABLE cards (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id  uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  run_id        uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  step_id       uuid NOT NULL REFERENCES run_steps(id) ON DELETE CASCADE,
  card_id       uuid NOT NULL,
  parent_id     uuid, -- card it was derived from ("why" trail)
  position      int NOT NULL DEFAULT 0,
  kind          text NOT NULL, -- web_result, business, page, email, draft, event, summary…
  data          jsonb NOT NULL DEFAULT '{}'::jsonb, -- Fields
  trust         jsonb NOT NULL DEFAULT '{}'::jsonb, -- per field: user_entered | extracted | untrusted
  source_url    text,
  kept          boolean,
  score         numeric,
  reason        text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (step_id, card_id)
);
CREATE INDEX cards_run_idx ON cards (run_id);

CREATE TABLE feed_cards (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id  uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  helper_id     uuid REFERENCES helpers(id) ON DELETE CASCADE,
  run_id        uuid REFERENCES runs(id) ON DELETE CASCADE,
  step_id       uuid REFERENCES run_steps(id) ON DELETE SET NULL,
  type          text NOT NULL CHECK (type IN ('result', 'approval', 'pick', 'question', 'problem')),
  title         text NOT NULL,
  body          text,
  payload       jsonb NOT NULL DEFAULT '{}'::jsonb,
  state         text NOT NULL DEFAULT 'open'
                CHECK (state IN ('open', 'approved', 'edited', 'skipped', 'answered', 'expired', 'dismissed')),
  decision      jsonb, -- edits, picked card ids, answer
  expires_at    timestamptz,
  decided_by    uuid REFERENCES users(id) ON DELETE SET NULL,
  decided_at    timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX feed_cards_ws_idx ON feed_cards (workspace_id, created_at DESC);
CREATE INDEX feed_cards_open_idx ON feed_cards (workspace_id) WHERE state = 'open';
-- sweep: decided cards whose run is still waiting
CREATE INDEX feed_cards_decided_idx ON feed_cards (run_id) WHERE state <> 'open' AND run_id IS NOT NULL;

ALTER TABLE runs
  ADD CONSTRAINT runs_waiting_card_fk
  FOREIGN KEY (waiting_on_card_id) REFERENCES feed_cards(id) ON DELETE SET NULL;

-- outbound_messages: written BEFORE sending (audit trail, caps, idempotency)
CREATE TABLE outbound_messages (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id         uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  run_id               uuid REFERENCES runs(id) ON DELETE SET NULL,
  idempotency_key      text NOT NULL UNIQUE, -- run_id + brick_id + card_id
  channel              text NOT NULL CHECK (channel IN ('email', 'discord', 'slack', 'teams', 'telegram', 'posta')),
  lane                 smallint CHECK (lane IN (1, 2, 3)), -- email only
  recipient            text NOT NULL,
  subject              text,
  body                 text,
  media_file_ids       uuid[] NOT NULL DEFAULT '{}',
  scheduled_for        timestamptz,
  status               text NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'draft', 'sent', 'failed', 'blocked', 'bounced', 'complained')),
  provider_message_id  text,
  error                text,
  sent_at              timestamptz,
  created_at           timestamptz NOT NULL DEFAULT now(),
  CHECK (channel <> 'email' OR lane IS NOT NULL)
);
CREATE INDEX outbound_ws_time_idx ON outbound_messages (workspace_id, created_at DESC);

CREATE TABLE lists (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id  uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name          text NOT NULL,
  columns       jsonb NOT NULL DEFAULT '[]'::jsonb,
  dedupe_field  text,
  share_token   text UNIQUE, -- read-only share link; null = not shared
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, name)
);
CREATE TRIGGER lists_updated_at BEFORE UPDATE ON lists
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE list_rows (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id   uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  list_id        uuid NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
  data           jsonb NOT NULL DEFAULT '{}'::jsonb,
  dedupe_key     text,
  source_run_id  uuid REFERENCES runs(id) ON DELETE SET NULL,
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX list_rows_list_idx ON list_rows (list_id, created_at DESC);
CREATE UNIQUE INDEX list_rows_dedupe_idx ON list_rows (list_id, dedupe_key) WHERE dedupe_key IS NOT NULL;

-- files: metadata for Eurobase storage objects
CREATE TABLE files (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id     uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  storage_path     text NOT NULL UNIQUE,
  name             text,
  mime             text,
  size_bytes       bigint,
  parsed_text_ref  text, -- storage path of extracted text
  created_at       timestamptz NOT NULL DEFAULT now()
);

-- deferred FKs from 0001 / 0002
ALTER TABLE usage_ledger
  ADD CONSTRAINT usage_ledger_run_fk FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE SET NULL;
ALTER TABLE calendar_events
  ADD CONSTRAINT calendar_events_run_fk FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE SET NULL;
ALTER TABLE inbound_emails
  ADD CONSTRAINT inbound_emails_run_fk FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE SET NULL;

-- ---------------------------------------------------------------------------
-- RLS
ALTER TABLE runs              ENABLE ROW LEVEL SECURITY;
ALTER TABLE run_steps         ENABLE ROW LEVEL SECURITY;
ALTER TABLE cards             ENABLE ROW LEVEL SECURITY;
ALTER TABLE feed_cards        ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbound_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE lists             ENABLE ROW LEVEL SECURITY;
ALTER TABLE list_rows         ENABLE ROW LEVEL SECURITY;
ALTER TABLE files             ENABLE ROW LEVEL SECURITY;

CREATE POLICY service_all ON runs              USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON run_steps         USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON cards             USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON feed_cards        USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON outbound_messages USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON lists             USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON list_rows         USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON files             USING (public.is_service_role()) WITH CHECK (public.is_service_role());

CREATE POLICY member_read ON runs              FOR SELECT USING (is_workspace_member(workspace_id));
CREATE POLICY member_read ON run_steps         FOR SELECT USING (is_workspace_member(workspace_id));
CREATE POLICY member_read ON cards             FOR SELECT USING (is_workspace_member(workspace_id));
CREATE POLICY member_read ON feed_cards        FOR SELECT USING (is_workspace_member(workspace_id));
CREATE POLICY member_read ON outbound_messages FOR SELECT USING (is_workspace_member(workspace_id));
CREATE POLICY member_read ON lists             FOR SELECT USING (is_workspace_member(workspace_id));
CREATE POLICY member_read ON list_rows         FOR SELECT USING (is_workspace_member(workspace_id));
CREATE POLICY member_read ON files             FOR SELECT USING (is_workspace_member(workspace_id));
