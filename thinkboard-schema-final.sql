-- ============================================================
-- ThinkBoard — CONSOLIDATED Supabase schema (final)
-- Supersedes: thinkboard-schema.sql + thinkboard-schema-personas-revised.sql
-- Postgres 15+ / Supabase. Run as one migration on a fresh project.
-- ============================================================

create extension if not exists "pgcrypto";
create extension if not exists "citext";
create extension if not exists "vector";

-- ============================================================
-- 0. Enums
-- ============================================================

create type identity_kind   as enum ('internal', 'individual');
create type team_role       as enum ('leader', 'admin', 'member', 'viewer');
create type session_mode    as enum ('planning', 'descriptive', 'visualize');
create type session_status  as enum ('draft', 'running', 'done', 'archived');

-- The core mechanism, in execution order. scope_anchor is stage 0 so every
-- run records how it was scoped, not just what it produced.
create type pipeline_stage  as enum (
  'scope_anchor',
  'analytic',
  'research',
  'validation',
  'questioning',
  'research_scoped',
  'result'
);

create type run_status      as enum ('pending','running','done','failed','cancelled');
create type message_role    as enum ('user','assistant','system');
create type warning_status  as enum ('open','acknowledged','dismissed');
create type artifact_kind   as enum ('pdf','image','note','output');
create type artifact_slot   as enum ('main','note');
create type extraction_kind as enum ('text_layer','ocr');
create type source_kind     as enum ('internet','material');
create type memory_scope    as enum ('persona','group','conversation','initial');
create type memory_kind     as enum ('idea','limitation');
create type diagram_kind    as enum ('mermaid','flow');
create type key_source      as enum ('user','shared');
create type key_status      as enum ('active','invalid','revoked');

-- ============================================================
-- 1. Identity
-- ============================================================
-- Supabase auth.users holds exactly ONE email per row. Dual login
-- (internal + individual) is therefore modelled by mapping two auth users
-- onto one profile via profile_identities. profiles.secondary_email is kept
-- for contact / recovery / lookup as specified.

create table profiles (
  id              uuid primary key default gen_random_uuid(),
  email           citext not null unique,
  secondary_email citext unique,
  secondary_email_verified_at timestamptz,
  full_name       text,
  avatar_url      text,
  locale          text not null default 'id',
  created_at      timestamptz not null default now(),
  updated_at      timestamptz not null default now(),
  constraint emails_differ check (secondary_email is null or secondary_email <> email)
);

create table profile_identities (
  id           uuid primary key default gen_random_uuid(),
  profile_id   uuid not null references profiles(id) on delete cascade,
  auth_user_id uuid not null unique references auth.users(id) on delete cascade,
  email        citext not null,
  kind         identity_kind not null,
  is_primary   boolean not null default false,
  linked_at    timestamptz not null default now(),
  unique (profile_id, kind)
);
create unique index profile_identities_one_primary
  on profile_identities (profile_id) where is_primary;
create index profile_identities_email_idx on profile_identities (email);

create or replace function current_profile_id() returns uuid
language sql stable security definer set search_path = public as $$
  select profile_id from profile_identities where auth_user_id = auth.uid() limit 1;
$$;

create table teams (
  id         uuid primary key default gen_random_uuid(),
  name       text not null,
  slug       citext not null unique,
  created_by uuid references profiles(id) on delete set null,
  created_at timestamptz not null default now()
);

create table team_members (
  team_id    uuid not null references teams(id) on delete cascade,
  profile_id uuid not null references profiles(id) on delete cascade,
  role       team_role not null default 'member',
  joined_at  timestamptz not null default now(),
  primary key (team_id, profile_id)
);
create index team_members_profile_idx on team_members (profile_id);

-- Exactly one leader per team — the person who decides the team persona.
create unique index team_members_one_leader
  on team_members (team_id) where role = 'leader';

create or replace function is_team_member(target_team uuid) returns boolean
language sql stable security definer set search_path = public as $$
  select exists (select 1 from team_members
    where team_id = target_team and profile_id = current_profile_id());
$$;

create or replace function is_team_leader(target_team uuid) returns boolean
language sql stable security definer set search_path = public as $$
  select exists (select 1 from team_members
    where team_id = target_team
      and profile_id = current_profile_id()
      and role = 'leader');
$$;

-- ============================================================
-- 2. Personas — two layers with opposite jobs
-- ============================================================
-- user_personas GENERATE (a professional lens, discipline-typed)
-- team_personas CONSTRAIN (a railroad guard, no discipline, one per team)

create table persona_disciplines (
  id          uuid primary key default gen_random_uuid(),
  key         citext not null unique,
  label       text not null,
  description text,
  schema      jsonb not null default '{}'::jsonb,
  is_active   boolean not null default true,
  created_at  timestamptz not null default now()
);

-- owner_profile_id IS NULL  -> shared library persona ("Psychology"), any user may pick it
-- owner_profile_id NOT NULL -> that user's own private persona
create table user_personas (
  id               uuid primary key default gen_random_uuid(),
  discipline_id    uuid references persona_disciplines(id) on delete set null,
  owner_profile_id uuid references profiles(id) on delete cascade,
  name             text not null,
  system_prompt    text not null,
  attributes       jsonb not null default '{}'::jsonb,
  is_active        boolean not null default true,
  created_at       timestamptz not null default now(),
  updated_at       timestamptz not null default now()
);
create index user_personas_owner_idx      on user_personas (owner_profile_id) where is_active;
create index user_personas_discipline_idx on user_personas (discipline_id);
create index user_personas_attributes_idx on user_personas using gin (attributes);
create index user_personas_library_idx    on user_personas (discipline_id)
  where owner_profile_id is null and is_active;

create table team_personas (
  id                  uuid primary key default gen_random_uuid(),
  team_id             uuid not null references teams(id) on delete cascade,
  name                text not null,
  guard_prompt        text not null,
  validation_criteria jsonb not null default '[]'::jsonb,
  applies_to_stages   pipeline_stage[] not null default '{questioning}',
  is_active           boolean not null default true,
  decided_by          uuid references profiles(id) on delete set null,
  created_at          timestamptz not null default now(),
  updated_at          timestamptz not null default now()
);
-- "1 team = 1 persona focus"
create unique index team_personas_one_active
  on team_personas (team_id) where is_active;
create index team_personas_team_idx on team_personas (team_id);

create or replace function active_team_persona(target_team uuid) returns uuid
language sql stable security definer set search_path = public as $$
  select id from team_personas where team_id = target_team and is_active limit 1;
$$;

-- ============================================================
-- 3. Boards & sessions (sessions ARE the kanban cards)
-- ============================================================

create table boards (
  id         uuid primary key default gen_random_uuid(),
  team_id    uuid not null references teams(id) on delete cascade,
  name       text not null,
  created_at timestamptz not null default now()
);
create index boards_team_idx on boards (team_id);

create table board_columns (
  id       uuid primary key default gen_random_uuid(),
  board_id uuid not null references boards(id) on delete cascade,
  name     text not null,
  position int not null default 0
);
create index board_columns_board_idx on board_columns (board_id, position);

create table sessions (
  id               uuid primary key default gen_random_uuid(),
  column_id        uuid not null references board_columns(id) on delete cascade,
  user_persona_id  uuid references user_personas(id) on delete set null,
  team_persona_id  uuid references team_personas(id) on delete set null,
  title            text not null,
  initial_question text not null,              -- the scope anchor
  scope_embedding  vector(1536),               -- drift comparison baseline
  default_mode     session_mode not null default 'planning',
  status           session_status not null default 'draft',
  internet_ratio   numeric(3,2) not null default 0.20
                     check (internet_ratio >= 0 and internet_ratio <= 1),
  message_cap      int not null default 40,
  position         int not null default 0,
  created_by       uuid references profiles(id) on delete set null,
  created_at       timestamptz not null default now(),
  updated_at       timestamptz not null default now()
);
create index sessions_column_idx       on sessions (column_id, position);
create index sessions_user_persona_idx on sessions (user_persona_id);
create index sessions_team_persona_idx on sessions (team_persona_id);

create table messages (
  id            uuid primary key default gen_random_uuid(),
  session_id    uuid not null references sessions(id) on delete cascade,
  role          message_role not null,
  content       text not null,
  token_count   int not null default 0,
  is_summarized boolean not null default false,
  created_at    timestamptz not null default now()
);
create index messages_session_idx on messages (session_id, created_at);

create table context_warnings (
  id             uuid primary key default gen_random_uuid(),
  session_id     uuid not null references sessions(id) on delete cascade,
  message_id     uuid references messages(id) on delete cascade,
  drift_score    numeric(4,3) not null,
  detected_topic text,
  status         warning_status not null default 'open',
  created_at     timestamptz not null default now()
);
create index context_warnings_session_idx on context_warnings (session_id, status);

-- ============================================================
-- 4. Pipeline — the core mechanism
-- ============================================================

create table pipeline_runs (
  id          uuid primary key default gen_random_uuid(),
  session_id  uuid not null references sessions(id) on delete cascade,
  mode        session_mode not null default 'planning',
  status      run_status not null default 'pending',
  started_at  timestamptz,
  finished_at timestamptz,
  created_at  timestamptz not null default now()
);
create index pipeline_runs_session_idx on pipeline_runs (session_id, created_at desc);

create table pipeline_stages (
  id          uuid primary key default gen_random_uuid(),
  run_id      uuid not null references pipeline_runs(id) on delete cascade,
  stage       pipeline_stage not null,
  status      run_status not null default 'pending',
  input       jsonb,
  output      jsonb,
  error       text,
  started_at  timestamptz,
  finished_at timestamptz,
  unique (run_id, stage)
);
create index pipeline_stages_run_idx on pipeline_stages (run_id);

create table points (
  id              uuid primary key default gen_random_uuid(),
  run_id          uuid not null references pipeline_runs(id) on delete cascade,
  parent_point_id uuid references points(id) on delete cascade,
  content         text not null,
  position        int not null default 0,
  created_at      timestamptz not null default now()
);
create index points_run_idx on points (run_id, position);

create table point_conclusions (
  id               uuid primary key default gen_random_uuid(),
  point_id         uuid not null references points(id) on delete cascade,
  conclusion       text not null,
  temperature      numeric(3,2) not null check (temperature >= 0 and temperature <= 1),
  confidence_label text,
  open_question    text,                       -- written by the questioning stage
  created_at       timestamptz not null default now()
);
create index point_conclusions_point_idx on point_conclusions (point_id);

-- ------------------------------------------------------------
-- The 3 modes. One row per mode per run: a single thinking run can be
-- rendered three ways without re-running the pipeline.
--   planning    -> content_md holds the structured plan
--   descriptive -> content_md holds prose citing source bias_note/credibility
--   visualize   -> diagram_spec holds mermaid text or flow nodes/edges
-- ------------------------------------------------------------
create table run_renderings (
  id             uuid primary key default gen_random_uuid(),
  run_id         uuid not null references pipeline_runs(id) on delete cascade,
  mode           session_mode not null,
  content_md     text,
  diagram_kind   diagram_kind,
  diagram_spec   jsonb,
  is_user_edited boolean not null default false,
  edited_by      uuid references profiles(id) on delete set null,
  created_at     timestamptz not null default now(),
  updated_at     timestamptz not null default now(),
  unique (run_id, mode),
  constraint rendering_payload check (
    (mode = 'visualize' and diagram_spec is not null and diagram_kind is not null)
    or (mode <> 'visualize' and content_md is not null)
  )
);
create index run_renderings_run_idx on run_renderings (run_id);

-- ============================================================
-- 5. Artifacts, highlights, per-user term weighting
-- ============================================================

create table artifacts (
  id           uuid primary key default gen_random_uuid(),
  session_id   uuid not null references sessions(id) on delete cascade,
  kind         artifact_kind not null,
  slot         artifact_slot,
  title        text,
  storage_path text,
  page_count   int,
  created_by   uuid references profiles(id) on delete set null,
  created_at   timestamptz not null default now()
);
create index artifacts_session_idx on artifacts (session_id);
create unique index artifacts_one_per_slot
  on artifacts (session_id, slot) where slot is not null;

create table highlights (
  id          uuid primary key default gen_random_uuid(),
  artifact_id uuid not null references artifacts(id) on delete cascade,
  profile_id  uuid not null references profiles(id) on delete cascade,
  page        int,
  bbox        jsonb,
  text        text not null,
  extraction  extraction_kind not null default 'text_layer',
  confidence  numeric(4,3),
  weight      numeric(4,2) not null default 1.0,
  created_at  timestamptz not null default now()
);
create index highlights_artifact_idx on highlights (artifact_id);
create index highlights_profile_idx  on highlights (profile_id);

create table mini_conclusions (
  id           uuid primary key default gen_random_uuid(),
  highlight_id uuid not null unique references highlights(id) on delete cascade,
  content      text not null,
  created_at   timestamptz not null default now()
);

-- Per-user weighting: terms this person highlights often get boosted.
create table highlight_term_weights (
  profile_id uuid not null references profiles(id) on delete cascade,
  term       citext not null,
  weight     numeric(4,2) not null default 1.0,
  hit_count  int not null default 1,
  updated_at timestamptz not null default now(),
  primary key (profile_id, term)
);

-- ============================================================
-- 6. Sources — the 20/80 mix, plus bias data for Descriptive mode
-- ============================================================

create table sources (
  id          uuid primary key default gen_random_uuid(),
  session_id  uuid not null references sessions(id) on delete cascade,
  kind        source_kind not null,
  url         text,
  artifact_id uuid references artifacts(id) on delete set null,
  title       text,
  excerpt     text,
  bias_note   text,                              -- surfaced by Descriptive mode
  credibility numeric(3,2) check (credibility between 0 and 1),
  embedding   vector(1536),
  created_at  timestamptz not null default now(),
  constraint source_target check (
    (kind = 'internet' and url is not null) or
    (kind = 'material' and artifact_id is not null)
  )
);
create index sources_session_idx   on sources (session_id, kind);
create index sources_embedding_idx on sources using hnsw (embedding vector_cosine_ops);

create table point_sources (
  point_id  uuid not null references points(id) on delete cascade,
  source_id uuid not null references sources(id) on delete cascade,
  weight    numeric(4,3) not null default 1.0,
  primary key (point_id, source_id)
);

-- ============================================================
-- 7. Memory — 4 scopes in one table, ideas vs limitations
-- ============================================================

create table memory_entries (
  id         uuid primary key default gen_random_uuid(),
  scope      memory_scope not null,
  persona_id uuid references user_personas(id) on delete cascade,
  team_id    uuid references teams(id) on delete cascade,
  session_id uuid references sessions(id) on delete cascade,
  kind       memory_kind not null default 'idea',
  content    text not null,
  embedding  vector(1536),
  created_by uuid references profiles(id) on delete set null,
  created_at timestamptz not null default now(),
  constraint memory_scope_owner check (
    (scope = 'persona'      and persona_id is not null) or
    (scope = 'group'        and team_id    is not null) or
    (scope = 'conversation' and session_id is not null) or
    (scope = 'initial'      and session_id is not null)
  )
);
create index memory_entries_persona_idx   on memory_entries (persona_id) where scope = 'persona';
create index memory_entries_team_idx      on memory_entries (team_id)    where scope = 'group';
create index memory_entries_session_idx   on memory_entries (session_id, scope);
create index memory_entries_embedding_idx on memory_entries using hnsw (embedding vector_cosine_ops);

-- ============================================================
-- 8. LLM layer
-- ============================================================

create table llm_providers (
  id        uuid primary key default gen_random_uuid(),
  key       citext not null unique,
  label     text not null,
  base_url  text not null,
  is_active boolean not null default true
);

create table llm_models (
  id             uuid primary key default gen_random_uuid(),
  provider_id    uuid not null references llm_providers(id) on delete cascade,
  model_key      text not null,
  label          text,
  input_cost     numeric(12,6) not null default 0,
  output_cost    numeric(12,6) not null default 0,
  context_window int,
  is_free_tier   boolean not null default false,
  is_active      boolean not null default true,
  unique (provider_id, model_key)
);

-- SECURITY: the raw API key is NEVER stored here. vault_secret_id points at
-- Supabase Vault. Only the Go gateway ever resolves it.
create table user_llm_keys (
  id                uuid primary key default gen_random_uuid(),
  profile_id        uuid not null references profiles(id) on delete cascade,
  provider_id       uuid not null references llm_providers(id) on delete cascade,
  vault_secret_id   uuid not null,
  label             text,
  status            key_status not null default 'active',
  last_validated_at timestamptz,
  created_at        timestamptz not null default now(),
  unique (profile_id, provider_id)
);

create table llm_requests (
  id                uuid primary key default gen_random_uuid(),
  stage_id          uuid references pipeline_stages(id) on delete set null,
  session_id        uuid references sessions(id) on delete set null,
  profile_id        uuid references profiles(id) on delete set null,
  model_id          uuid references llm_models(id) on delete set null,
  key_source        key_source not null,
  prompt_tokens     int not null default 0,
  completion_tokens int not null default 0,
  cost              numeric(12,6) not null default 0,
  latency_ms        int,
  status            run_status not null default 'done',
  error             text,
  created_at        timestamptz not null default now()
);
create index llm_requests_session_idx on llm_requests (session_id, created_at desc);
create index llm_requests_profile_idx on llm_requests (profile_id, created_at desc);
create index llm_requests_stage_idx   on llm_requests (stage_id);

-- ============================================================
-- 9. Row Level Security
-- ============================================================
-- Access flows from team membership. The Go gateway uses the service role key
-- and bypasses RLS entirely; these policies protect direct client reads.

alter table profiles               enable row level security;
alter table profile_identities     enable row level security;
alter table teams                  enable row level security;
alter table team_members           enable row level security;
alter table persona_disciplines    enable row level security;
alter table user_personas          enable row level security;
alter table team_personas          enable row level security;
alter table boards                 enable row level security;
alter table board_columns          enable row level security;
alter table sessions               enable row level security;
alter table messages               enable row level security;
alter table context_warnings       enable row level security;
alter table pipeline_runs          enable row level security;
alter table pipeline_stages        enable row level security;
alter table points                 enable row level security;
alter table point_conclusions      enable row level security;
alter table run_renderings         enable row level security;
alter table artifacts              enable row level security;
alter table highlights             enable row level security;
alter table mini_conclusions       enable row level security;
alter table highlight_term_weights enable row level security;
alter table sources                enable row level security;
alter table point_sources          enable row level security;
alter table memory_entries         enable row level security;
alter table llm_providers          enable row level security;
alter table llm_models             enable row level security;
alter table user_llm_keys          enable row level security;
alter table llm_requests           enable row level security;

create or replace function can_access_session(target_session uuid) returns boolean
language sql stable security definer set search_path = public as $$
  select exists (
    select 1 from sessions s
    join board_columns c on c.id = s.column_id
    join boards b        on b.id = c.board_id
    where s.id = target_session and is_team_member(b.team_id)
  );
$$;

create policy "own profile" on profiles
  for all using (id = current_profile_id()) with check (id = current_profile_id());
create policy "own identities" on profile_identities
  for select using (profile_id = current_profile_id());
create policy "member teams"  on teams        for select using (is_team_member(id));
create policy "member roster" on team_members for select using (is_team_member(team_id));

-- Personas: library personas readable by all, private ones only by owner.
create policy "read personas" on user_personas
  for select using (owner_profile_id is null or owner_profile_id = current_profile_id());
create policy "write own personas" on user_personas
  for all using (owner_profile_id = current_profile_id())
  with check (owner_profile_id = current_profile_id());

-- The rail: every member reads it, ONLY THE LEADER changes it.
create policy "read team rail"   on team_personas for select using (is_team_member(team_id));
create policy "leader sets rail" on team_personas
  for all using (is_team_leader(team_id)) with check (is_team_leader(team_id));

create policy "team boards"  on boards for all using (is_team_member(team_id));
create policy "team columns" on board_columns for all using (exists (
  select 1 from boards b where b.id = board_id and is_team_member(b.team_id)));
create policy "team sessions" on sessions for all using (exists (
  select 1 from board_columns c join boards b on b.id = c.board_id
  where c.id = column_id and is_team_member(b.team_id)));

create policy "session messages"  on messages         for all using (can_access_session(session_id));
create policy "session warnings"  on context_warnings for all using (can_access_session(session_id));
create policy "session runs"      on pipeline_runs    for all using (can_access_session(session_id));
create policy "session artifacts" on artifacts        for all using (can_access_session(session_id));
create policy "session sources"   on sources          for all using (can_access_session(session_id));
create policy "session memory"    on memory_entries   for select using (
  (scope in ('conversation','initial') and can_access_session(session_id))
  or (scope = 'group'   and is_team_member(team_id))
  or (scope = 'persona' and exists (
        select 1 from user_personas p where p.id = persona_id
        and (p.owner_profile_id is null or p.owner_profile_id = current_profile_id())))
);

-- Run children inherit access from their run.
create policy "run stages"     on pipeline_stages for all using (exists (
  select 1 from pipeline_runs r where r.id = run_id and can_access_session(r.session_id)));
create policy "run points"     on points          for all using (exists (
  select 1 from pipeline_runs r where r.id = run_id and can_access_session(r.session_id)));
create policy "run renderings" on run_renderings  for all using (exists (
  select 1 from pipeline_runs r where r.id = run_id and can_access_session(r.session_id)));
create policy "point conclusions" on point_conclusions for all using (exists (
  select 1 from points p join pipeline_runs r on r.id = p.run_id
  where p.id = point_id and can_access_session(r.session_id)));
create policy "point sources" on point_sources for all using (exists (
  select 1 from points p join pipeline_runs r on r.id = p.run_id
  where p.id = point_id and can_access_session(r.session_id)));
create policy "artifact highlights" on highlights for all using (exists (
  select 1 from artifacts a where a.id = artifact_id and can_access_session(a.session_id)));
create policy "highlight conclusions" on mini_conclusions for all using (exists (
  select 1 from highlights h join artifacts a on a.id = h.artifact_id
  where h.id = highlight_id and can_access_session(a.session_id)));

create policy "own term weights" on highlight_term_weights
  for all using (profile_id = current_profile_id())
  with check (profile_id = current_profile_id());
create policy "own llm keys" on user_llm_keys
  for all using (profile_id = current_profile_id())
  with check (profile_id = current_profile_id());
create policy "own llm usage" on llm_requests
  for select using (profile_id = current_profile_id());

create policy "read disciplines" on persona_disciplines for select using (auth.role() = 'authenticated');
create policy "read providers"   on llm_providers       for select using (auth.role() = 'authenticated');
create policy "read models"      on llm_models          for select using (auth.role() = 'authenticated');

-- ============================================================
-- 10. Seed
-- ============================================================

insert into persona_disciplines (key, label, description, schema) values
  ('general', 'General', 'No professional specialization', '{}'::jsonb),
  ('psychology', 'Psychology', 'Behavioral and cognitive lens',
   '{"type":"object","properties":{
       "orientation":{"type":"string"},
       "focus_areas":{"type":"array","items":{"type":"string"}},
       "evidence_bias":{"type":"string"}}}'::jsonb);

insert into user_personas (discipline_id, owner_profile_id, name, system_prompt, attributes)
select d.id, null, 'General',
  'Analyze without a specific professional specialization. Prioritize clarity and explicit reasoning.',
  '{}'::jsonb
from persona_disciplines d where d.key = 'general';

insert into user_personas (discipline_id, owner_profile_id, name, system_prompt, attributes)
select d.id, null, 'Psychology',
  'Analyze through a behavioral and cognitive lens. Surface assumptions about motivation, bias, and group dynamics before accepting a conclusion.',
  '{"orientation":"cognitive-behavioral","evidence_bias":"empirical"}'::jsonb
from persona_disciplines d where d.key = 'psychology';
