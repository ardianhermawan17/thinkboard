-- ============================================================
-- ThinkBoard — restore standard Supabase role grants on public schema
-- Fixes: REST calls (both anon and service_role) failing with
-- "permission denied for schema public" on the cloud project. RLS
-- (already enabled per schema-final.sql §9) still governs anon/
-- authenticated row access; service_role bypasses RLS via BYPASSRLS
-- as usual, this only restores table/schema-level visibility.
-- ============================================================

grant usage on schema public to anon, authenticated, service_role;

grant all on all tables in schema public to anon, authenticated, service_role;
grant all on all routines in schema public to anon, authenticated, service_role;
grant all on all sequences in schema public to anon, authenticated, service_role;

alter default privileges in schema public grant all on tables to anon, authenticated, service_role;
alter default privileges in schema public grant all on routines to anon, authenticated, service_role;
alter default privileges in schema public grant all on sequences to anon, authenticated, service_role;
