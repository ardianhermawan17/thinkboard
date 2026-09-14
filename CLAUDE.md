# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is right now

ThinkBoard: a per-team AI thinking board (OCR/highlight-weighted input → six-stage validation
pipeline → Lean-Canvas-style "temperature" score per conclusion). **Only the Supabase layer
exists today** (`thinkboard-supabase/`) — the planned Golang gateway (`services/gateway/`) and
Next.js frontend (`apps/web/`) have not been created yet. Don't assume those directories exist;
check before referencing them.

Source of truth for product intent, in order:
1. `00-thinkboard-abstract-plan.md` — the full spec (pipeline stages, memory model, RAG ratio,
   personas, three render modes, stack decision, capacity plan, repo layout).
2. `thinkboard-schema-final.sql` (root) — canonical schema design, mirrored into
   `thinkboard-supabase/supabase/migrations/0001_initial.sql`.
3. `01-thinkboard-schema-rationale.md` — maps every schema object back to the plan section that
   requires it, and separates plan-sourced decisions from ones made outside the plan doc (see its
   "Decisions not sourced from the plan doc" section — treat those as things to confirm, not
   settled spec).
4. `02-working.md` — process contract for how an agent should track work (see below). Governs
   *process*, not product — never treat it as a source for what to build.

`old-prompt/` holds superseded earlier drafts of the plan; `agent-thinking/` holds a scratch HTML
blueprint. Neither is authoritative — don't cite them over the four docs above.

## Working process contract (`02-working.md`)

If asked to do implementation work in this repo, `02-working.md` defines a mandatory
analyze → code → test → validate → result cycle tracked under `agent-history/`:

- Read `agent-history/running-process.json` **first**, before anything else, at the start of a
  session — it's the resumption pointer (`current_task_id`, `current_phase`).
- No code before a task has `task.json` + a completed `analyze.json`.
- One task = one folder `agent-history/NNN-task-<kebab-slug>/` (global monotonic `NNN`, never
  reused). Phase files (`analyze.json`, `code.json`, `test.json`, `validate.json`, `result.json`)
  are created lazily, one per phase actually entered.
- A task can't enter `validate` while any `task.json.goals[]` entry is still
  `pending`/`in_progress` (must be `done` or `blocked` with a reason).
- Every task declares one primary `architecture` — `supabase` | `backend` | `frontend` — per the
  table in `02-working.md` §5; cross-cutting work lists the other side in
  `secondary_architecture` but stays one task, one folder.
- `agent-history/` currently has no task folders — the backlog in `02-working.md` §8 (tasks
  000–012) is the starting point, not yet started.

This contract only applies when doing tracked feature/implementation work — it's not needed for
answering questions about the plan or schema.

## Repo layout (target, per plan §9 — only `thinkboard-supabase/` exists so far)

```
apps/web/                 # Next.js — not yet created
services/gateway/         # Golang orchestration service — not yet created
packages/contracts/       # shared types generated from Supabase schema + gateway API — not yet created
thinkboard-supabase/       # exists — Supabase project (schema, migrations, MCP config)
```

## Supabase project (`thinkboard-supabase/`)

- Managed via the Supabase CLI; `supabase/config.toml` sets local ports (API 54321, DB 54322) and
  Postgres 17.
- Migrations apply in order: `0001_initial.sql` (full consolidated schema — enums, tables, RLS,
  seed personas), `0002_auth_trigger.sql` (`auth.users` → `profiles`/`profile_identities` bridge;
  currently every signup is hardcoded `kind = 'individual'` — the internal/individual dual-login
  split from plan §3.1 is deferred, not implemented), `0003_grant_schema.sql` (restores standard
  `anon`/`authenticated`/`service_role` grants on `public` — needed because RLS alone doesn't
  restore schema-level visibility on a cloud project).
- A `supabase` MCP server is configured (`.mcp.json`, project ref `rtksqfhiueargoewxsyj`) with
  docs/account/database/debugging/development/functions/branching features enabled — prefer it
  over raw SQL/CLI calls for Supabase operations when available.
- Key schema facts worth knowing before touching migrations (full rationale in
  `01-thinkboard-schema-rationale.md`):
  - The Golang gateway is meant to use the service-role key and bypass RLS entirely; RLS policies
    only protect the direct-client (Next.js → Supabase) read path.
  - `pipeline_stage` enum is a literal transcription of the plan's 7 named stages
    (`scope_anchor` is stage 0, a pre-stage).
  - `memory_entries` collapses the plan's "four distinct stores" into one table with a `scope`
    enum, not four physical tables.
  - `user_llm_keys.vault_secret_id` — raw BYO LLM keys are never stored in a column; only
    Supabase Vault + gateway resolution.
  - `profiles` + `profile_identities` exists to work around Supabase's one-email-per-`auth.users`-row
    constraint (needed for two login paths) — this split is an implementation decision, not
    something the plan doc specifies.

## Commands

No app code exists yet, so there is no build/lint/test command for `apps/web` or
`services/gateway`. For the Supabase project:

```bash
cd thinkboard-supabase
supabase start              # local stack (Postgres, API, etc.)
supabase db reset           # reapply all migrations from scratch against local db
supabase db push            # push migrations to the linked cloud project
```
