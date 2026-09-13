# ThinkBoard — Agent Working Contract (`working.md`)

Operating contract for any AI agent (Claude Code or otherwise) building ThinkBoard.
It defines how the agent **analyzes → reads → codes → tests → validates → reports**, and how
that work is tracked, resumably, in `agent-history/`. Source of truth for product intent
stays `00-thinkboard-abstract-plan.md` and `thinkboard-schema-final.sql` /
`01-thinkboard-schema-rationale.md` — this file only governs *process*, not *product*.

---

## 0. Non-negotiables

1. No code is written before a task has a `task.json` and a completed `analyze.json`.
2. No task moves to `validate` while any goal in `task.json.goals` is still `pending`/`in_progress`
   with no `blocked_reason`.
3. `agent-history/running-process.json` is read **first**, before touching any other file, at
   the start of every agent session — it is the resumption pointer.
4. Every JSON file in this contract must stay valid, parseable JSON. No comments, no trailing
   commas. Field tables below double as the schema documentation.

---

## 1. Directory contract

```
thinkboard/
  agent-history/
    running-process.json
    000-task-supabase-schema-migration/
      task.json
      analyze.json
      code.json
      test.json
      validate.json
      result.json
    001-task-gateway-analytic-stage-skeleton/
      task.json
      analyze.json
      ...
  apps/web/                 # Next.js — see plan §9 frontend feature-folder convention
  services/gateway/         # Golang — see plan §9 CQRS-lite convention
  packages/contracts/       # generated types shared by both
```

Rules:
- One unit of agent work = one folder under `agent-history/`, named exactly
  `NNN-task-<kebab-slug>` (rule in §3).
- A task folder always contains `task.json`. The five phase files are created lazily, one per
  phase, only as the agent actually enters that phase — a task interrupted mid-analyze
  legitimately has only `task.json` + `analyze.json` on disk, and that is a valid, resumable state.
- Nothing is ever deleted from `agent-history/`. A cancelled or superseded task keeps its folder
  and gets `task.json.status = "cancelled"` — it's the audit trail, not scratch space.

---

## 2. Phase model (5 stages → 5 files)

| Stage (as requested) | File | Notes |
|---|---|---|
| Analyze **+ Read** | `analyze.json` | "Read" is not a separate file — every `analyze.json` must open by reading the `plan_refs` and any existing related code *before* writing findings. This is also requirement #1's "analyze" folder. |
| Code | `code.json` | Requirement #1's "code" folder. |
| (Test) | `test.json` | Requirement #1's "test" folder — split out from validate because automated-test evidence and goal/spec validation are different checks with different failure modes. |
| Validate | `validate.json` | Requirement #1's "validation" folder. Checks code+tests against `task.json.goals` and against the plan/schema docs. |
| Result | `result.json` | The requested final "result" stage — closes the task, recomputes goal completion, updates `running-process.json`. |

A task's `task.json.phases` map tracks which of the five files exist and their individual status
(§4.2), so partial progress is always visible without opening every phase file.

---

## 3. Naming & numbering

- `NNN` — zero-padded 3-digit, **global monotonic counter across the whole project**, not
  per-architecture. Numbers are never reused or resequenced, even for a cancelled task.
- `task-` — literal, exactly as requested (requirement #2: every task folder/id starts
  `NNN-task-`).
- `<kebab-slug>` — short, descriptive, architecture-agnostic (architecture is a `task.json`
  field, not part of the name, so a task can be re-tagged without renaming its folder).
- Example progression: `000-task-supabase-schema-migration`,
  `001-task-gateway-analytic-stage-skeleton`, `002-task-frontend-vertical-slice`.

---

## 4. JSON contracts

All files are JSON. `task_id` inside every phase file must match the folder name exactly, so a
phase file can be validated in isolation without trusting its file path.

### 4.1 `agent-history/running-process.json`

The single resumption pointer. Updated at the **start and end** of every phase transition, so a
crash mid-phase leaves `current_phase` pointing at the phase that was interrupted — the agent's
first action on resume is to re-validate that phase's file, not blindly continue past it.

| Field | Type | Description |
|---|---|---|
| `schema_version` | string | Contract version, e.g. `"1.0"`. |
| `updated_at` | ISO-8601 string | Last write time. |
| `current_task_id` | string \| null | The task currently being worked, or `null` if idle. |
| `current_phase` | `"analyze"\|"code"\|"test"\|"validate"\|"result"\|"idle"` | Where the agent was/is. |
| `history_path` | string | Relative path to the current task folder. |
| `status` | `"idle"\|"in_progress"\|"blocked"` | Overall process state. |
| `task_sequence` | array | Ordered list of `{id, architecture, status, title}` — a flat index of every task, for a resuming agent to see the whole backlog without opening every folder. |

```json
{
  "schema_version": "1.0",
  "updated_at": "2026-09-13T09:40:00Z",
  "current_task_id": "000-task-supabase-schema-migration",
  "current_phase": "validate",
  "history_path": "agent-history/000-task-supabase-schema-migration",
  "status": "in_progress",
  "task_sequence": [
    { "id": "000-task-supabase-schema-migration", "architecture": "supabase", "status": "in_progress", "title": "Apply consolidated schema, auth, RLS" },
    { "id": "001-task-gateway-analytic-stage-skeleton", "architecture": "backend", "status": "pending", "title": "Golang gateway skeleton + Analytic stage stub" }
  ]
}
```

### 4.2 `task.json` — the per-task contract

| Field | Type | Description |
|---|---|---|
| `id` | string | Must equal the folder name. |
| `title` | string | Short human-readable title. |
| `architecture` | `"supabase"\|"backend"\|"frontend"` | **Primary** domain — see §5. Required. |
| `secondary_architecture` | array (optional) | Other domains this task touches, per §5's cross-cutting rule. |
| `created_at` | ISO-8601 string | |
| `status` | `"pending"\|"in_progress"\|"blocked"\|"done"\|"cancelled"` | |
| `depends_on` | array of task ids | Hard prerequisites; agent must not start until these are `done`. |
| `plan_refs` | array of `{doc, section}` | Pointers into the plan/schema docs this task implements. |
| `goals` | array of `{id, description, plan_ref, status, blocked_reason?}` | See §6. |
| `phases` | object | `{analyze: {file, status}, code: {...}, test: {...}, validate: {...}, result: {...}}` — `status` per phase is `"not_started"\|"in_progress"\|"done"`. |

```json
{
  "id": "000-task-supabase-schema-migration",
  "title": "Apply consolidated Supabase schema, auth, and RLS",
  "architecture": "supabase",
  "secondary_architecture": [],
  "created_at": "2026-09-13T08:00:00Z",
  "status": "in_progress",
  "depends_on": [],
  "plan_refs": [
    { "doc": "00-thinkboard-abstract-plan.md", "section": "§9 Build order — step 1" },
    { "doc": "thinkboard-schema-final.sql", "section": "full file" }
  ],
  "goals": [
    { "id": "g1", "description": "Run thinkboard-schema-final.sql as a clean migration on a fresh project", "plan_ref": "schema-final.sql", "status": "done" },
    { "id": "g2", "description": "Confirm RLS policies protect the direct-client read path per §9", "plan_ref": "§9 RLS", "status": "in_progress" },
    { "id": "g3", "description": "Flag the 3 non-plan-doc decisions (dual identity, two-layer persona, persona_disciplines extensibility) to the team", "plan_ref": "schema-rationale §Decisions not sourced from the plan doc", "status": "pending" }
  ],
  "phases": {
    "analyze": { "file": "analyze.json", "status": "done" },
    "code": { "file": "code.json", "status": "done" },
    "test": { "file": "test.json", "status": "done" },
    "validate": { "file": "validate.json", "status": "in_progress" },
    "result": { "file": "result.json", "status": "not_started" }
  }
}
```

### 4.3 `analyze.json`

| Field | Type | Description |
|---|---|---|
| `task_id`, `phase` | string | `phase` is always `"analyze"`. |
| `started_at` / `completed_at` | ISO-8601 | |
| `sources_read` | array of `{doc, section}` | Every doc/section actually opened — this **is** the "read" step; skipping it is a contract violation. |
| `scope` | string | What this task will and won't touch. |
| `acceptance_criteria` | array of strings, each tagged with a `goal_id` | Ties directly back to `task.json.goals`. |
| `open_questions` | array of strings | Anything ambiguous — in particular anything overlapping the schema rationale doc's "Decisions not sourced from the plan doc" list must be raised here, not silently resolved. |
| `risks` | array of strings | |

```json
{
  "task_id": "000-task-supabase-schema-migration",
  "phase": "analyze",
  "started_at": "2026-09-13T08:00:00Z",
  "completed_at": "2026-09-13T08:20:00Z",
  "sources_read": [
    { "doc": "thinkboard-schema-final.sql", "section": "full file" },
    { "doc": "01-thinkboard-schema-rationale.md", "section": "§0-§10 + Decisions not sourced from the plan doc" }
  ],
  "scope": "Migration + RLS only. No seed data beyond what's already in the SQL file. No gateway or frontend work.",
  "acceptance_criteria": [
    { "goal_id": "g1", "text": "Migration runs clean on a fresh Supabase project with no manual fixes." },
    { "goal_id": "g2", "text": "Every table has RLS enabled and a policy that matches §9's team-membership access model." }
  ],
  "open_questions": [
    "Dual-identity model (profiles + profile_identities) is an implementation decision, not plan-doc-sourced — confirm the dual-login UX it implies before other tasks build on it."
  ],
  "risks": [
    "persona_disciplines extensibility is untested beyond the seeded 'psychology' row."
  ]
}
```

### 4.4 `code.json`

| Field | Type | Description |
|---|---|---|
| `task_id`, `phase` | string | `phase` is always `"code"`. |
| `started_at` / `completed_at` | ISO-8601 | |
| `files_changed` | array of `{path, action, summary}` | `action` ∈ `created\|modified\|deleted`. |
| `decisions` | array of strings | Implementation choices made during coding that weren't already in `analyze.json`. |
| `commands_run` | array of strings | |

### 4.5 `test.json`

| Field | Type | Description |
|---|---|---|
| `task_id`, `phase` | string | `phase` is always `"test"`. |
| `test_files` | array of paths | |
| `commands_run` | array of `{command, result}` | |
| `summary` | `{total, passed, failed, skipped}` | |
| `coverage_notes` | string | Per plan §6: pipeline-stage transitions and the context-warning heuristic are explicitly called out as the parts "hard to eyeball-QA" — a task touching either must not reach `validate` with `summary.total: 0`. |

### 4.6 `validate.json`

| Field | Type | Description |
|---|---|---|
| `task_id`, `phase` | string | `phase` is always `"validate"`. |
| `goal_checks` | array of `{goal_id, status: "pass"\|"fail", notes}` | One entry per `task.json.goals` id — must cover all of them. |
| `cross_check_against_plan` | array of `{doc, section, result}` | Explicit re-read of the relevant plan/schema section to confirm the implementation still matches it. |
| `signed_off` | boolean | Only `true` once every `goal_checks` entry is `"pass"` or its matching goal is `"blocked"`. |
| `issues_found` | array of strings | |

### 4.7 `result.json`

| Field | Type | Description |
|---|---|---|
| `task_id`, `phase` | string | `phase` is always `"result"`. |
| `closed_at` | ISO-8601 | |
| `final_status` | `"done"\|"blocked"\|"cancelled"` | |
| `goals_summary` | `{total, done, blocked}` | Recomputed from `task.json.goals` at close time, not copy-pasted from memory. |
| `artifacts` | array of paths | Files/migrations/PRs this task produced. |
| `follow_up_tasks` | array of task ids | New tasks spawned (e.g. an `open_question` from `analyze.json` that turned into its own task). |

---

## 5. Architecture categorization

Every task declares exactly one **primary** `architecture`, matching the three real
subsystems from the plan's infra decision (§5, §9):

| Value | Covers | Repo location |
|---|---|---|
| `supabase` | Schema/migrations, RLS policies, auth config, Vault secrets, Realtime/storage config | `thinkboard-schema-final.sql`, Supabase project config |
| `backend` | Golang gateway: pipeline orchestration, LLM routing/BYO-key, RAG mixing (20/80), memory separation, the context-warning heuristic | `services/gateway/` |
| `frontend` | Next.js app: kanban board, artifact sheet, OCR/highlight capture, the 3 render modes, feature-folder UI | `apps/web/` |

Cross-cutting tasks (e.g. the artifact-sheet mini-conclusion, which needs both a Go endpoint and
a React `Sheet`) declare their dominant/owning side as `architecture` and list the other in
`secondary_architecture` — they stay **one task, one folder**, not two, unless the two halves
have genuinely independent acceptance criteria and could ship separately.

---

## 6. Goal tracking

- `task.json.goals[].status` ∈ `pending | in_progress | done | blocked` (`blocked` requires
  `blocked_reason`).
- A task cannot enter `validate` while any goal is `pending`/`in_progress` — every goal must be
  `done` or explicitly `blocked` before validation starts, so validation is always checking a
  claim, not an aspiration.
- `validate.json.goal_checks` must contain one entry per goal id — validation isn't complete
  until every goal has been individually checked off, not just the task as a whole.
- `result.json.goals_summary` is recomputed from `task.json` at close time — it is never
  hand-copied — so the two files can't silently drift apart.
- `running-process.json.task_sequence` only carries top-level task `status`, never individual
  goals — full goal detail has exactly one home, `task.json`, to avoid two sources of truth.

---

## 7. Workflow rules, phase by phase

1. **Analyze** — Read `plan_refs` and any existing related code first (this *is* the "read"
   step). Write `scope`, `acceptance_criteria` tied to goal ids, and `open_questions` — anything
   that overlaps the schema rationale's "Decisions not sourced from the plan doc" list gets
   flagged here, never silently decided.
2. **Code** — Implement only what `analyze.json.acceptance_criteria` covers. If scope needs to
   grow mid-task, that growth becomes a new task (`follow_up_tasks` at result time), not a scope
   change in-flight.
3. **Test** — Write/run tests; a task touching pipeline-stage transitions or the context-warning
   heuristic cannot reach `validate` with zero automated tests (plan §6 flags these as the parts
   "hard to eyeball-QA").
4. **Validate** — Check code + tests against every goal and against the relevant plan/schema
   section. `signed_off: true` only once every goal is accounted for.
5. **Result** — Close the task: write `result.json`, flip `task.json.status`, advance
   `running-process.json.current_task_id` to the next task in `task_sequence`.

---

## 8. Recommended initial task backlog

Derived from the plan's own build order (§9: Supabase → Golang gateway skeleton → Next.js) and
SDLC roadmap (§6: one vertical slice before fanning out). Treat this as the starting
`task_sequence`, not a fixed order — later tasks may get re-sequenced once earlier ones close.

| # | Task | Architecture | Plan ref | Depends on |
|---|---|---|---|---|
| 000 | Apply consolidated schema, auth, RLS | `supabase` | §9 build order step 1; full SQL file | — |
| 001 | Gateway skeleton + Analytic stage stub vs. stubbed LLM call | `backend` | §9 step 2 | 000 |
| 002 | Frontend vertical slice: both login types → one pipeline stage → Planned mode only | `frontend` | §6 Code phase; §9 step 3 | 000, 001 |
| 003 | Formalize the context-warning drift heuristic as testable rules | `backend` | §1 core differentiator; §6 Analyze phase | 001 |
| 004 | BYO-key LLM routing + shared-pool fallback | `backend` | §3.1 | 001 |
| 005 | Client-side OCR + highlight capture spike (text-layer + Tesseract.js) | `frontend` | §2 OCR section; §6 Analyze spike | 002 |
| 006 | Artifact sheet + highlight → mini-conclusion | `frontend` (secondary: `backend`) | §3.2 | 002, 005 |
| 007 | Memory separation (4 scopes, ideas/limitations split) | `backend` | §3.3 | 001 |
| 008 | Kanban board (boards/columns/sessions, dnd-kit) | `frontend` | §4 | 002 |
| 009 | Descriptive + Visualize modes (Mermaid / React Flow) | `frontend` (secondary: `backend`) | §3.5 | 002 |
| 010 | Internet/material RAG ratio mixing | `backend` | §3.4 | 001, 007 |
| 011 | Observability + LLM cost dashboard | `backend` | §6 Deploy phase | 001 |
| 012 | Capacity/load test at 450×8h profile | `supabase` (secondary: `backend`) | §5; §6 Deploy phase | 000, 001 |

Open items from the schema rationale's **"Decisions not sourced from the plan doc"** section
(dual-identity model, two-layer persona split, `persona_disciplines` extensibility) are not
separate tasks — they ride as `open_questions` inside task **000**'s and **001**'s
`analyze.json`, to be confirmed with the team before anything downstream depends on them.

---

## 9. Worked example (abbreviated)

`agent-history/running-process.json` while task 000 is mid-validate:

```json
{
  "schema_version": "1.0",
  "updated_at": "2026-09-13T09:40:00Z",
  "current_task_id": "000-task-supabase-schema-migration",
  "current_phase": "validate",
  "history_path": "agent-history/000-task-supabase-schema-migration",
  "status": "in_progress",
  "task_sequence": [
    { "id": "000-task-supabase-schema-migration", "architecture": "supabase", "status": "in_progress", "title": "Apply consolidated schema, auth, RLS" },
    { "id": "001-task-gateway-analytic-stage-skeleton", "architecture": "backend", "status": "pending", "title": "Gateway skeleton + Analytic stage stub" },
    { "id": "002-task-frontend-vertical-slice", "architecture": "frontend", "status": "pending", "title": "Login → one stage → Planned mode" }
  ]
}
```

`agent-history/000-task-supabase-schema-migration/task.json` and `analyze.json` are shown in
full in §4.2 and §4.3 above — `code.json`, `test.json`, `validate.json`, and `result.json` follow
the same shape from §4.4–§4.7, populated as each phase actually completes.
