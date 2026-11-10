-- Templates, helpers, versions, triggers and inbound email.

-- templates: the Shelf. Public catalog data, readable by anyone.
-- Written only by the service. It has no owner column, so template changes
-- reach no end user over realtime; the app re-reads the Shelf instead.
CREATE TABLE templates (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug             text NOT NULL UNIQUE,
  name             text NOT NULL,
  promise          text NOT NULL,
  category         text NOT NULL,
  stack            jsonb NOT NULL,
  setup_questions  jsonb NOT NULL DEFAULT '[]'::jsonb,
  required         jsonb NOT NULL DEFAULT '{}'::jsonb, -- e.g. {"brain": true, "places": ["telegram"]}
  sort_order       int NOT NULL DEFAULT 0,
  published        boolean NOT NULL DEFAULT true,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER templates_updated_at BEFORE UPDATE ON templates
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE helpers (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id        uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name                text NOT NULL,
  avatar              text,
  color               text,
  status              text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'paused')),
  current_version_id  uuid, -- FK added below
  template_id         uuid REFERENCES templates(id) ON DELETE SET NULL,
  created_via         text NOT NULL DEFAULT 'scratch' CHECK (created_via IN ('shelf', 'chat', 'scratch')),
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX helpers_ws_idx ON helpers (workspace_id);
CREATE TRIGGER helpers_updated_at BEFORE UPDATE ON helpers
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- helper_versions: immutable snapshots of a stack
CREATE TABLE helper_versions (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id   uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  helper_id      uuid NOT NULL REFERENCES helpers(id) ON DELETE CASCADE,
  version        int NOT NULL CHECK (version > 0),
  stack          jsonb NOT NULL,
  source_prompt  text, -- Talk to build input, if any
  created_by     uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (helper_id, version)
);

ALTER TABLE helpers
  ADD CONSTRAINT helpers_current_version_fk
  FOREIGN KEY (current_version_id) REFERENCES helper_versions(id) ON DELETE SET NULL;

-- triggers: what starts a run
CREATE TABLE triggers (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id     uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  helper_id        uuid NOT NULL REFERENCES helpers(id) ON DELETE CASCADE,
  type             text NOT NULL CHECK (type IN ('schedule', 'manual', 'email', 'watch', 'webhook')),
  config           jsonb NOT NULL DEFAULT '{}'::jsonb,
  next_run_at      timestamptz,
  inbound_address  text UNIQUE, -- e.g. meetings.x7k@in.laamo.app
  webhook_token    text UNIQUE,
  last_hash        text, -- watch triggers (post-MVP)
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX triggers_helper_idx ON triggers (helper_id);
-- enqueue_due_triggers() scans this every minute
CREATE INDEX triggers_due_idx ON triggers (next_run_at) WHERE next_run_at IS NOT NULL;
CREATE TRIGGER triggers_updated_at BEFORE UPDATE ON triggers
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- inbound_emails: mail received by a helper address (untrusted content)
CREATE TABLE inbound_emails (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id         uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  helper_id            uuid NOT NULL REFERENCES helpers(id) ON DELETE CASCADE,
  message_id           text,
  from_addr            text NOT NULL,
  cc_addrs             text[] NOT NULL DEFAULT '{}', -- Lane 2: recipient must be from_addr or a CC
  subject              text,
  body_text            text,
  attachment_file_ids  uuid[] NOT NULL DEFAULT '{}',
  received_at          timestamptz NOT NULL DEFAULT now(),
  run_id               uuid -- FK added in 0003
);
CREATE INDEX inbound_emails_helper_idx ON inbound_emails (helper_id, received_at DESC);

ALTER TABLE calendar_events
  ADD CONSTRAINT calendar_events_helper_fk
  FOREIGN KEY (helper_id) REFERENCES helpers(id) ON DELETE SET NULL;

-- ---------------------------------------------------------------------------
-- RLS
ALTER TABLE templates       ENABLE ROW LEVEL SECURITY;
ALTER TABLE helpers         ENABLE ROW LEVEL SECURITY;
ALTER TABLE helper_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE triggers        ENABLE ROW LEVEL SECURITY;
ALTER TABLE inbound_emails  ENABLE ROW LEVEL SECURITY;

CREATE POLICY service_all ON templates       USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON helpers         USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON helper_versions USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON triggers        USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON inbound_emails  USING (public.is_service_role()) WITH CHECK (public.is_service_role());

CREATE POLICY public_read ON templates       FOR SELECT USING (published);
CREATE POLICY member_read ON helpers         FOR SELECT USING (is_workspace_member(workspace_id));
CREATE POLICY member_read ON helper_versions FOR SELECT USING (is_workspace_member(workspace_id));
CREATE POLICY member_read ON triggers        FOR SELECT USING (is_workspace_member(workspace_id));
CREATE POLICY member_read ON inbound_emails  FOR SELECT USING (is_workspace_member(workspace_id));
