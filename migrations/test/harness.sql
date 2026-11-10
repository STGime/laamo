-- Local stand-in for a Eurobase tenant database (test only, never applied to Eurobase).
-- Mirrors: per-project schema, users table, RLS helper functions driven by
-- app.end_user_id / app.end_user_role, and a separate non-owner role for API traffic.
CREATE SCHEMA tenant_test;

CREATE FUNCTION public.is_service_role() RETURNS boolean LANGUAGE sql STABLE AS
  $$ SELECT current_setting('app.end_user_role', true) = 'service' $$;
CREATE FUNCTION public.current_end_user_id() RETURNS uuid LANGUAGE sql STABLE AS
  $$ SELECT NULLIF(current_setting('app.end_user_id', true), '')::uuid $$;
CREATE FUNCTION tenant_test.auth_uid() RETURNS uuid LANGUAGE sql STABLE AS
  $$ SELECT NULLIF(current_setting('app.end_user_id', true), '')::uuid $$;

CREATE TABLE tenant_test.users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text UNIQUE
);

CREATE ROLE tenant_func LOGIN NOSUPERUSER NOBYPASSRLS;
GRANT USAGE ON SCHEMA tenant_test TO tenant_func;
ALTER ROLE tenant_func SET search_path = tenant_test, public;
