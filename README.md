# ThinkBoard Architecture

ThinkBoard is a per-team AI thinking board for turning scoped team input into validated, scored conclusions. The product combines OCR/highlight-weighted input, a fixed validation pipeline, separated memory scopes, and a Lean-Canvas-style `temperature` score for each conclusion.

This repository is currently an architecture and Supabase-schema workspace. The only implemented layer today is `thinkboard-supabase/`; the planned Go gateway and Next.js frontend have not been created yet.

## Current status

- **Exists now:** `thinkboard-supabase/` with Supabase configuration and migrations.
- **Canonical schema:** `thinkboard-schema-final.sql`, mirrored into `thinkboard-supabase/supabase/migrations/0001_initial.sql`.
- **Planned later:**
  - `services/gateway/` — Go orchestration gateway.
  - `apps/web/` — Next.js frontend.
  - `packages/contracts/` — generated shared contracts.

## Product summary

ThinkBoard treats ideation like a supply chain: raw notes, highlighted PDFs, scanned material, and conversation turns move through a staged pipeline and come out as structured, evidence-backed conclusions.

The core differentiator is the **context warning** guardrail. Each session is anchored to an initial question, and new input is checked for scope drift before the pipeline processes it.

The pipeline stages are:

1. **Scope anchor** — pre-stage that defines the topic boundary and applies highlight weighting.
2. **Analytic** — decomposes input into atomic points.
3. **Research** — retrieves supporting material using the internet/material ratio.
4. **Validation** — checks claims against sources.
5. **Questioning** — adversarial second-pass validation.
6. **Research scoped** — targeted follow-up research for open questions.
7. **Result** — produces per-point conclusions and temperature scores.

## Key architecture decisions

- Use a **hybrid architecture**: Next.js frontend + Go orchestration gateway + Supabase auth/Postgres/storage/realtime.
- Keep the gateway focused on complex orchestration: pipeline runs, streaming, RAG mixing, memory, context warnings, BYO-key resolution, and LLM accounting.
- Let simple client reads, such as kanban state and artifact lists, go directly from Next.js to Supabase through RLS.
- Use **CQRS-lite**, not full CQRS: separate write-side pipeline orchestration from read-side session queries, without command buses, event sourcing, projections, or an outbox.
- Use client-side OCR/highlight capture for artifacts:
  - Browser Selection/Range API for text-layer PDFs and HTML artifacts.
  - Tesseract.js in-browser OCR for scanned/image pages.
- Use a default **20/80 internet/material retrieval ratio**, adjustable per session.
- Store BYO LLM keys through Supabase Vault references only; raw keys are never stored in a plain database column.

## Repository layout

```text
thinkboard-architecture/
├── README.md
├── 00-thinkboard-abstract-plan.md
├── 01-thinkboard-schema-rationale.md
├── 02-working.md
├── 03-backend-folder-architecture.md
├── 04-TODO.md
├── 05-agent-limitation.md
├── CLAUDE.md
├── thinkboard-schema-final.sql
├── agent-history/
├── agent-thinking/
├── old-prompt/
└── thinkboard-supabase/
```

Target layout from the architecture plan:

```text
apps/web/                 # Next.js app — not created yet
services/gateway/         # Go orchestration gateway — not created yet
packages/contracts/       # Shared generated types — not created yet
thinkboard-supabase/      # Supabase project — exists
```

## Documentation map

The docs should be read in this order when implementing the project:

1. [`00-thinkboard-abstract-plan.md`](./00-thinkboard-abstract-plan.md) — primary product and architecture specification.
2. [`thinkboard-schema-final.sql`](./thinkboard-schema-final.sql) — canonical database schema.
3. [`01-thinkboard-schema-rationale.md`](./01-thinkboard-schema-rationale.md) — schema-to-plan cross-reference and notes on decisions not explicitly sourced from the plan.
4. [`02-working.md`](./02-working.md) — mandatory agent work-tracking process for implementation tasks.
5. [`03-backend-folder-architecture.md`](./03-backend-folder-architecture.md) — planned Go gateway package structure, import rules, transport model, concurrency model, and build order.
6. [`04-TODO.md`](./04-TODO.md) — how the `agent-thinking/todo` contract workflow turns a human prompt into a tracked task, and its relation to `agent-history`.
7. [`05-agent-limitation.md`](./05-agent-limitation.md) — hard limits on what an agent may do unattended, especially around git write commands.
8. [`CLAUDE.md`](./CLAUDE.md) — repository-specific instructions for coding agents.

Archived/superseded drafts:

- [`old-prompt/00-thinkboard-abstract-plan.md`](./old-prompt/00-thinkboard-abstract-plan.md)
- [`old-prompt/01-thinkboard-abstract-plan.md`](./old-prompt/01-thinkboard-abstract-plan.md)

The files under `old-prompt/` are historical drafts and are not authoritative. Do not cite them over the current root-level plan and schema rationale.

## Supabase project

The Supabase project lives in `thinkboard-supabase/` and is managed with the Supabase CLI.

Migration order:

1. `0001_initial.sql` — consolidated schema, enums, tables, RLS, seed personas.
2. `0002_auth_trigger.sql` — `auth.users` bridge into `profiles` and `profile_identities`.
3. `0003_grant_schema.sql` — restores standard schema-level grants for `anon`, `authenticated`, and `service_role`.

Useful commands:

```bash
cd thinkboard-supabase
supabase start
supabase db reset
supabase db push
```

Notes:

- Local API port: `54321`.
- Local DB port: `54322`.
- Postgres version: 17.
- The planned Go gateway will use the service-role key and bypass RLS, so gateway authorization must mirror the RLS protections enforced on the direct-client path.

## Planned Go gateway

The planned gateway will live at `services/gateway/` and use:

- Go 1.23+
- Gin for HTTP and SSE
- `pgx/v5` against Supabase Postgres via the transaction pooler
- Bounded concurrency with `errgroup`
- CQRS-lite structure:

```text
services/gateway/internal/
├── pipeline/        # write side: seven-stage orchestration and streaming
├── session/         # read side: run/session queries
├── llm/             # provider clients and BYO-key resolution
├── memory/          # persona/group/conversation/initial memory scopes
├── contextwarning/  # scope-drift heuristic
├── auth/            # RLS-equivalent authorization for service-role gateway path
├── httpapi/         # Gin router, middleware, handlers, DTOs, SSE
├── platform/        # config, Postgres, telemetry, web search, embeddings
└── shared/          # kernel types, concurrency primitives, errors, clock
```

Important gateway rules:

- Do not query `team_members` outside `internal/auth/`.
- Do not resolve Supabase Vault secrets outside `internal/llm/vault.go`.
- Do not pass `*gin.Context` past handlers.
- Do not start bare goroutines outside approved concurrency owners.
- Keep handlers thin: bind request, call one service method, map result/error.
- A new pipeline stage should require one stage file, one registry entry, and one transition test.

## Planned frontend

The planned frontend will live at `apps/web/` and use feature folders:

```text
features/<feature-name>/
├── index.ts
├── <feature-name>.tsx
├── ui.tsx
├── use-<feature-name>.ts
└── <feature-name>.types.ts
```

State management guidance:

- TanStack Query for server state.
- Zustand for cross-feature UI state.
- URL state for shareable view state.
- Local `useState` for component-local state.

Primary UI pieces include a ChatGPT-style conversation pane, kanban board, artifact sheet, OCR/highlight capture, and three render modes: Planned, Descriptive, and Visualize.

## Agent implementation process

For implementation work, follow [`02-working.md`](./02-working.md):

1. Read `agent-history/running-process.json` first.
2. Create one task folder under `agent-history/NNN-task-<kebab-slug>/`.
3. Do not write code until `task.json` and a completed `analyze.json` exist.
4. Track phases with `analyze.json`, `code.json`, `test.json`, `validate.json`, and `result.json`.
5. Do not enter validation while any task goal is still pending or in progress.

This process applies to tracked feature/implementation work. It is not required for simply reading or explaining the architecture docs.

## Known decisions to confirm

The schema rationale identifies these as internally consistent but not explicitly sourced from the main plan:

- The dual-identity model with `profiles` and `profile_identities`.
- The two-layer persona split with `user_personas` and `team_personas`.
- `persona_disciplines` extensibility and the seeded psychology persona.
- Several Go gateway structure choices, including Gin, `httpapi/`, `platform/`, `shared/`, detached run contexts, and light DDD boundaries.

Treat these as design choices to confirm before downstream work depends on them.

## Open risks

- OCR accuracy for scanned or handwritten material may require manual correction.
- The context-warning heuristic may produce false positives if thresholds are wrong.
- BYO API key adoption is uncertain; if adoption is low, the shared LLM pool may drive costs.
- The 450-user, 8-hour sustained streaming profile must be tested against real Supabase and gateway capacity before broad rollout.
