-- Notifications on phones and desktops (Web Push): one row per browser that
-- turned them on. The worker and the api send a push when a Feed card
-- arrives. Service only: the api writes and reads these with the secret key.
--
-- user_id is the person, so a workspace owner's devices are found by their
-- id. Realtime may deliver a row to that same person: it holds their own
-- browser's public key and push address, nothing of anyone else's.
CREATE TABLE push_subscriptions (
  -- The id is derived from the endpoint (store.PushID), so saving the same
  -- browser twice hits the primary key instead of making a second row.
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  endpoint    text NOT NULL UNIQUE CHECK (endpoint ~ '^https://' AND length(endpoint) <= 1000),
  p256dh      text NOT NULL CHECK (length(p256dh) BETWEEN 40 AND 200), -- the browser's public key
  auth        text NOT NULL CHECK (length(auth) BETWEEN 16 AND 100),   -- its shared secret
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX push_subscriptions_user_idx ON push_subscriptions (user_id, created_at);

ALTER TABLE push_subscriptions ENABLE ROW LEVEL SECURITY;
CREATE POLICY service_all ON push_subscriptions USING (public.is_service_role()) WITH CHECK (public.is_service_role());
