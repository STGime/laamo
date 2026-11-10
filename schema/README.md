# Schema

SQL migrations for the Eurobase projects (dev, staging, prod), applied through Eurobase's schema tooling.

- Files: `NNNNNN_name.up.sql` / `NNNNNN_name.down.sql`, numbered in order.
- Every schema change goes here: tables, RLS policies, RPC functions (`claim_jobs`, `insert_cards`, `enqueue_due_triggers`) and the cron entry.
- Never change the live database by hand; the change would not exist in staging or prod.

Full data model: see issue #6.
