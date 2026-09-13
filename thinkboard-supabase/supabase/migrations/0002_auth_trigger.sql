-- ============================================================
-- ThinkBoard — auth.users -> profiles/profile_identities bridge
-- Deferred: internal/individual distinction. For now every signup
-- gets exactly one profile_identities row, kind = 'individual',
-- is_primary = true. Revisit once the internal-login mechanism
-- (domain allowlist / SSO / invite flow) is decided.
-- ============================================================

create or replace function public.handle_new_user()
returns trigger
language plpgsql
security definer
set search_path = public
as $$
declare
  new_profile_id uuid;
begin
  insert into public.profiles (email, full_name, avatar_url)
  values (
    new.email,
    new.raw_user_meta_data ->> 'full_name',
    new.raw_user_meta_data ->> 'avatar_url'
  )
  returning id into new_profile_id;

  insert into public.profile_identities (profile_id, auth_user_id, email, kind, is_primary)
  values (new_profile_id, new.id, new.email, 'individual', true);

  return new;
end;
$$;

create trigger on_auth_user_created
  after insert on auth.users
  for each row execute function public.handle_new_user();
