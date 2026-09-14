# 05 — Agent limitations

Hard limits on what a coding agent may do unattended in this repo. These override any other
instruction, saved memory, or standing preference that conflicts with them.

## 1. Git write commands require confirmation

An agent must **ask the user and get an explicit yes before running any git write command** —
`commit`, `push`, `merge`, `pull --rebase`, force-push, branch delete, or `gh pr create`. Read-only
git commands (`status`, `log`, `diff`, `show`) never need confirmation.

This applies every time, not just the first time in a session — one approval does not carry over
to the next write command.

## 2. Writes are only accepted on the shadow branch

A git write is only acceptable when it lands on the current session's own branch (the
**shadow branch**, aka the "conservation" branch — e.g. `claude/*` worktree branches such as
`claude/ponytail-ultra-01b4fa`), never directly on `master`. Landing work on `master` — including a
local fast-forward merge — is a git write like any other and needs the same confirmation as a
`commit` or `push`, treated as at least as risky as a push since it changes the branch other work
builds on.

To ship a change: commit on the shadow branch, confirm, then push and open a PR against `master`
for review — don't merge into `master` directly from an agent session.

## 2.1 Correction on record

An earlier session in this repo fast-forward-merged a shadow branch straight into the main
checkout's `master` without asking first. That was a violation of §2 and is the reason this rule
exists in writing — don't repeat it.

## 3. "Human Mode" — the expected interactive flow

When a user starts a request with **"Human Mode"**, the agent follows this sequence instead of
jumping straight to implementation:

1. User gives the prompt, prefixed or flagged as "Human Mode".
2. Agent reads `README.md` first, for full project context.
3. Agent asks the user clarifying questions to validate what they actually want — do not assume
   intent from a short prompt.
4. Agent implements by first writing the contract in `agent-thinking/todo/` (see
   [`04-TODO.md`](./04-TODO.md)), then tracking execution in `agent-history/` per
   [`02-working.md`](./02-working.md).
5. Agent writes the code and validates it (tests, or manual verification where no test harness
   exists) before reporting the task done.

Steps 4 and 5 still fall under §1 and §2 for any git write they produce.
