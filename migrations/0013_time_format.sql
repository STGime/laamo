-- How times are shown in the app: 24h (default) or 12h with AM/PM.
ALTER TABLE workspaces
  ADD COLUMN time_format text NOT NULL DEFAULT '24h'
  CONSTRAINT workspaces_time_format_check CHECK (time_format IN ('24h', '12h'));
