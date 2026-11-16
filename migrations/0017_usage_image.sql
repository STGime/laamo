-- Draw a picture: images are paid work too, counted against the brain
-- allowance like model calls.
ALTER TABLE usage_ledger DROP CONSTRAINT usage_ledger_kind_check;
ALTER TABLE usage_ledger ADD CONSTRAINT usage_ledger_kind_check
  CHECK (kind IN ('llm', 'search', 'fetch', 'email', 'places', 'image'));
