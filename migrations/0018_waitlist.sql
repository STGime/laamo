-- The waiting list (marketing site): people who want an invite. Service only:
-- the api writes and reads it with the secret key; the browser never does.
-- Double opt-in: an entry counts once its confirmation link was used
-- (confirmed_at). Unconfirmed entries are deleted after 7 days.
CREATE TABLE waitlist (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  -- No owner: realtime must never deliver these rows to anyone.
  user_id       uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  email         text NOT NULL UNIQUE CHECK (email = lower(email) AND length(email) BETWEEN 3 AND 254),
  locale        text NOT NULL DEFAULT 'en' CHECK (locale IN ('en', 'de')),
  interests     jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(interests) = 'array'),
  source        text CHECK (length(source) <= 200),          -- page or campaign they came from
  ref_code      text NOT NULL UNIQUE CHECK (ref_code ~ '^[a-z0-9]{6,12}$'), -- their share code
  referred_by   text CHECK (referred_by ~ '^[a-z0-9]{6,12}$'),            -- the code they came with
  consent       text NOT NULL,                               -- version of the text they agreed to
  token_hash    text UNIQUE,                                 -- sha256 of the confirmation token; NULL once used
  token_sent_at timestamptz,
  confirmed_at  timestamptz,
  invited_at    timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  CHECK (user_id = '00000000-0000-0000-0000-000000000000')
);
CREATE INDEX waitlist_referred_by ON waitlist (referred_by) WHERE confirmed_at IS NOT NULL;

ALTER TABLE waitlist ENABLE ROW LEVEL SECURITY;
CREATE POLICY service_all ON waitlist USING (public.is_service_role()) WITH CHECK (public.is_service_role());

-- Daily cron (cron.json): deletes entries never confirmed within 7 days.
CREATE OR REPLACE FUNCTION purge_waitlist()
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  deleted int;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'purge_waitlist is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;
  DELETE FROM waitlist WHERE confirmed_at IS NULL AND created_at < now() - interval '7 days';
  GET DIAGNOSTICS deleted = ROW_COUNT;
  RETURN jsonb_build_object('deleted', deleted);
END $$;
