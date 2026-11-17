-- Tests for the waiting list (0018). Runs as tenant_func after rls_test.sql.
\set ON_ERROR_STOP on
\o /dev/null
SET search_path = tenant_test, public;

CREATE FUNCTION pg_temp.act(role text, uid uuid) RETURNS void LANGUAGE sql AS $$
  SELECT set_config('app.end_user_role', role, false),
         set_config('app.end_user_id', coalesce(uid::text, ''), false);
$$;
SELECT pg_temp.act('service', NULL);

INSERT INTO waitlist (email, ref_code, consent, token_hash, created_at) VALUES
  ('new@example.com', 'aaaaaa01', 'waitlist-v1', 'h1', now()),
  ('old@example.com', 'aaaaaa02', 'waitlist-v1', 'h2', now() - interval '8 days');
INSERT INTO waitlist (email, ref_code, consent, confirmed_at, created_at) VALUES
  ('kept@example.com', 'aaaaaa03', 'waitlist-v1', now() - interval '8 days', now() - interval '9 days');

-- Signed-in users see and write nothing.
SELECT pg_temp.act('authenticated', 'aaaaaaaa-0000-0000-0000-000000000001');
DO $$
BEGIN
  ASSERT (SELECT count(*) FROM waitlist) = 0, 'the waiting list is service only';
  BEGIN
    INSERT INTO waitlist (email, ref_code, consent) VALUES ('x@example.com', 'aaaaaa09', 'v1');
    RAISE EXCEPTION 'user could add to the waiting list';
  EXCEPTION WHEN insufficient_privilege THEN NULL;
  END;
  BEGIN
    PERFORM purge_waitlist();
    RAISE EXCEPTION 'user could purge the waiting list';
  EXCEPTION WHEN insufficient_privilege THEN NULL;
  END;
END $$;

-- Purge: only unconfirmed entries older than 7 days go.
SELECT pg_temp.act('service', NULL);
DO $$
BEGIN
  ASSERT (purge_waitlist() ->> 'deleted')::int = 1, 'one stale entry purged';
  ASSERT (SELECT array_agg(email ORDER BY email) FROM waitlist) = ARRAY['kept@example.com', 'new@example.com'],
    'confirmed and fresh entries stay';
  BEGIN
    INSERT INTO waitlist (email, ref_code, consent) VALUES ('Upper@Example.com', 'aaaaaa05', 'v1');
    RAISE EXCEPTION 'mixed-case email accepted';
  EXCEPTION WHEN check_violation THEN NULL;
  END;
END $$;
DELETE FROM waitlist;

\o
\echo 'waitlist tests passed'
