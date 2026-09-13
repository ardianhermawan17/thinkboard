# ThinkBoard — abstract plan & delivery roadmap

*A per-team AI thinking board: OCR/highlight-weighted input, a six-stage validation pipeline, and a Lean-Canvas-style "temperature" read on every conclusion.*

---

## 1. Concept

ThinkBoard treats team ideation like a supply chain: raw input (a note, a highlighted PDF, a scanned page) moves through a fixed set of processing stages and comes out the other side as a scored, visualized conclusion — rather than a chat log someone has to re-read to extract meaning.

**The differentiator isn't the pipeline — it's the guardrail.** Every session is anchored to an *initial question* (the scoping statement). As the conversation grows, ThinkBoard checks new input against that anchor and raises a **context warning** when the session is drifting into a second, unrelated topic — the single most common failure mode of long AI chat threads, and the one existing tools don't address. This is worth protecting as the product's core loop; everything else (OCR, personas, kanban UI) is in service of keeping sessions scoped enough that the pipeline's output stays trustworthy.

**Personas** tag each idea/session with a distinct voice or stakeholder lens, so that when multiple teams run sessions in parallel, outputs don't converge into the same generic answer.

**Temperature** is a Lean-Canvas-style confidence/risk read attached to each per-point conclusion — not a single "is this good" score, but a quick visual signal for how validated vs. speculative each point still is.

---

## 2. Core pipeline (backend)

| Stage | Input | What happens | Output |
|---|---|---|---|
| **Scope anchor** (pre-stage) | Initial question + OCR-highlighted text | Sets the topic boundary; highlighted words get a per-user weight bump | Scoped prompt context |
| **Analytic** | Scoped prompt | Breaks the raw input into discrete claims/points | List of atomic points |
| **Research** | Atomic points | Pulls supporting material at the internet/material ratio (default 20/80) | Point + sources |
| **Validation** | Point + sources | Checks each claim against sources, flags unsupported ones | Point + confidence flags |
| **Questioning** (2nd-pass validation) | Validated points | Actively tries to break weak points — the adversarial pass | Points + open questions |
| **Research (scoped)** | Open questions | Narrower, targeted follow-up research — only for what Questioning flagged | Resolved or still-open points |
| **Result** | Resolved points | Per-point conclusion + temperature score | Final scored output |

Throughout, the **context warning** runs as a side-check on every new turn: if new input's topic distance from the scope anchor exceeds a threshold, the UI surfaces a warning before the pipeline processes it, rather than silently absorbing scope creep.

**On OCR/highlight weighting specifically:** run this entirely client-side, as specified.
- Text layers (PDF.js-rendered PDFs, HTML artifacts) — use the browser's native Selection/Range API. No ML needed; you get the highlighted string directly.
- Scanned/image pages — Tesseract.js (WebAssembly Tesseract port) runs OCR fully in-browser, which keeps documents off the server and avoids server load entirely, matching the requirement. Combine bounding-box output with a freehand "highlighter" gesture (pointer/stylus stroke intersecting word boxes) to determine which recognized words were highlighted, then weight those tokens higher per-user.
- Caveat: Tesseract handles printed/typed text well but is noticeably weaker on handwriting — budget a manual-correction fallback for handwritten highlights rather than assuming OCR accuracy there.

---

## 3. Feature spec

### 3.1 Access & LLM routing
Two login paths (internal email / individual email) as specified. One redesign is needed: **there is no way to query whether an arbitrary email has a ChatGPT Plus subscription** — that status is only visible inside the account holder's own ChatGPT settings, and API access is a separate product from the ChatGPT Plus web subscription. Replace "check if email is subscribed" with a **bring-your-own-key** pattern: user optionally supplies their own API key (validated with a cheap test call); the gateway uses it first, then falls back to a shared, rate-limited free/default pool. Same UX goal (prioritize free usage, let power users use their own paid access), technically real.

### 3.2 Artifact sheet
shadcn `Sheet` component holding up to 2 optional artifacts side by side: "Main Thinking" + "(PDF) note". Highlighting text inside either artifact fires a scoped, cheap mini-summarization call (only the highlighted span as context, not the full document) that produces an auto mini-conclusion chip — treating the highlight as a variable rather than re-running the full pipeline.

### 3.3 Memory separation
Four distinct stores, not one conversation buffer:
- **Persona memory** — persists across sessions; the voice/lens for a given idea track.
- **Group thinking memory** (optional) — shared across a team's sessions.
- **Conversation memory** — session-scoped, hard-capped (turn count or token budget) specifically to bound hallucination risk; once the cap is hit, older turns get summarized down into the Research(scoped) stage rather than kept verbatim.
- **Initial custom thinking memory** — the user's pinned starting concept, split on the backend into two labeled fields: `ideas` and `limitations`, so the pipeline can reason about what's proposed vs. what's explicitly ruled out.

### 3.4 Internet vs. material ratio
A retrieval-mix slider, default 20/80 (web search : uploaded material). Implementation-wise this is a RAG weighting parameter — top-k chunks pulled 80% from the session's vector store of uploaded artifacts, 20% from live web search, merged before the Research stage. Adjustable per session, not fixed.

### 3.5 Three modes
- **Planned mode** — the structured pipeline view (the flowchart above).
- **Descriptive mode** — same conclusions, but with sources and bias/limitations spelled out in prose.
- **Visualize mode** — diagram output, not generated images. Two library recommendations:
  - **Mermaid.js** for anything the pipeline itself emits as diagram syntax (deterministic, no drag-and-drop needed, cheap to render).
  - **React Flow** (`@xyflow/react`) for anything the user builds/edits by hand — MIT-licensed, no watermark, free for commercial use.
  - Avoid **tldraw** for this: since SDK 4.0 it requires a license key in production, and the free "hobby" license keeps a visible "made with tldraw" watermark — only a paid commercial license removes it, which is a bad fit for an internal enterprise tool.
  - **Excalidraw** (MIT, fully open source) is a solid alternative if a hand-drawn whiteboard feel fits Visualize mode better than a graph editor.

---

## 4. UI/UX

Assume a ChatGPT-style conversation pane combined with a Kanban board for organizing sessions/ideas per team — tablet and desktop are the priority form factors, phone is supported but secondary. Practically: dnd-kit or similar for the board, shadcn `Sheet` for artifacts, and pointer/stylus events (not mouse-only) for the highlight-drawing interaction on tablets.

---

## 5. Infrastructure & capacity plan

**Target:** ≥450 concurrent users, ≥8 hours/day of active prompting each.

**Realtime connections.** Supabase Realtime's concurrent-connection ceiling is 200 on Free, 500 on standard Pro, and 10,000 on Pro with the spend cap removed or on Team. At 450 concurrent users, standard Pro (500) leaves almost no margin — each user likely holds more than one channel (kanban sync, chat stream, presence), which multiplies real connection count well past 450. Plan for the no-spend-cap Pro tier or Team plan.

**Database connections.** Compute size sets the ceiling: Micro supports 60 direct/200 pooler connections, Small 90/400, Medium 120/600, Large 160/800, XL 240/1,000. Your Golang gateway holds its own connection pool on top of client traffic, so budget at least Medium, likely Large once the gateway's pool is accounted for. Use the transaction-mode pooler (port 6543), not direct/session mode, for anything serverless or high-connection-count.

**Stack decision: Next.js + Supabase, Next.js + Golang, or hybrid?**

| | Next.js + Supabase only | Next.js + Golang only | Hybrid (recommended) |
|---|---|---|---|
| Speed to MVP | Fastest — auth/DB/storage/realtime out of the box | Slowest — hand-roll everything | Fast — commodity parts stay managed |
| Custom pipeline logic (6 stages, RAG mix, context-warning) | Awkward — lives in Edge Functions with execution-time limits | Full control, goroutines handle many concurrent streaming sessions cheaply | Full control, isolated in one service |
| Ops burden | Lowest | Highest | Moderate — one Go service to run |
| Fit for 450×8h streaming load | Realtime/Edge Function limits bite first | Handles it natively | Handles it natively where it matters |

**Recommendation:** Next.js client (shadcn UI, client-side OCR/diagram libraries) → a dedicated **Golang orchestration gateway** that owns the six-stage pipeline, streaming responses, RAG mixing, memory separation, and the context-warning heuristic → **Supabase** for auth, Postgres, file storage, and (optionally) Realtime for kanban/presence sync. This keeps the genuinely differentiated logic (the pipeline) under full control while not reinventing auth/storage/DB.

*Note: this is a different call than the plain-Supabase decision made for the Super P3MD superapp — that app was mostly RLS-guarded CRUD; ThinkBoard's sustained streaming load and multi-stage orchestration logic are different enough to justify the added Go service this time.*

---

## 6. SDLC roadmap

### Analyze
- Requirements workshop with a pilot team: define personas, the ideas/limitations schema, and success metrics for the temperature score.
- Technical spikes: client-side OCR + highlight capture; Mermaid/React Flow rendering in Visualize mode.
- Validate the 450×8h capacity assumption against real pilot usage, not just the target number.
- Write down the exact context-warning heuristic (what counts as scope drift) as testable rules, since it's the product's core differentiator.

### Code
- Monorepo: Next.js app + Golang gateway service, shared CI, infrastructure-as-code for both.
- Build one full vertical slice first: login (both types) → one pipeline stage → Planned mode only. Get this demoable before fanning out.
- Iterate outward: artifact sheet + highlight capture → memory separation → kanban board → remaining pipeline stages → Descriptive and Visualize modes → internet/material ratio control.
- Prioritize automated tests on pipeline stage transitions and the context-warning logic — that's the part that's hard to eyeball-QA and the part most worth protecting.

### Deploy
- Staged rollout: single pilot team → phased by team → full 450 users. Never all at once.
- Load-test the actual profile (sustained streaming sessions over 8 hours, not short bursts) against Supabase and gateway capacity before the final team joins.
- Observability from day one: concurrent connections, LLM latency/cost per persona/team, memory store growth.
- Post-launch: cost dashboard for LLM spend by team, and revisit the default 20/80 internet/material ratio based on real usage rather than the initial guess.

---

## 7. Open risks to validate with the pilot team

- Whether OCR accuracy on scanned/handwritten material is good enough without a manual-correction step.
- Whether the context-warning heuristic produces false positives often enough to be annoying rather than useful.
- Whether the BYO-API-key model gets meaningful adoption, or whether the shared free-tier pool ends up carrying most of the 450-user load (which changes the cost model significantly).

---

## 8. Sources

- Supabase Realtime connection limits by plan — https://supabase.com/docs/guides/realtime/limits
- Supabase compute add-on connection limits by size — https://supabase.com/docs/guides/platform/compute-add-ons
- Tesseract.js (browser OCR, WebAssembly) — https://github.com/naptha/tesseract.js
- tldraw SDK license key requirement — https://tldraw.dev/sdk-features/license-key
- Excalidraw license (MIT) — https://github.com/excalidraw/excalidraw
- React Flow / xyflow license (MIT) — https://reactflow.dev
- ChatGPT Plus subscription status is account-only, not externally queryable — https://help.openai.com/ja-jp/articles/6950777-chatgpt-plus-

---

## 9. Architecture & implementation conventions (for AI-agent-driven build)

### Build order
1. **Supabase** — schema, auth, RLS first. Every type contract flows from here.
2. **Golang gateway** — skeleton with one pipeline stage (Analytic) against a stubbed LLM call, proving the orchestration shape before UI investment.
3. **Next.js frontend** — built against the real (if minimal) API surface from steps 1–2, not mocks.

### Repo layout
Single repository — an AI agent reasons far better with everything visible in one workspace than across separate checkouts.
```
thinkboard/
  apps/
    web/                 # Next.js app
  services/
    gateway/             # Golang orchestration service
  packages/
    contracts/           # Shared types: generated from Supabase schema + the gateway's API
```
pnpm workspace covers `apps/web` + `packages/contracts`; `services/gateway` is a plain sibling Go module — no JS workspace tooling needed, it just needs to live in the same repo. Lighter than the Super P3MD monorepo, since ThinkBoard only needs one frontend app.

### Backend: CQRS-lite, not full CQRS
Full CQRS (separate read/write models, event sourcing, projections) earns its complexity with heavy read/write divergence, UI data shaped very differently from storage, or separate teams owning commands vs. queries — none of which apply here yet.
Keep the split, skip the machinery:
```
services/gateway/internal/
  pipeline/        # commands: the 6-stage orchestration, streaming
  session/         # queries: fetch session, history, artifacts
  llm/             # provider clients + BYO-key resolution
  memory/          # persona/group/conversation/initial stores
  contextwarning/  # scope-drift heuristic
```
Let simple reads (kanban state, artifact list) go directly from the Next.js client to Supabase via RLS, bypassing the gateway — it's only in the path for the one thing that's actually complex: running the pipeline.

### Frontend: feature folders + the index/ui/use-ui split
```
features/<feature-name>/
  index.ts              # public barrel — only what other features may import
  <feature-name>.tsx    # composition: wires ui.tsx + the hook together
  ui.tsx                # pure presentational — props in, JSX out, no data fetching
  use-<feature-name>.ts # TanStack Query (server state) + Zustand (cross-feature UI state) + local useState
  <feature-name>.types.ts
```
Same shape as Bulletproof React's feature-folder convention. Good for agent-driven codegen specifically because every feature gets the same four files with the same responsibilities — the agent never re-decides where logic goes.

### State management
- **TanStack Query** — server state, the only thing that talks to Supabase or the gateway.
- **Zustand** — cross-feature client state (active mode, current persona, sheet open/closed, selected card). The clear 2026 default for this tier: no provider, ~3KB, single-store model maps directly onto a `use-ui.ts` hook. Skip Redux Toolkit — different scale problem.
- **URL state** (`nuqs` or `useSearchParams`) — shareable view state: active mode, selected card, open artifact.
- Local `useState` — anything scoped to one component tree.

Start from the latest `create-next-app` scaffold — recent versions ship an AI-ready project structure by default, and Next.js 16.3's client-state-preserving instant navigations reduce how much needs to live in a global store just to survive route changes.

### Sources (this section)
- State management landscape 2026 — https://www.pkgpulse.com/guides/zustand-vs-jotai-vs-nanostores-micro-state-management-2026
- Bulletproof React feature-folder pattern — https://github.com/alan2207/bulletproof-react
- When (not) to use CQRS — https://dev.to/lufc/when-to-use-cqrs-on-your-clean-arch-net-project-307o
- Next.js building for an agentic future / 16.3 — https://nextjs.org/blog
