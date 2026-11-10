-- Job queue and scheduler functions (issue #7).
--
-- Called via RPC (POST /v1/db/rpc/{fn}) with the secret key, or by Eurobase cron.
-- Eurobase RPC returns a single value, so functions return jsonb, never SETOF.
-- Each function refuses non-service callers explicitly (RLS would also stop them).
-- Writes made here fire no realtime events; that's intended (see migrations/README.md).

-- ---------------------------------------------------------------------------
-- next_schedule_time: next run of a schedule trigger strictly after from_ts.
--
-- config: {"freq": "daily" | "weekdays" | "weekly", "time": "HH:MM", "days": ["mon", …]}
--   daily    every day at time
--   weekdays Monday to Friday at time
--   weekly   on the listed days at time (days required)
-- tz: IANA timezone of the workspace, e.g. "Europe/Berlin". DST is handled by Postgres.
-- Returns NULL when the config can never fire (e.g. weekly with no days).
-- Also used by the API when a trigger is saved or a helper is switched on.
CREATE FUNCTION next_schedule_time(config jsonb, tz text, from_ts timestamptz)
RETURNS timestamptz
LANGUAGE plpgsql STABLE AS $$
DECLARE
  freq       text := coalesce(config->>'freq', 'daily');
  at_time    time := coalesce(config->>'time', '08:00')::time;
  local_day  date := (from_ts AT TIME ZONE tz)::date;
  d          date;
  dow        text;
  candidate  timestamptz;
BEGIN
  IF freq NOT IN ('daily', 'weekdays', 'weekly') THEN
    RAISE EXCEPTION 'unknown schedule freq: %', freq USING ERRCODE = 'invalid_parameter_value';
  END IF;

  FOR i IN 0..7 LOOP
    d := local_day + i;
    dow := lower(to_char(d, 'Dy')); -- mon, tue, … (not locale dependent without TM)
    IF freq = 'daily'
       OR (freq = 'weekdays' AND extract(isodow FROM d) <= 5)
       OR (freq = 'weekly' AND coalesce(config->'days', '[]'::jsonb) ? dow) THEN
      candidate := (d + at_time) AT TIME ZONE tz;
      IF candidate > from_ts THEN
        RETURN candidate;
      END IF;
    END IF;
  END LOOP;
  RETURN NULL;
END;
$$;

-- ---------------------------------------------------------------------------
-- enqueue_due_triggers: called by Eurobase cron every minute.
--
-- For every due schedule trigger: insert a "run" job (if the helper is active) and
-- advance next_run_at, in one transaction. SKIP LOCKED makes overlapping calls safe.
-- next_run_at is computed from now(), so after an outage each trigger catches up
-- with one run, not one per missed slot.
-- The worker creates the runs row via REST when it picks up the job, so the UI
-- sees the run appear live.
CREATE FUNCTION enqueue_due_triggers()
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  t         record;
  enqueued  int := 0;
  skipped   int := 0;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'enqueue_due_triggers is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;

  FOR t IN
    SELECT tr.id, tr.helper_id, tr.workspace_id, tr.config, tr.next_run_at,
           w.timezone, h.status AS helper_status, h.current_version_id
      FROM triggers tr
      JOIN helpers h    ON h.id = tr.helper_id
      JOIN workspaces w ON w.id = tr.workspace_id
     WHERE tr.type = 'schedule'
       AND tr.next_run_at <= now()
     ORDER BY tr.next_run_at
     LIMIT 500
     FOR UPDATE OF tr SKIP LOCKED
  LOOP
    IF t.helper_status = 'active' AND t.current_version_id IS NOT NULL THEN
      INSERT INTO jobs (workspace_id, kind, payload)
      VALUES (t.workspace_id, 'run', jsonb_build_object(
        'trigger_id',    t.id,
        'helper_id',     t.helper_id,
        'version_id',    t.current_version_id,
        'trigger_type',  'schedule',
        'scheduled_for', t.next_run_at
      ));
      enqueued := enqueued + 1;
    ELSE
      skipped := skipped + 1;
    END IF;

    UPDATE triggers
       SET next_run_at = next_schedule_time(t.config, t.timezone, now())
     WHERE id = t.id;
  END LOOP;

  RETURN jsonb_build_object('enqueued', enqueued, 'skipped', skipped);
END;
$$;

-- ---------------------------------------------------------------------------
-- claim_jobs: a worker takes up to n jobs.
--
-- Picks due queued jobs and running jobs whose lease expired (crash recovery).
-- FOR UPDATE SKIP LOCKED: two workers never get the same job.
-- Expired jobs that already used all attempts are marked failed instead of
-- claimed again. Returns a jsonb array of the claimed job rows.
-- Call: POST /v1/db/rpc/claim_jobs {"worker": "worker-1", "n": 10}
CREATE FUNCTION claim_jobs(worker text, n int DEFAULT 10)
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  claimed jsonb;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'claim_jobs is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;
  IF n IS NULL OR n < 1 OR n > 100 THEN
    RAISE EXCEPTION 'n must be between 1 and 100' USING ERRCODE = 'invalid_parameter_value';
  END IF;

  UPDATE jobs
     SET status = 'failed',
         last_error = coalesce(last_error || E'\n', '') || 'lease expired after final attempt',
         lease_until = NULL
   WHERE status = 'running'
     AND lease_until < now()
     AND attempts >= max_attempts;

  WITH picked AS (
    SELECT id
      FROM jobs
     WHERE (status = 'queued'  AND run_after   <= now())
        OR (status = 'running' AND lease_until <  now())
     ORDER BY run_after
     LIMIT n
     FOR UPDATE SKIP LOCKED
  ), updated AS (
    UPDATE jobs j
       SET status      = 'running',
           claimed_by  = worker,
           claimed_at  = now(),
           lease_until = now() + interval '5 minutes',
           attempts    = j.attempts + 1
      FROM picked
     WHERE j.id = picked.id
    RETURNING j.*
  )
  SELECT coalesce(jsonb_agg(to_jsonb(updated) ORDER BY updated.run_after), '[]'::jsonb)
    INTO claimed
    FROM updated;

  RETURN claimed;
END;
$$;

-- ---------------------------------------------------------------------------
-- insert_cards: bulk insert a card pile in one transaction.
--
-- rows: jsonb array of card objects with the cards table's columns. id, position,
-- data and trust are optional. The Go store keeps each call under Eurobase's
-- 1 MB request limit. Returns [{"id": …, "card_id": …}, …] ordered by position.
-- Cards are not watched live, so RPC (no realtime events) is fine here.
CREATE FUNCTION insert_cards(rows jsonb)
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  result jsonb;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'insert_cards is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;
  IF rows IS NULL OR jsonb_typeof(rows) <> 'array' THEN
    RAISE EXCEPTION 'rows must be a jsonb array' USING ERRCODE = 'invalid_parameter_value';
  END IF;

  WITH input AS (
    SELECT r.*, ord
      FROM jsonb_array_elements(rows) WITH ORDINALITY AS e(elem, ord),
           jsonb_to_record(e.elem) AS r(
             id uuid, workspace_id uuid, user_id uuid, run_id uuid, step_id uuid,
             card_id uuid, parent_id uuid, "position" int, kind text,
             data jsonb, trust jsonb, source_url text, kept boolean, score numeric, reason text)
  ), inserted AS (
    INSERT INTO cards (id, workspace_id, user_id, run_id, step_id, card_id, parent_id,
                       "position", kind, data, trust, source_url, kept, score, reason)
    SELECT coalesce(i.id, gen_random_uuid()), i.workspace_id, i.user_id, i.run_id, i.step_id,
           i.card_id, i.parent_id, coalesce(i."position", i.ord::int - 1), i.kind,
           coalesce(i.data, '{}'::jsonb), coalesce(i.trust, '{}'::jsonb),
           i.source_url, i.kept, i.score, i.reason
      FROM input i
     ORDER BY i.ord
    RETURNING id, card_id, "position"
  )
  SELECT coalesce(jsonb_agg(jsonb_build_object('id', id, 'card_id', card_id) ORDER BY "position"), '[]'::jsonb)
    INTO result
    FROM inserted;

  RETURN result;
END;
$$;
