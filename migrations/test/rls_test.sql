-- RLS tests. Runs as tenant_func (non-owner, like Eurobase API traffic).
-- Fails with an ASSERT error on the first broken expectation.
\set ON_ERROR_STOP on
\o /dev/null
SET search_path = tenant_test, public;

CREATE FUNCTION pg_temp.act(role text, uid uuid) RETURNS void LANGUAGE sql AS $$
  SELECT set_config('app.end_user_role', role, false),
         set_config('app.end_user_id', coalesce(uid::text, ''), false);
$$;

-- Fixed ids
\set a '\'aaaaaaaa-0000-0000-0000-000000000001\''
\set b '\'bbbbbbbb-0000-0000-0000-000000000002\''
\set wa '\'aaaaaaaa-1111-0000-0000-000000000001\''
\set wb '\'bbbbbbbb-1111-0000-0000-000000000002\''

-- ---- service seeds two workspaces --------------------------------------
SELECT pg_temp.act('service', NULL);
INSERT INTO users (id, email) VALUES (:a, 'a@example.com'), (:b, 'b@example.com');
INSERT INTO workspaces (id, user_id, name) VALUES (:wa, :a, 'A'), (:wb, :b, 'B');
INSERT INTO members (workspace_id, user_id) VALUES (:wa, :a), (:wb, :b);
INSERT INTO templates (slug, name, promise, category, stack)
  VALUES ('morning-briefing', 'Morning Briefing', '5 things', 'news', '{"bricks":[]}');
INSERT INTO helpers (id, workspace_id, user_id, name) VALUES
  ('aaaaaaaa-2222-0000-0000-000000000001', :wa, :a, 'A helper'),
  ('bbbbbbbb-2222-0000-0000-000000000002', :wb, :b, 'B helper');
INSERT INTO helper_versions (id, workspace_id, user_id, helper_id, version, stack) VALUES
  ('aaaaaaaa-3333-0000-0000-000000000001', :wa, :a, 'aaaaaaaa-2222-0000-0000-000000000001', 1, '{}'),
  ('bbbbbbbb-3333-0000-0000-000000000002', :wb, :b, 'bbbbbbbb-2222-0000-0000-000000000002', 1, '{}');
INSERT INTO runs (id, workspace_id, user_id, helper_id, version_id, trigger_type) VALUES
  ('aaaaaaaa-4444-0000-0000-000000000001', :wa, :a, 'aaaaaaaa-2222-0000-0000-000000000001', 'aaaaaaaa-3333-0000-0000-000000000001', 'manual'),
  ('bbbbbbbb-4444-0000-0000-000000000002', :wb, :b, 'bbbbbbbb-2222-0000-0000-000000000002', 'bbbbbbbb-3333-0000-0000-000000000002', 'manual');
INSERT INTO feed_cards (workspace_id, user_id, run_id, type, title) VALUES
  (:wa, :a, 'aaaaaaaa-4444-0000-0000-000000000001', 'approval', 'A card'),
  (:wb, :b, 'bbbbbbbb-4444-0000-0000-000000000002', 'approval', 'B card');
INSERT INTO brains (workspace_id, user_id, key_ciphertext) VALUES (:wa, :a, '\x01');
INSERT INTO places (workspace_id, user_id, type, label, secret_ciphertext) VALUES (:wa, :a, 'slack', 'Team', '\x02');
INSERT INTO calendar_feeds (workspace_id, user_id, token) VALUES (:wa, :a, 'secret-token');
INSERT INTO jobs (workspace_id, user_id, kind) VALUES (:wa, :a, 'run');
INSERT INTO jobs (kind) VALUES ('send_digest');

DO $$ BEGIN
  ASSERT (SELECT count(*) FROM workspaces) = 2, 'service should see all workspaces';
  ASSERT (SELECT count(*) FROM jobs) = 2, 'service should see all jobs';
END $$;

-- ---- user A -------------------------------------------------------------
SELECT pg_temp.act('authenticated', :a);
DO $$ BEGIN
  ASSERT (SELECT count(*) FROM workspaces) = 1, 'A sees only own workspace';
  ASSERT (SELECT name FROM workspaces) = 'A', 'A sees workspace A';
  ASSERT (SELECT count(*) FROM members) = 1, 'A sees only own membership';
  ASSERT (SELECT count(*) FROM helpers) = 1, 'A sees only own helpers';
  ASSERT (SELECT count(*) FROM runs) = 1, 'A sees only own runs';
  ASSERT (SELECT count(*) FROM feed_cards) = 1, 'A sees only own feed cards';
  ASSERT (SELECT title FROM feed_cards) = 'A card', 'A sees card A';
  ASSERT (SELECT count(*) FROM templates) = 1, 'A sees published templates';
  ASSERT (SELECT count(*) FROM brains) = 0, 'brains are service only';
  ASSERT (SELECT count(*) FROM places) = 0, 'places are service only';
  ASSERT (SELECT count(*) FROM calendar_feeds) = 0, 'calendar feeds are service only';
  ASSERT (SELECT count(*) FROM jobs) = 0, 'jobs are service only';
END $$;

-- A cannot write: updates silently match 0 rows, inserts are rejected.
DO $$
DECLARE n int;
BEGIN
  UPDATE feed_cards SET state = 'approved'; GET DIAGNOSTICS n = ROW_COUNT;
  ASSERT n = 0, 'A must not update feed cards directly';
  UPDATE workspaces SET plan = 'business'; GET DIAGNOSTICS n = ROW_COUNT;
  ASSERT n = 0, 'A must not change the plan';
  DELETE FROM runs; GET DIAGNOSTICS n = ROW_COUNT;
  ASSERT n = 0, 'A must not delete runs';
  BEGIN
    INSERT INTO feed_cards (workspace_id, user_id, type, title)
      VALUES ('bbbbbbbb-1111-0000-0000-000000000002', 'bbbbbbbb-0000-0000-0000-000000000002', 'result', 'spoof');
    RAISE EXCEPTION 'A inserted a feed card into workspace B';
  EXCEPTION WHEN insufficient_privilege THEN NULL; -- expected: RLS violation
  END;
  BEGIN
    INSERT INTO members (workspace_id, user_id)
      VALUES ('bbbbbbbb-1111-0000-0000-000000000002', 'aaaaaaaa-0000-0000-0000-000000000001');
    RAISE EXCEPTION 'A joined workspace B';
  EXCEPTION WHEN insufficient_privilege THEN NULL;
  END;
END $$;

-- ---- user B -------------------------------------------------------------
SELECT pg_temp.act('authenticated', :b);
DO $$ BEGIN
  ASSERT (SELECT count(*) FROM feed_cards) = 1, 'B sees only own feed cards';
  ASSERT (SELECT title FROM feed_cards) = 'B card', 'B sees card B';
  ASSERT (SELECT count(*) FROM runs WHERE workspace_id = 'aaaaaaaa-1111-0000-0000-000000000001') = 0,
    'B cannot read A''s runs';
END $$;

-- ---- anonymous (public key, no user) -------------------------------------
SELECT pg_temp.act('anon', NULL);
DO $$ BEGIN
  ASSERT (SELECT count(*) FROM workspaces) = 0, 'anon sees no workspaces';
  ASSERT (SELECT count(*) FROM feed_cards) = 0, 'anon sees no feed cards';
  ASSERT (SELECT count(*) FROM templates) = 1, 'anon sees published templates';
END $$;

-- ---- realtime safety: every table has a NOT NULL user_id ------------------
SELECT pg_temp.act('service', NULL);
DO $$
DECLARE missing text;
BEGIN
  SELECT string_agg(t.table_name, ', ') INTO missing
    FROM information_schema.tables t
   WHERE t.table_schema = 'tenant_test' AND t.table_type = 'BASE TABLE'
     AND t.table_name NOT IN ('users', 'templates')
     AND NOT EXISTS (
       SELECT 1 FROM information_schema.columns c
        WHERE c.table_schema = t.table_schema AND c.table_name = t.table_name
          AND c.column_name = 'user_id' AND c.is_nullable = 'NO');
  ASSERT missing IS NULL, 'tables without NOT NULL user_id (realtime would broadcast them): ' || missing;
END $$;

\echo 'RLS tests passed'
