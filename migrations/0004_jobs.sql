-- Job queue on the Eurobase API (see MVP plan, "Job queue on the Eurobase API").
-- The claim/enqueue RPC functions and the cron entry come in issue #7.
--
-- Service only. Workers subscribe to realtime INSERTs as a wake-up signal.
-- user_id is the workspace owner, or the all-zero "system" id for jobs without
-- a workspace; no end user has that id, so realtime never sends them a job row.
-- Keep payloads to ids: never put user content or secrets in a job.

CREATE TABLE jobs (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id  uuid REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id       uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  kind          text NOT NULL, -- run, send_digest, …
  payload       jsonb NOT NULL DEFAULT '{}'::jsonb,
  status        text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
  run_after     timestamptz NOT NULL DEFAULT now(),
  attempts      int NOT NULL DEFAULT 0,
  max_attempts  int NOT NULL DEFAULT 5,
  claimed_by    text,
  claimed_at    timestamptz,
  lease_until   timestamptz,
  last_error    text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  CHECK (workspace_id IS NOT NULL OR user_id = '00000000-0000-0000-0000-000000000000')
);
-- claim_jobs() looks for due queued jobs and expired leases
CREATE INDEX jobs_queued_idx ON jobs (run_after) WHERE status = 'queued';
CREATE INDEX jobs_lease_idx ON jobs (lease_until) WHERE status = 'running';

ALTER TABLE jobs ENABLE ROW LEVEL SECURITY;
CREATE POLICY service_all ON jobs USING (public.is_service_role()) WITH CHECK (public.is_service_role());
