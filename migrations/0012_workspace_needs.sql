-- What the user wants help with (onboarding S3: pick up to 3). Pre-sorts the
-- Shelf. Values are the template categories (packages/catalog/template.schema.json).
ALTER TABLE workspaces
  ADD COLUMN needs text[] NOT NULL DEFAULT '{}'
  CONSTRAINT workspaces_needs_check CHECK (
    cardinality(needs) <= 3
    AND needs <@ ARRAY['find_customers', 'stay_up_to_date', 'sort_emails', 'book_meetings',
                       'watch_competitors', 'plan_events', 'post_social']::text[]
  );
