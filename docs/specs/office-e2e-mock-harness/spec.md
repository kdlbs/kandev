---
status: shipped
created: 2026-05-03
owner: cfl
---

# Office: E2E mock harness for task sessions and messages

## Why

The Playwright suite at `apps/web/e2e/tests/office/` boots a real
backend per worker. Tasks are created via real API calls. But
**sessions** — the runtime executions that drive the live-presence
UI (inline running blocks, topbar Working spinner, sidebar live
badge, "ran N commands" derivation) — only exist when an executor
launches an agent. Executors require Docker / agentctl / running
processes and aren't available in CI.

The result is that every E2E scenario that depends on a session in a
specific state has to either:

- spin up a real executor (slow, flaky, requires CI infra), or
- get skipped.

Two scenarios in `live-presence.spec.ts` are currently
`test.skip` for exactly this reason:

- "completed session collapses to one-line summary in the timeline;
  click re-expands"
- "multiple completed sessions render in chronological order (no
  SessionTabs)"

Future tests will hit the same wall — the active-session entry,
streaming chat embed, "ran N commands" derivation, transitions
between RUNNING/WAITING_FOR_INPUT/COMPLETED, and the sidebar
counts all benefit from precise state seeding.

The Jira and Linear integrations already solve this pattern:
test-only routes guarded by an env var, mounted only when present
in the e2e harness, drive the system without real external services.
We extend the same convention to office sessions.

## What

A small set of test-only HTTP routes plus matching e2e helpers,
plus the two unblocked E2E scenarios.

### A. Test-only backend routes

When the env var `KANDEV_E2E_MOCK=true` is set at startup, register a
new route group `/api/v1/_test/...` that exposes:

- `POST /api/v1/_test/task-sessions` — body
  ```
  {
    "task_id": "...",
    "state": "RUNNING" | "WAITING_FOR_INPUT" | "COMPLETED" | "FAILED" | "CANCELLED",
    "agent_instance_id": "...",       // optional; defaults to task's assignee
    "started_at":   "ISO 8601 ...",   // optional; defaults to now
    "completed_at": "ISO 8601 ...",   // optional; required for terminal states
    "command_count": 0                 // optional; if > 0, helper inserts that many tool_call messages
  }
  ```
  Inserts a row directly into `task_sessions`. Returns the created
  session with its server-generated ID. Also publishes the
  `session.state_changed` WS event so subscribers update.

- `POST /api/v1/_test/messages` — body
  ```
  {
    "session_id": "...",
    "type": "message" | "tool_call" | "tool_output" | ...,
    "content": "...",
    "metadata": { ... }   // optional, opaque
  }
  ```
  Inserts a row directly into the messages table and publishes
  `session.message.added` so the streaming chat embed updates live.

The routes are mounted in `cmd/kandev/main.go` only when the env
var is set; production builds never expose them.

### B. e2e helpers

In `apps/web/e2e/helpers/api-client.ts` (or a sibling
`e2e-mock-client.ts` if separation reads cleaner):

```ts
seedTaskSession(taskId: string, opts: SeedSessionOpts): Promise<{ session_id: string }>
seedSessionMessage(sessionId: string, opts: SeedMessageOpts): Promise<void>
seedToolCallMessages(sessionId: string, count: number): Promise<void>
```

`seedToolCallMessages` is sugar over `seedSessionMessage` looped
`count` times with `type: "tool_call"` and a synthetic command
name — useful for driving the "ran N commands" derivation.

The `office-fixture.ts` exports these helpers directly via the
`apiClient` fixture so existing tests get them for free.

### C. Test fixture env wiring

The Playwright global setup (`apps/web/e2e/global-setup.ts`)
already starts a backend per worker. It must set
`KANDEV_E2E_MOCK=true` in the spawned backend's environment so the
routes are registered. Add an explicit assertion in the fixture
that confirms the test routes are reachable on startup so an
accidental misconfiguration fails loudly instead of silently
turning all session-driven tests into flake.

### D. Replace the two `test.skip` scenarios

Once the helpers exist, update
`apps/web/e2e/tests/office/live-presence.spec.ts`:

- "completed session collapses to one-line summary":
  - Create task. Seed a COMPLETED session via `seedTaskSession`
    with `started_at` 30s ago and `completed_at` now. Seed 3
    tool-call messages.
  - Assert the timeline entry renders collapsed with
    *"... worked for 30s · ran 3 commands"*.
  - Click the entry. Assert the embedded chat panel becomes
    visible.
- "multiple completed sessions in chronological order":
  - Seed 3 COMPLETED sessions with monotonically increasing
    `started_at`. Assert the entries render in that order in the
    DOM.

### Out of scope

- A general "fake everything" mode. Only sessions and messages get
  test routes. Tasks already have public APIs that work fine.
- Routes for arbitrary DB writes. Each route validates inputs (real
  task ID exists, state is in the canonical set, etc.) so tests
  can't accidentally corrupt the DB.
- Production-time behaviour. The env var is set only by the e2e
  harness; production deployments never set it.
- Replacing the existing mock-agent setup
  (`apps/backend/cmd/mock-agent/`). That's about driving real
  executor protocols; this is about bypassing executors entirely
  for UI tests.

## Acceptance

1. With `KANDEV_E2E_MOCK=true`, a `POST /api/v1/_test/task-sessions`
   creates a row in `task_sessions` with the requested state and
   timestamps, and a `session.state_changed` WS event fires.
2. With the env var unset (production build), the same request
   returns 404 — the route group is not mounted.
3. The Playwright fixture exposes `apiClient.seedTaskSession` and
   `apiClient.seedToolCallMessages`.
4. The two previously-skipped scenarios in
   `live-presence.spec.ts` now run with real assertions
   and pass reliably (10 consecutive `--repeat-each=3` runs green).
5. Existing E2E tests continue to pass — the test routes don't
   pollute non-test code paths.
