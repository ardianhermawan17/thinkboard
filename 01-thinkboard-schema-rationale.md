# ThinkBoard — Schema Rationale & Plan Cross-Reference

**Purpose:** This document maps every object in `thinkboard-schema-final.sql` to the section of `02-thinkboard-abstract-plan.md` that specifies it. It exists so an implementing agent can verify *why* a table/column/constraint exists without re-deriving intent from the SQL alone. Where a schema decision is **not** explicit in the plan doc (extends it, or comes from earlier project discussion), that is called out explicitly under "Decisions not sourced from the plan doc" — treat those as design choices to validate, not settled spec.

Read order: this doc assumes both `02-thinkboard-abstract-plan.md` and `thinkboard-schema-final.sql` are available alongside it.

---

## Quick-reference table

| Schema object | Plan section | One-line rationale |
|---|---|---|
| `pipeline_stage` enum | §2 core pipeline table | Enum values = the 7 named stages (scope_anchor is stage 0, a pre-stage) |
| `pipeline_runs`, `pipeline_stages` | §2 | One row per run, one row per stage per run (`unique(run_id, stage)`) |
| `points`, `point_conclusions` | §2 (Analytic → Result rows) | `points` = "list of atomic points"; `temperature` = §1's Lean-Canvas confidence score |
| `sessions.initial_question` | §1 "initial question (the scoping statement)" | The scope anchor |
| `sessions.scope_embedding` | §1 context warning; §6 "write down the exact context-warning heuristic" | Drift-comparison baseline |
| `context_warnings` | §1, §2 "side-check on every new turn" | Persisted record of a drift trip |
| `sessions.internet_ratio` (default 0.20) | §3.4 | The 20/80 slider, adjustable per session |
| `sessions.message_cap`, `messages.is_summarized` | §3.3 conversation memory | Hard cap + "older turns summarized into Research(scoped)" |
| `run_renderings` (unique per run+mode) | §3.5 three modes | One pipeline run backs Planned/Descriptive/Visualize without rerunning |
| `run_renderings.diagram_kind` (mermaid/flow) | §3.5 library picks | Matches the two recommended libs; tldraw/Excalidraw excluded |
| `artifacts.slot` (main/note) + unique-per-slot index | §3.2 "up to 2 optional artifacts" | DB-level enforcement of the 2-artifact cap |
| `highlights.extraction` (text_layer/ocr) | §2 OCR section | Selection/Range API vs Tesseract.js, encoded as data |
| `highlights.confidence` | §2 "noticeably weaker on handwriting" | Backs a manual-correction fallback UI |
| `highlights.weight`, `highlight_term_weights` | §2 "weight those tokens higher per-user" | Per-highlight + persistent cross-session term boost |
| `mini_conclusions` | §3.2 "auto mini-conclusion chip" | 1:1 with a highlight, not folded into `point_conclusions` |
| `sources.kind`, `sources.embedding` | §3.4 RAG mix | internet/material split backing the 20/80 vector retrieval |
| `sources.bias_note`, `sources.credibility` | §3.5 Descriptive mode | "sources and bias/limitations spelled out in prose" |
| `memory_entries` (single table, `scope` enum) | §3.3 "four distinct stores" | Four scopes collapsed into one table with an owner-check constraint |
| `memory_entries.kind` (idea/limitation) | §3.3 "split... into ideas and limitations" | Backend split for the initial-memory scope |
| `user_llm_keys.vault_secret_id` | §3.1 BYO-key redesign | Raw key never stored; only the Go gateway resolves it |
| `llm_requests.key_source` (user/shared) | §3.1 "uses it first, then falls back to a shared... pool" | Fallback logic as data |
| `llm_requests.cost/latency_ms/*_tokens` | §6 Deploy: "cost dashboard for LLM spend by team" | Fields the observability dashboard would query |
| RLS policies + `security definer` helpers | §9 "Golang gateway... bypasses RLS"; "simple reads... go directly... via RLS" | RLS protects the client-direct read path only |
| `persona_disciplines`, seeded `psychology` row | earlier discussion (not in plan doc) | Extensibility for future per-profession personas |
| `profiles` + `profile_identities` | §3.1 two login paths (partial — see note below) | Dual-login resolved around a Supabase constraint the plan doc doesn't address |
| `user_personas` / `team_personas` split | earlier discussion (not in plan doc) | Two-layer persona model; plan §1 only describes personas generically |

---

## 0. Enums

`pipeline_stage` is a literal transcription of the **§2 pipeline table**: `scope_anchor, analytic, research, validation, questioning, research_scoped, result`. `scope_anchor` sits at position 0 because the plan calls it a "pre-stage," not stage 1 of the pipeline proper. Making this an enum (not free text) is what lets `pipeline_stages` carry a `unique(run_id, stage)` constraint — one row per stage per run, matching §1's "fixed set of processing stages."

`source_kind` (`internet`/`material`) and `sessions.internet_ratio` implement **§3.4**'s 20/80 slider. `memory_scope` (`persona/group/conversation/initial`) and `memory_kind` (`idea/limitation`) implement **§3.3**. `diagram_kind` (`mermaid/flow`) implements **§3.5**'s library choice — no `tldraw` or `excalidraw` value exists, consistent with tldraw being rejected outright (license-key/watermark) and Excalidraw being offered only as an alternative, not the chosen path.

## 1. Identity

**§3.1** states the real constraint the plan is solving around: there's no way to externally query whether an email holds a ChatGPT Plus subscription, so the plan replaces "check subscription" with bring-your-own-key. That redesign lives in the LLM layer (§8 of the schema), not here.

What this section solves — and what the plan doc does *not* spell out — is that Supabase's `auth.users` allows exactly one email per row, so two login paths (internal + individual) can't collapse into a single auth row. `profiles` (one durable identity) + `profile_identities` (multiple auth rows mapped to it, `kind internal/individual`, one `is_primary` per profile) is the schema's answer to that gap. **This is an implementation decision, not a plan-doc-sourced one** — flag it if the agent needs to revisit the dual-login UX.

`current_profile_id()` exists to support **§9**'s stated build order: "Supabase — schema, auth, RLS first."

## 2. Personas

**§1** describes personas only generically: "tag each idea/session with a distinct voice or stakeholder lens." The schema's two-table split — `user_personas` (professional lens, `discipline_id`-typed, nullable owner for shared-library vs. private) and `team_personas` (one active row per team via `team_personas_one_active`, a `guard_prompt`, `applies_to_stages` defaulting to `{questioning}`) — is **more detailed than the plan doc specifies**. That split (a "generate" layer vs. a "constrain" layer) and its default attach point at Questioning came from earlier project discussion, not from `02-thinkboard-abstract-plan.md` itself.

What *is* plan-sourced: `applies_to_stages pipeline_stage[]` only makes sense because §2's pipeline is a fixed, named stage sequence a persona can bind to.

## 3. Boards & sessions

`boards → board_columns → sessions` is the Kanban structure from **§4** ("Kanban board for organizing sessions/ideas per team"). Sessions *are* the cards — `column_id` + `position` drive the drag-and-drop ordering the plan assigns to dnd-kit.

Within `sessions`:
- `initial_question` — the scope anchor (§1, §2's scope_anchor row).
- `scope_embedding vector(1536)` — the drift-comparison baseline that makes the context warning computable (§1, §6's call to formalize the drift heuristic as testable rules).
- `internet_ratio numeric(3,2) default 0.20` — §3.4's default, adjustable per session as required.
- `message_cap int default 40` — §3.3's conversation-memory cap.
- `default_mode session_mode` — §3.5's three modes.

`messages.token_count` + `is_summarized` exist for §3.3's cap mechanic specifically: once the cap hits, older turns get summarized into `research_scoped` rather than kept verbatim; `is_summarized` marks a message as already folded into that summary.

`context_warnings` (`drift_score`, `detected_topic`, `status open/acknowledged/dismissed`) persists every trip of §1's core differentiator — the side-check described in §2 that "surfaces a warning before the pipeline processes it."

## 4. Pipeline

The most directly plan-sourced section. `pipeline_runs → pipeline_stages → points → point_conclusions` mirrors **§2's table** structurally: one run per execution, one row per stage with generic `input`/`output jsonb` (stage output shapes differ), `points` as Analytic's "list of atomic points," and `point_conclusions.temperature` as §1's per-point confidence/risk score.

`points.parent_point_id` (self-referencing) is not explicit in the plan text but follows from it: Analytic decomposes input into discrete points, and Questioning can spawn sub-points from a flagged weak point.

`point_conclusions.open_question` is written by Questioning per the pipeline table's "Points + open questions" output, and is what `research_scoped` reads as input.

`run_renderings` implements **§3.5** directly — the schema's own comment states "a single thinking run can be rendered three ways without re-running the pipeline." The `rendering_payload` check constraint (visualize needs `diagram_spec`+`diagram_kind`; other modes need `content_md`) enforces the plan's distinction between Descriptive (prose with sources/bias) and Visualize (diagram syntax, explicitly "not generated images").

## 5. Artifacts, highlights, term weighting

`artifacts.slot` (`main`/`note`) plus the `artifacts_one_per_slot` unique index enforces **§3.2**'s "up to 2 optional artifacts side by side: Main Thinking + (PDF) note" as a DB constraint, not just a UI rule.

`highlights.extraction` (`text_layer`/`ocr`) encodes **§2**'s OCR approach directly: browser Selection/Range API for text layers, Tesseract.js for scanned pages. `confidence numeric(4,3)` exists because the same section flags Tesseract as weaker on handwriting — confidence is what a manual-correction fallback would key off.

`highlights.weight` and `highlight_term_weights` implement "weight those tokens higher per-user" (§2) — `weight` is the per-highlight boost; `highlight_term_weights` is the persistent, cross-session per-user term boost ("terms this person highlights often get boosted"). This is effectively §3.3's persona-memory idea applied at term granularity.

`mini_conclusions`, 1:1 with a highlight, is §3.2's "auto mini-conclusion chip" from the scoped mini-summarization call — kept as its own table because it treats the highlight as a variable, not a full pipeline rerun.

## 6. Sources

`source_kind` (`internet`/`material`) plus `sources.credibility` and `sources.bias_note` implement two plan requirements at once: the 20/80 mix (§3.4) and Descriptive mode's "sources and bias/limitations spelled out in prose" (§3.5) — the schema's own comment states this: "bias data for Descriptive mode." `point_sources.weight` is the per-point source attribution the pipeline table's Research-stage output ("Point + sources") implies but doesn't name.

`sources.embedding` + the `hnsw` index is what makes the 80%-material side of the ratio a real RAG lookup: §3.4 calls this "top-k chunks pulled 80% from the session's vector store of uploaded artifacts."

## 7. Memory

One `memory_entries` table with a `scope` enum — rather than four physical tables — deliberately collapses **§3.3**'s "four distinct stores, not one conversation buffer" into one table, with `memory_scope_owner` enforcing which foreign key must be set per scope. `memory_entries.kind` (`idea`/`limitation`) is the literal backend split §3.3 specifies for the initial-memory scope.

## 8. LLM layer

This is where **§3.1**'s BYO-key redesign lives. `user_llm_keys` never stores the raw key — `vault_secret_id` points at Supabase Vault, and the schema comment states "Only the Go gateway ever resolves it," matching §5's architecture (Golang gateway owns orchestration; Supabase is auth/DB/storage). `llm_requests.key_source` (`user`/`shared`) is exactly §3.1's fallback: "the gateway uses it first, then falls back to a shared, rate-limited free/default pool."

`llm_requests` as a whole also backs **§6**'s Deploy-phase requirement: "cost dashboard for LLM spend by team" and "LLM latency/cost per persona/team" — `prompt_tokens`, `completion_tokens`, `cost`, `latency_ms` are the fields that dashboard would query.

## 9. RLS

The section's own comment states the logic: "The Go gateway uses the service role key and bypasses RLS entirely; these policies protect direct client reads." This is a direct implementation of **§9**'s architecture convention: "Let simple reads (kanban state, artifact list) go directly from the Next.js client to Supabase via RLS, bypassing the gateway — it's only in the path for the one thing that's actually complex: running the pipeline." RLS exists specifically because the gateway is not meant to be on the critical path for cheap reads.

`team_personas`'s two policies (everyone reads, only `is_team_leader()` writes) enforce the leader-decides-the-team-persona rule — again, a refinement from earlier discussion, not stated in the plan doc.

## 10. Seed

The seeded `general` + `psychology` personas support the earlier-stated (not plan-doc-sourced) intent that personas "should stay extensible for future per-profession variants... deferred, not needed immediately, but the schema shouldn't block it later." The `psychology` row's populated `attributes` JSON schema demonstrates that extensibility without the plan doc itself calling for a psychology persona.

---

## Decisions not sourced from the plan doc

For an implementing agent: these are internally consistent with the plan but were not specified in `02-thinkboard-abstract-plan.md`. Treat as design choices to confirm with the team, not settled requirements:

1. **Dual-identity model** (`profiles` + `profile_identities`) — solves a Supabase `auth.users` constraint the plan doc doesn't address.
2. **Two-layer persona split** (`user_personas` vs. `team_personas`, "railroad guard" semantics, default attach at Questioning) — plan doc only describes personas generically.
3. **`persona_disciplines`/psychology extensibility** — stated as a future-proofing goal in earlier discussion, not in the plan doc.
