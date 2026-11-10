-- Accounts, billing, brain and places.
--
-- Conventions for every Laamo table (see migrations/README.md):
--   * id uuid PK (Eurobase REST is wired to a column named id)
--   * workspace_id for RLS by membership
--   * user_id NOT NULL = workspace owner. Eurobase realtime filters events by
--     this column; a row without it reaches no end user over realtime.
--   * Policies: members may SELECT; all writes go through the Go services
--     with the secret key (public.is_service_role()).

CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;

-- ---------------------------------------------------------------------------
-- workspaces
CREATE TABLE workspaces (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, -- owner
  name        text NOT NULL,
  timezone    text NOT NULL DEFAULT 'Europe/Berlin',
  locale      text NOT NULL DEFAULT 'en' CHECK (locale IN ('en', 'de')),
  plan        text NOT NULL DEFAULT 'free' CHECK (plan IN ('free', 'starter', 'business')),
  profile     jsonb NOT NULL DEFAULT '{}'::jsonb, -- About me facts: city, industry, offer, tone…
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX workspaces_user_idx ON workspaces (user_id);
CREATE TRIGGER workspaces_updated_at BEFORE UPDATE ON workspaces
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- members (user_id here is the member itself)
CREATE TABLE members (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id  uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role          text NOT NULL DEFAULT 'owner' CHECK (role IN ('owner', 'member')),
  display_name  text,
  about_me      text,
  signature     text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, user_id)
);
CREATE INDEX members_user_idx ON members (user_id);

-- Is the signed-in end user a member of workspace ws?
-- Invoker function: the members SELECT policy only exposes the caller's own rows,
-- which is exactly what this check needs (no recursion).
CREATE FUNCTION is_workspace_member(ws uuid) RETURNS boolean
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (
    SELECT 1 FROM members m
     WHERE m.workspace_id = ws
       AND m.user_id = auth_uid()
  );
$$;

-- subscriptions (Mollie)
CREATE TABLE subscriptions (
  id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id            uuid NOT NULL UNIQUE REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id                 uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  mollie_customer_id      text,
  mollie_subscription_id  text,
  plan                    text NOT NULL DEFAULT 'free' CHECK (plan IN ('free', 'starter', 'business')),
  status                  text NOT NULL DEFAULT 'none'
                          CHECK (status IN ('none', 'pending', 'active', 'past_due', 'canceled')),
  current_period_end      timestamptz,
  created_at              timestamptz NOT NULL DEFAULT now(),
  updated_at              timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER subscriptions_updated_at BEFORE UPDATE ON subscriptions
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- usage_ledger: every LLM / search / fetch / email / places call
CREATE TABLE usage_ledger (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id   uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  run_id         uuid, -- FK added in 0003
  kind           text NOT NULL CHECK (kind IN ('llm', 'search', 'fetch', 'email', 'places')),
  brain_mode     text CHECK (brain_mode IN ('included', 'byok')),
  model          text,
  units          numeric NOT NULL DEFAULT 0,
  brain_credits  numeric NOT NULL DEFAULT 0,
  est_cost_eur   numeric(12, 6) NOT NULL DEFAULT 0,
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX usage_ledger_ws_time_idx ON usage_ledger (workspace_id, created_at DESC);

-- brains: included model or (post-MVP) the user's own key
CREATE TABLE brains (
  id                        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id              uuid NOT NULL UNIQUE REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id                   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  mode                      text NOT NULL DEFAULT 'included' CHECK (mode IN ('included', 'byok')),
  provider                  text,
  model                     text,
  key_ciphertext            bytea, -- envelope-encrypted; null when included
  key_last4                 text,
  status                    text NOT NULL DEFAULT 'ok'
                            CHECK (status IN ('ok', 'invalid', 'over_budget', 'allowance_used')),
  allowance_credits         numeric NOT NULL DEFAULT 0,
  used_credits_this_period  numeric NOT NULL DEFAULT 0,
  period_start              timestamptz NOT NULL DEFAULT date_trunc('month', now()),
  monthly_cap_eur           numeric(10, 2), -- byok only
  spent_this_month_eur      numeric(12, 6) NOT NULL DEFAULT 0,
  created_at                timestamptz NOT NULL DEFAULT now(),
  updated_at                timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER brains_updated_at BEFORE UPDATE ON brains
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- places: where helpers tell you things
CREATE TABLE places (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id       uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id            uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type               text NOT NULL CHECK (type IN ('discord', 'slack', 'teams', 'telegram', 'email', 'posta')),
  label              text NOT NULL,
  secret_ciphertext  bytea, -- webhook URL, chat id or API key (envelope-encrypted)
  secret_last4       text,
  meta               jsonb NOT NULL DEFAULT '{}'::jsonb,
  verified_at        timestamptz,
  created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX places_ws_idx ON places (workspace_id);

-- calendar
CREATE TABLE calendar_feeds (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id  uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token         text NOT NULL UNIQUE, -- unguessable
  created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE calendar_events (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id    uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  helper_id       uuid, -- FK added in 0002
  run_id          uuid, -- FK added in 0003
  uid             text NOT NULL UNIQUE, -- iCalendar UID
  title           text NOT NULL,
  starts_at       timestamptz NOT NULL,
  ends_at         timestamptz NOT NULL,
  location        text,
  description     text,
  attendees       jsonb NOT NULL DEFAULT '[]'::jsonb,
  invite_sent_at  timestamptz,
  status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'canceled', 'failed')),
  created_at      timestamptz NOT NULL DEFAULT now(),
  CHECK (ends_at >= starts_at)
);
CREATE INDEX calendar_events_ws_idx ON calendar_events (workspace_id, starts_at);

-- ---------------------------------------------------------------------------
-- RLS
ALTER TABLE workspaces      ENABLE ROW LEVEL SECURITY;
ALTER TABLE members         ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscriptions   ENABLE ROW LEVEL SECURITY;
ALTER TABLE usage_ledger    ENABLE ROW LEVEL SECURITY;
ALTER TABLE brains          ENABLE ROW LEVEL SECURITY;
ALTER TABLE places          ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_feeds  ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_events ENABLE ROW LEVEL SECURITY;

-- Service (Go api/worker with the secret key) can do everything.
CREATE POLICY service_all ON workspaces      USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON members         USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON subscriptions   USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON usage_ledger    USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON brains          USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON places          USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON calendar_feeds  USING (public.is_service_role()) WITH CHECK (public.is_service_role());
CREATE POLICY service_all ON calendar_events USING (public.is_service_role()) WITH CHECK (public.is_service_role());

-- Members can read. brains, places and calendar_feeds hold secrets/tokens:
-- no end-user access at all; the API returns safe fields (e.g. last4).
CREATE POLICY member_read ON workspaces      FOR SELECT USING (is_workspace_member(id));
CREATE POLICY member_read ON members         FOR SELECT USING (user_id = auth_uid());
CREATE POLICY member_read ON subscriptions   FOR SELECT USING (is_workspace_member(workspace_id));
CREATE POLICY member_read ON usage_ledger    FOR SELECT USING (is_workspace_member(workspace_id));
CREATE POLICY member_read ON calendar_events FOR SELECT USING (is_workspace_member(workspace_id));
