# TODO — `agent-thinking/todo` workflow

This file explains how human requests become tracked implementation work in this repo, via
`agent-thinking/todo/` and its index, `agent-thinking/tracking-todo.json`.

## `agent-history` vs `agent-thinking`

- **`agent-history/`** — what an agent *has been doing* / past history. One folder per task
  (`NNN-task-<slug>/`), each with its own `analyze.json` / `code.json` / `test.json` /
  `validate.json` / `result.json` phase files (see `02-working.md`). Structure is more complex
  because it's a full audit trail of *execution*.
- **`agent-thinking/todo/`** — how a raw human prompt becomes a contract-based prompt that's easy
  to implement correctly. One folder per todo (`NNN-todo-<slug>/contract.json`) holding just
  `id`, `name`, `human_prompt` (verbatim ask) and `translated_prompt_agent` (the same ask,
  rewritten as concrete steps against the actual codebase). This is *pre-execution* — it exists so
  the agent building `translated_prompt_agent` has **already read `README.md`** and the plan docs
  before an implementation task is ever opened, instead of re-deriving context mid-task.

## `agent-thinking/tracking-todo.json`

The index of every todo contract created under `agent-thinking/todo/`. One entry per contract:

| Field | Description |
|---|---|
| `id` | Matches the todo folder name, e.g. `005-todo-swagger-backend-api`. |
| `name` | Short human-readable title. |
| `goal` | One-line statement of what the todo is trying to achieve. |
| `todo_path` *(optimize_for_agent)* | Plain relative path to the `contract.json`, e.g. `agent-thinking/todo/005-todo-swagger-backend-api/contract.json`. Kept as a bare path, not prose, so an agent can resolve it programmatically instead of parsing free text. |
| `agent_history` *(optimize_for_agent)* | Plain relative path to the matching `agent-history/NNN-task-<slug>/` folder once the todo is picked up for implementation, or `null` while it's still unstarted. Same "path, not prose" rule. |
| `status` | `"pending"` \| `"in_progress"` \| `"done"` — local status of the todo itself (separate from the `agent-history` task's own `status`). |

```json
{
  "schema_version": "1.0",
  "updated_at": "2026-09-14T00:00:00Z",
  "tracked_todos": [
    {
      "id": "005-todo-swagger-backend-api",
      "name": "Add Swagger to backend and implement api.json",
      "goal": "...",
      "todo_path": "agent-thinking/todo/005-todo-swagger-backend-api/contract.json",
      "agent_history": null,
      "status": "pending"
    }
  ]
}
```

## Connection to `agent-history/running-process.json`

The two files are sequential, not parallel:

```
tracking-todo.json  →  running-process.json
(agent-thinking)        (agent-history)
```

1. A human prompt is turned into a contract and registered in `tracking-todo.json` first
   (`agent_history: null`, `status: "pending"`) — this is the intake queue.
2. When an agent actually starts implementing a todo, it opens a new
   `agent-history/NNN-task-<slug>/` folder per `02-working.md` and adds it to
   `running-process.json`'s `task_sequence` — this is execution tracking.
3. At that point the corresponding `tracking-todo.json` entry is updated: `agent_history` is
   filled in with the new task folder's path, and `status` moves to `"in_progress"`.

`tracking-todo.json` never replaces `running-process.json` — it's what feeds it. A todo is the
contract; the `agent-history` task is the record of doing it.
