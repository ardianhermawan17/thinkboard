# ThinkBoard — Golang Gateway Folder Architecture (`03-backend-folder-architecture.md`)

**Scope:** folder layout, layering rules, and concurrency model for `services/gateway/` — the Golang
orchestration service from plan §5/§9. This doc governs *structure*. Product intent stays in
`00-thinkboard-abstract-plan.md`, data contracts in `thinkboard-schema-final.sql` /
`01-thinkboard-schema-rationale.md`, process in `02-working.md`.

**Stack:** Go 1.23+, **Gin** (HTTP + SSE), **pgx/v5** against Supabase Postgres via the transaction
pooler, **CQRS-lite per plan §9** with light DDD tactical patterns inside the write side,
`errgroup`-bounded concurrency throughout.

This layout is the plan's §9 package list, expanded — not replaced. The five packages it names
(`pipeline/ session/ llm/ memory/ contextwarning/`) appear verbatim; everything added beyond them is
listed and justified in §10.

---

## 1. What this service owns

| Owns (gateway) | Does **not** own (client → Supabase direct) |
|---|---|
| The 7-stage pipeline + streaming | Auth issuance, JWT minting, password flows |
| Scope-drift / context-warning heuristic | Kanban state, board/column CRUD, artifact listing (plan §9) |
| RAG mixing (internet/material, §3.4) | File storage of artifacts |
| Memory separation (4 scopes, §3.3) | Client-side OCR / highlight capture (§2 — browser only) |
| BYO-key resolution + shared-pool fallback (§3.1) | Realtime presence / kanban sync |
| LLM cost & latency accounting (§6 Deploy) | |

Plan §9: *"it's only in the path for the one thing that's actually complex: running the pipeline."*
When in doubt about whether an endpoint belongs here, the default answer is no.

**The one consequence that drives everything below:** the gateway uses the **service-role key and
bypasses RLS**. Every rule RLS enforces on the client path must be re-implemented in Go on the
gateway path, or the gateway is a hole straight through the security model. That lives in exactly one
package — `internal/auth` (§5.6) — and nothing else may query `team_members` for authorization.

---

## 2. Top-level layout

```
services/gateway/
├── cmd/
│   └── gateway/main.go          # single binary: Gin router + in-process run supervisor
├── internal/
│   ├── pipeline/                # §9: commands — the 7-stage orchestration, streaming
│   ├── session/                 # §9: queries — fetch session, history, artifacts
│   ├── llm/                     # §9: provider clients + BYO-key resolution
│   ├── memory/                  # §9: persona/group/conversation/initial stores
│   ├── contextwarning/          # §9: scope-drift heuristic
│   ├── auth/                    # RLS replacement for the service-role path (§5.6)
│   ├── httpapi/                 # Gin transport: router, middleware, handlers, DTOs, SSE
│   ├── platform/                # config, pgx pool, telemetry, websearch, embeddings
│   └── shared/                  # kernel types + concurrency primitives + error mapping
├── test/
│   ├── integration/             # testcontainers Postgres + the real Supabase migrations
│   └── fixtures/
├── api/openapi.yaml             # source for packages/contracts codegen
├── Makefile
└── go.mod                       # plain sibling Go module (plan §9 — not in the pnpm workspace)
```

One binary. The pipeline runs as a supervised goroutine pool inside `cmd/gateway`; splitting it into
a second process is a `main.go` change and nothing else, because the orchestrator never touches
`gin.Context`. Don't split it until a deploy of the web tier actually needs to not kill an 8-hour run.

---

## 3. CQRS-lite: what we keep, what we skip

Plan §9: *"Keep the split, skip the machinery."*

| Keep | Skip |
|---|---|
| Write path (`pipeline/`) separate from read path (`session/`) | Command/query bus, generic `Handler[C,R]` interfaces |
| Reads use hand-written SQL → DTO, no aggregate hydration | Separate read database, projections, denormalized views |
| Writes load a `Run`, call methods on it, save | Event sourcing, event store, replay |
| Domain invariants live in pure Go types (`transition.go`, `Temperature`) | Domain-event dispatcher, outbox table, in-process pub/sub |
| Repository interfaces where a fake is genuinely useful for tests | An interface per struct; mock-everything wiring |
| One transaction per write operation | Unit-of-work abstraction layer |

Concretely: a Gin handler calls `pipeline.Service.StartRun(ctx, cmd)` **directly**. No dispatch
indirection, no middleware chain to trace through. Cross-cutting concerns (auth, timeout, recovery,
tracing, rate limit) are Gin middleware, which is where they already belong.

Because there is no event dispatcher, there is **no `domain_events` outbox table** — nothing needs
adding to `thinkboard-schema-final.sql`. Side effects after a write are ordinary function calls in
the service method, inside or after the same transaction, and they're visible in the call stack.

**Where DDD still earns its keep** (and where it doesn't): `pipeline/` and `contextwarning/` get real
domain types with enforced invariants, because the stage machine and the drift heuristic are the two
things plan §6 calls hard to eyeball-QA. `memory/`, `llm/`, and `session/` are thin data access with
a service method on top — giving them aggregates and value objects would be ceremony around CRUD.

---

## 4. Import rules

```
shared  ←  platform  ←  { pipeline, session, llm, memory, contextwarning, auth }  ←  httpapi  ←  cmd
```

| Rule | Enforcement |
|---|---|
| `platform` imports no feature package | depguard |
| `httpapi` calls service methods only — never a store, never raw SQL | depguard + review |
| `pipeline` may call `llm`, `memory`, `contextwarning`, `auth`; the reverse is forbidden | depguard allowlist |
| `session` (read side) imports no feature package — it owns its SQL | depguard |
| Pure rule files (`pipeline/transition.go`, `contextwarning/rules.go`) import stdlib only | lint |
| No `time.Now()` outside `shared/clock` | lint |

`pipeline` sitting above the others is deliberate and is the whole reason this isn't a ball of mud:
dependencies point one way, so the orchestration is testable with fakes for four collaborators and
nothing else needs a fake at all.

---

## 5. Package detail

### 5.1 `internal/pipeline/` — the write side

```
pipeline/
├── service.go          # ENTRYPOINT: StartRun, AdvanceStage, RetryStage, CancelRun, Render
├── run.go              # Run: state + invariants (status, stage cursor, mode). No I/O.
├── stage.go            # Stage type mirroring the pipeline_stage enum 1:1
├── transition.go       # PURE: allowed(from, to) bool — the stage state machine
├── point.go            # Point + parent_point_id tree (schema §4)
├── temperature.go      # Temperature VO, [0,1] invariant enforced in Go, not only by CHECK
├── rendering.go        # planning | descriptive | visualize payload rules (§3.5)
├── orchestrator.go     # drives stages in order; owns the run goroutine
├── fanout.go           # bounded per-point parallelism (§7.3)
├── lease.go            # stage ownership claim via unique(run_id, stage) (§7.6)
├── hub.go              # per-run event broker for SSE attach/reattach (§7.5)
├── deps.go             # the 4 interfaces pipeline needs: LLM, Retriever, Memory, Personas
├── store.go            # Store interface (what Postgres must provide)
├── store_pg.go         # pgx implementation
└── stages/
    ├── registry.go         # map[Stage]Handler — the ONLY declaration of stage order
    ├── scope_anchor.go     # stage 0: scoped prompt context + highlight weight bump
    ├── analytic.go         # → []Point
    ├── research.go         # → Point + sources (20/80 mix)
    ├── validation.go       # → Point + confidence flags
    ├── questioning.go      # adversarial pass; team persona guard attaches HERE by default
    ├── research_scoped.go  # narrower follow-up, only what questioning flagged
    └── result.go           # → conclusion + temperature
```

`registry.go` is the single source of stage order in Go. `stage.go` mirrors the DB enum. Nothing else
may hardcode a sequence — which is what turns plan §6's *"automated tests on pipeline stage
transitions"* into a table-driven test over `transition.go` + the registry, instead of an integration
test that burns seven LLM calls per run.

A new stage is: one file in `stages/`, one registry entry, one transition test. Nothing else.

### 5.2 `internal/session/` — the read side

```
session/
├── service.go      # RunDetail, RunHistory, StageOutput, OpenWarnings, CostSummary
├── dto.go          # flat response structs — no domain types cross this boundary
├── queries.go      # hand-written SQL, straight to DTO. No hydration, no reuse of pipeline/store.
└── messages.go     # the few WRITES that must pass through the gateway (see below)
```

The read side is deliberately thin. Plan §9 routes cheap reads (kanban state, artifact list) straight
from Next.js to Supabase via RLS, so the gateway only exposes queries over state the client can't
safely compute: run progress, stage output, drift history, LLM cost aggregates.

`messages.go` holds the exception on the write side: appending a user turn goes through the gateway
because it must pass the scope-drift gate (§7.4) and may trigger the `message_cap` summarization
rollover (§3.3) before anything is persisted. Board/column/session-card CRUD does **not** — that's
client → Supabase via RLS.

**Rule:** if a new query duplicates something the client can already read through RLS, it doesn't
belong here. Deleting an endpoint is cheaper than the second source of truth it creates.

### 5.3 `internal/llm/`

```
llm/
├── service.go        # Complete / Stream — the only API pipeline sees
├── resolver.go       # BYO key first → shared rate-limited pool (§3.1)
├── vault.go          # ONLY caller of Supabase Vault; resolves user_llm_keys.vault_secret_id
├── usage.go          # batched llm_requests writer (§7.8)
├── ratelimit.go      # per-profile token bucket on the shared pool
└── provider/{openai.go,anthropic.go,compat.go}
```

A resolved API key never leaves this package — `Complete`/`Stream` take a `ProfileID`, not a key.
Resolved secrets sit in a short-TTL in-memory cache, never in a struct that gets logged, zeroed on
revoke. `usage.go` is what makes §6's *"cost dashboard for LLM spend by team"* queryable.

### 5.4 `internal/memory/`

```
memory/
├── service.go     # Persona(), Group(), Conversation(), Initial() — one accessor per scope
├── entry.go       # kind: idea | limitation (§3.3 backend split)
├── summarize.go   # message_cap rollover: older turns → research_scoped input
└── store_pg.go    # single memory_entries table, scope-filtered (schema §7)
```

Four accessors over one table. The separate methods exist so a caller cannot accidentally read the
wrong store — the plan's *"four distinct stores, not one conversation buffer"* enforced by API shape
rather than by a query parameter someone will eventually pass wrong.

### 5.5 `internal/contextwarning/` — the differentiator

```
contextwarning/
├── rules.go       # THE HEURISTIC — pure funcs, zero I/O (plan §6 "testable rules")
├── rules_test.go  # the executable form of that requirement
├── threshold.go   # tunable thresholds as typed config, not magic numbers
├── checker.go     # embed new turn → compare to sessions.scope_embedding → verdict
└── store_pg.go    # context_warnings persistence
```

`rules.go` stays pure and dependency-free so §7's open risk (false-positive rate) can be measured by
running the table against pilot transcripts offline, with no LLM and no database.

### 5.6 `internal/auth/` — the RLS replacement

```
auth/
├── jwt.go          # verify Supabase JWT via JWKS → ProfileID
├── authorizer.go   # CanAccessSession / IsTeamMember / IsTeamLeader
└── cache.go        # membership cache, TTL ≤ 60s, invalidated on role change
```

`authorizer.go` is the Go mirror of the SQL helpers `is_team_member()`, `is_team_leader()`,
`can_access_session()` in schema §9. **Rule:** add an RLS policy to the schema and you add the
matching check here in the same task, cited in `validate.json.cross_check_against_plan`.

### 5.7 `internal/shared/` and `internal/platform/`

```
shared/
├── kernel/       # ProfileID, TeamID, SessionID, RunID, Ratio
├── concurrency/  # pool.go, semaphore.go, supervisor.go, batcher.go, singleflight.go
├── errs/         # domain error → HTTP status mapping (used by httpapi, defined once)
└── clock/

platform/
├── config/       # every bound and timeout in §7 lives here, not as a literal
├── postgres/     # pgxpool construction (§7.9)
├── telemetry/    # otel traces, structured logging, metrics
├── websearch/    # the 20% internet leg
└── embedding/    # vector(1536) generation for scope anchors, sources, memory
```

---

## 6. Transport (Gin)

```
httpapi/
├── router.go              # route table; the only file that knows URLs
├── middleware/
│   ├── recover.go         # gin.CustomRecoveryWithWriter → structured 500, never a stack to client
│   ├── requestid.go
│   ├── otel.go
│   ├── auth.go            # auth.JWT → ProfileID on context
│   ├── authz.go           # auth.Authorizer — the RLS replacement, per route group
│   ├── ratelimit.go
│   └── nogzip.go          # SSE routes MUST skip compression
├── sse/
│   ├── writer.go          # single-owner writer goroutine + buffered channel
│   └── heartbeat.go       # 15s comment ping — proxies kill idle streams long before 8h
├── handler/{run.go,session.go,message.go,memory.go,llmkey.go,health.go}
└── dto/                   # request structs + binding tags ONLY
```

Handler contract — bind, call, map. Three lines of logic:

```go
func (h *RunHandler) Start(c *gin.Context) {
    var req dto.StartRunRequest
    if err := c.ShouldBindJSON(&req); err != nil { fail(c, errs.Invalid(err)); return }
    id, err := h.pipeline.StartRun(c.Request.Context(), pipeline.StartRunCmd{
        SessionID: kernel.SessionID(req.SessionID),
        Mode:      req.Mode,
    })
    if err != nil { fail(c, err); return }
    c.JSON(http.StatusAccepted, dto.StartRunResponse{RunID: id.String()})
}
```

No business rules, no store access, no `if` on domain state. If a handler grows a fourth concern, the
missing piece is a service method.

**Gin specifics that bite:**

- `c.Request.Context()` is cancelled on client disconnect. Use it for queries. Do **not** use it as
  the pipeline run's context — see §7.5.
- Passing `*gin.Context` to a goroutine requires `c.Copy()`. Better: never pass it — extract what you
  need and pass a plain `context.Context`.
- `c.Writer` is not safe for concurrent writes. One writer goroutine per SSE stream, always.
- Register compression per-group, excluding `/v1/runs/:id/stream`.
- `gin.New()` + explicit middleware, not `gin.Default()`; `gin.SetMode(gin.ReleaseMode)` in `main.go`.

---

## 7. Concurrency model

Plan §5's target is **≥450 concurrent users × ≥8h/day of sustained streaming** — a
long-lived-connection problem, not a throughput problem. Every goroutine below is bounded and owned.

### 7.1 Rules

1. No bare `go` outside `shared/concurrency`, an `errgroup`, or the supervisor — each recovers panics
   and has an owner that waits for it.
2. Every goroutine takes a `context.Context` and returns on cancel. No `select` without `<-ctx.Done()`.
3. Every channel has a declared capacity and a documented full-behaviour (block / drop / error).
4. Bounds come from `platform/config`, never literals: `MaxConcurrentRuns`, `MaxPointFanout`,
   `MaxRetrieval`, `SSEBufferSize`.
5. `go test -race ./...` is a merge gate.

### 7.2 Stage sequencing — sequential by data dependency

The 7 stages are a chain (§2's table): each consumes the previous stage's output. `orchestrator.go`
runs them in order, one at a time per run, persisting each `pipeline_stages` row as it completes.
Parallelism does not live here — overlapping Research with Validation breaks both the schema's
`unique(run_id, stage)` semantics and the plan's stated data flow.

### 7.3 Fan-out *within* a stage — where the parallelism actually is

Analytic emits N atomic points. Research, Validation, and Questioning operate **per point** and are
embarrassingly parallel:

```go
g, gctx := errgroup.WithContext(ctx)
g.SetLimit(cfg.MaxPointFanout)          // e.g. 8 — per-run, not global
results := make([]StageResult, len(points))
for i, p := range points {
    i, p := i, p
    g.Go(func() error {
        r, err := h.processPoint(gctx, p)   // ≥1 LLM call + retrieval
        if err != nil { return fmt.Errorf("point %s: %w", p.ID, err) }
        results[i] = r
        return nil
    })
}
if err := g.Wait(); err != nil { return stage.Fail(err) }
```

Indexed writes into a pre-sized slice — no mutex, no append race. `g.SetLimit` caps per-run fan-out;
the global LLM semaphore (§7.7) caps everything across runs. Both are needed: one protects latency
fairness between users, the other protects the provider rate limit.

### 7.4 Retrieval and the scope gate — parallel, ratio-merged, degradable

The 20/80 mix (§3.4) is two independent lookups. Run them concurrently, merge after:

| Leg | Source | Timeout | On failure |
|---|---|---|---|
| material (80%) | pgvector HNSW over `sources.embedding` | 2s | fail the stage — it's the primary leg |
| internet (20%) | web search provider | 1.5s | **degrade**: proceed material-only, record a note on the stage output |

Web search is the slowest, least reliable dependency and contributes the smaller share of context;
blocking a stage on it is the wrong trade. The degradation must be visible — it feeds Descriptive
mode's bias/limitations prose (§3.5).

The context-warning check is different: it **gates** the pipeline (plan §2: *"surfaces a warning
before the pipeline processes it"*), so it must complete before `StartRun` proceeds. Its embedding
call runs concurrently with persisting the user message, then both join.

### 7.5 Streaming, disconnects, and the run hub

A tablet on hotel wifi will drop during an 8-hour session. If the run dies with the socket, the user
loses work and you pay for the tokens twice.

```
POST /v1/runs            → 202 {run_id}   (starts the run on a DETACHED context)
GET  /v1/runs/:id/stream → SSE            (subscribes to the hub; reattachable, replayable)
```

- The run's context is `context.WithoutCancel(reqCtx)` plus its own budget (`WithTimeout`, e.g. 15
  min per run) — **not** the request context.
- `hub.go` keeps `map[RunID]*runTopic`; each topic has a subscriber set under `sync.RWMutex` and a
  bounded ring buffer of recent events for replay-on-reattach. Slow subscribers are **dropped, never
  blocked** — a stalled SSE client must not stall the run.
- Each SSE connection: one writer goroutine owns `c.Writer`, reads a buffered channel
  (`SSEBufferSize`, e.g. 64), flushes per event, exits on `c.Request.Context().Done()`.
- Multi-instance caveat: an in-process hub only works if the SSE request lands on the instance
  running the pipeline. At MVP run one replica, or pin by `run_id` at the load balancer. Beyond that,
  back the hub with Postgres `LISTEN/NOTIFY` or Supabase Realtime — a `hub.go` change that touches
  nothing else, which is why the orchestrator stays ignorant of Gin.

### 7.6 Stage ownership — the schema is already the lock

`pipeline_stages` has `unique (run_id, stage)`. A worker claims a stage by inserting the row with
`status = 'running'`; a unique violation means someone else owns it. No lock table, no Redis. For
run-level exclusivity on retries or orphan reclaim, take
`pg_advisory_xact_lock(hashtextextended(run_id::text, 0))` inside the claiming transaction.

### 7.7 Admission control

Three nested bounds, outermost first:

| Bound | Mechanism | Why |
|---|---|---|
| Concurrent runs per instance | weighted semaphore (`MaxConcurrentRuns`) | protects DB pool + memory |
| Concurrent runs per team | per-key semaphore, TTL-evicted map | one team can't starve the other 449 users |
| In-flight LLM calls per provider | weighted semaphore + per-profile token bucket | provider limits; shared-pool fairness (§3.1) |

Over the limit, prefer enqueueing to rejecting: `pipeline_runs.status` already has `pending`, so
queue the run, return 202, and surface queue position through the hub. Reserve `429` for abuse.

### 7.8 Batched, off-path writes

`llm_requests` is written on every LLM call — hot path, nothing reads it synchronously.
`shared/concurrency/batcher.go` buffers on a channel and flushes every 500ms or 100 rows via `COPY`.
On full buffer: block up to 50ms, then drop with a counter increment — cost telemetry is not worth
failing a user's pipeline over. Flush on shutdown. The same batcher serves `highlight_term_weights`
increments (§2's per-user term boost), which are `ON CONFLICT DO UPDATE` upserts and coalesce
naturally.

### 7.9 Database pool sizing

Supabase compute sets the ceiling (plan §5: Medium = 120 direct / 600 pooler; Large = 160/800). Use
the **transaction-mode pooler on port 6543**, per plan §5.

```go
cfg.MaxConns        = 40             // per gateway instance
cfg.MinConns        = 8
cfg.MaxConnLifetime = 30 * time.Minute
cfg.MaxConnIdleTime = 5 * time.Minute
// REQUIRED under transaction pooling — server-side prepared statements break:
cfg.ConnConfig.DefaultQueryExecMode   = pgx.QueryExecModeExec
cfg.ConnConfig.StatementCacheCapacity = 0
```

Worked budget, Medium compute, 2 replicas: 2 × 40 = 80 pooler connections for the gateway, leaving
~520 for direct client reads via PostgREST. With `MaxConcurrentRuns = 24` and fan-out 8, peak DB
demand per instance stays well under 40 because fan-out work is LLM-bound — each point holds a
connection only for its persist step. **Validate against the real 450×8h profile before the final
team joins** (plan §6 Deploy) rather than trusting this arithmetic.

### 7.10 Graceful shutdown

On SIGTERM: stop accepting new runs → `srv.Shutdown(ctx)` with a 30s drain → emit a `shutdown` SSE
event so clients reconnect → wait for in-flight stages to a deadline → mark unfinished runs
`cancelled` (or leave them `running` for reclaim via §7.6) → flush batchers → close the pool.
`shared/concurrency/supervisor.go` owns the sequence; `main.go` just calls it.

---

## 8. Testing

| Level | Location | What |
|---|---|---|
| Pure unit | `pipeline/transition_test.go`, `contextwarning/rules_test.go` | table-driven, no mocks, no DB |
| Service unit | `pipeline/service_test.go` | fakes for the 4 interfaces in `deps.go` |
| Concurrency | `pipeline/fanout_test.go`, `hub_test.go`, `lease_test.go` | `-race`: cancellation, subscriber drop, lease contention |
| Integration | `test/integration/` | testcontainers Postgres + the real Supabase migrations; stores and RLS-parity checks |
| Contract | `api/openapi.yaml` | drives `packages/contracts` codegen; drift fails CI |

Per `02-working.md` §7.3 and plan §6: **a task touching pipeline-stage transitions or the
context-warning heuristic cannot reach `validate` with `test.json.summary.total: 0`.** Those two areas
are exactly `transition.go` + `stages/registry.go` and `contextwarning/rules.go` — all pure functions
with no I/O, so the rule is cheap to satisfy rather than something to negotiate around.

---

## 9. Build order

Maps onto `02-working.md` §8, task **001** and successors.

| Step | Deliverable | Proves |
|---|---|---|
| 1 | `shared/` + `platform/postgres` + health route + shutdown | wiring, pool, drain |
| 2 | `auth/` + auth/authz middleware | the RLS replacement exists *before* anything bypasses RLS |
| 3 | `pipeline/` domain core: `run.go`, `stage.go`, `transition.go` + tests | the state machine, no I/O |
| 4 | `StartRun` + `analytic` stage vs. a **stubbed** LLM (plan §9 step 2) | orchestration shape |
| 5 | SSE hub + detached run context | the streaming contract the frontend builds against |
| 6 | `llm/` real provider + BYO-key resolution + usage batcher | §3.1, §6 |
| 7 | retrieval + 20/80 mix | §3.4 |
| 8 | remaining 5 stages via the registry | §2 |
| 9 | `contextwarning/` wired as the pre-pipeline gate | §1 differentiator |
| 10 | `memory/` four scopes + cap rollover | §3.3 |

Step 2 before step 4 is deliberate. The moment the gateway makes its first service-role query it has
left the RLS model behind, and retrofitting authorization onto handlers that already work is exactly
how the hole ships.

---

## 10. Additions not in the plan doc

Following `01-thinkboard-schema-rationale.md`'s convention — confirm these; they're choices, not spec.
The five §9 packages are unchanged; these are the additions around them.

1. **Gin.** The plan names no HTTP router. Gin is a fine fit; note SSE needs the manual care in §6
   that `net/http` + a thin mux gives for free.
2. **`internal/auth`.** Not in §9's package list, but unavoidable: §9 states the gateway bypasses RLS,
   which means the gateway must own an equivalent check. Treat as required, not optional.
3. **`httpapi/`, `platform/`, `shared/`.** §9 lists domain packages only; transport, infrastructure,
   and shared primitives need somewhere to live.
4. **`stages/` subpackage inside `pipeline/`.** §9 says `pipeline/` holds the orchestration; splitting
   one file per stage is a readability choice that also makes the registry the single ordering source.
5. **Detached run contexts** (runs survive client disconnect, §7.5). Implied by the 8h streaming
   target but never stated, and it changes cost behaviour — an abandoned run still bills tokens.
   Decide the abandonment policy with the pilot team.
6. **Light DDD inside `pipeline/` and `contextwarning/`** (aggregate-ish `Run`, `Temperature` VO, pure
   rule files) while the other three packages stay thin data access. §9 doesn't specify internal
   structure either way.

---

## 11. Quick rules for an implementing agent

- A new pipeline stage = one file in `stages/` + one registry entry + one transition test. Nothing else.
- Gin handlers bind, call one service method, map the error. No `if` on domain state.
- Never pass `*gin.Context` past the handler.
- Never query `team_members` outside `auth/`. Never resolve a Vault secret outside `llm/vault.go`.
- Never start a goroutine outside `shared/concurrency`, an `errgroup`, or the supervisor.
- Read side returns DTOs and owns its own SQL; it never imports `pipeline`.
- Before adding a gateway endpoint, check whether the client can read it through RLS. If it can, don't.
- No command bus, no event dispatcher, no outbox. If you're reaching for one, re-read §3 first.
