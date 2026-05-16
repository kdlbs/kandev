---
status: shipped
created: 2026-05-03
owner: cfl
---

# Office: E2E Mock Harness for Task Sessions and Messages

## Why

The Playwright suite at `apps/web/e2e/tests/office/` boots a real backend per worker. Tasks are created via real API calls. But **sessions** - the runtime executions that drive the live-presence UI (inline running blocks, topbar Working spinner, sidebar live badge, "ran N commands" derivation) - only exist when an executor launches an agent. Executors require Docker / agentctl / running processes and aren't available in CI.

The result is that every E2E scenario depending on a session in a specific state has to either spin up a real executor (slow, flaky, requires CI infra) or get skipped. Future tests for the active-session entry, streaming chat embed, "ran N commands" derivation, state transitions, and sidebar counts all benefit from precise state seeding.

The Jira and Linear integrations already solve this pattern: test-only routes guarded by an env var, mounted only when present in the e2e harness, drive the system without real external services. We extend the same convention to office sessions.

## What

- When `KANDEV_E2E_MOCK=true` is set at startup, the backend SHALL register a `/api/v1/_test/...` route group that lets tests directly seed `task_sessions` and message rows.
- Production builds (env var unset) SHALL NOT mount these routes - the same request returns 404.
- Seed routes SHALL publish the same WS events (`session.state_changed`, `session.message.added`) a real executor would, so subscribers update.
- Input validation SHALL reject corrupting writes (unknown task ID, non-canonical state).
- Playwright fixtures expose typed helpers so tests get the seed API without per-spec boilerplate.

## API surface

### `POST /api/v1/_test/task-sessions`

Inserts a row directly into `task_sessions` and publishes `session.state_changed`.

```json
{
  "task_id": "...",
  "state": "RUNNING" | "WAITING_FOR_INPUT" | "COMPLETED" | "FAILED" | "CANCELLED",
  "agent_instance_id": "...",       // optional; defaults to task's assignee
  "started_at":   "ISO 8601 ...",   // optional; defaults to now
  "completed_at": "ISO 8601 ...",   // optional; required for terminal states
  "command_count": 0                 // optional; if > 0, helper inserts that many tool_call messages
}
```

Returns the created session including its server-generated ID.

### `POST /api/v1/_test/messages`

Inserts a row directly into the messages table and publishes `session.message.added`.

```json
{
  "session_id": "...",
  "type": "message" | "tool_call" | "tool_output" | ...,
  "content": "...",
  "metadata": { ... }
}
```

### Mounting

Routes are mounted in `cmd/kandev/main.go` only when `KANDEV_E2E_MOCK=true`. Production builds never expose them. The Playwright global setup (`apps/web/e2e/global-setup.ts`) sets this env var on the spawned backend.

### Playwright fixture helpers

In `apps/web/e2e/helpers/api-client.ts` (or `e2e-mock-client.ts`):

```ts
seedTaskSession(taskId: string, opts: SeedSessionOpts): Promise<{ session_id: string }>
seedSessionMessage(sessionId: string, opts: SeedMessageOpts): Promise<void>
seedToolCallMessages(sessionId: string, count: number): Promise<void>
```

`seedToolCallMessages` is sugar over `seedSessionMessage` looped `count` times with `type: "tool_call"` and a synthetic command name - useful for driving the "ran N commands" derivation.

The `office-fixture.ts` exposes these helpers via the `apiClient` fixture so existing tests get them for free.

### Fixture liveness check

The Playwright fixture asserts the test routes are reachable on startup. An accidental misconfiguration fails loudly instead of silently turning all session-driven tests into flake.

## Failure modes

- **`KANDEV_E2E_MOCK` unset in production**: route group is not mounted; `POST /api/v1/_test/*` returns 404.
- **Invalid task ID**: seed route rejects with 400; no row inserted.
- **Non-canonical state value**: seed route rejects with 400.
- **Terminal state without `completed_at`**: seed route rejects with 400.
- **Fixture reachability check fails at test startup**: fixture errors loudly so the whole suite fails fast rather than producing flaky session-dependent tests.

## Persistence guarantees

State boundaries inside the e2e harness (`apps/web/e2e/fixtures/backend.ts`, `office-fixture.ts`):

**Per worker, persists for the worker's lifetime:**

- A unique tmpdir (`os.tmpdir()/kandev-e2e-<workerIndex>-<rand>`) containing the SQLite DB (`kandev.db`), HOME (`.kandev/`), worktree base, repo-clone base, an isolated `.gitconfig`, and a `git` shim. All seeded rows - tasks, sessions, messages inserted via `POST /api/v1/_test/...` - land in this DB.
- The worker-scoped `officeSeed` fixture: one workspace + CEO agent + project, created once via `completeOnboarding` and reused across every test in the worker.
- The backend process and its env baseline. `backend.restart(envOverrides)` kills and respawns the process against the same DB and tmpdir, so persisted seed data survives a restart; only in-memory execution state (running agent processes, live WS connections) is lost. Overrides do not leak into a subsequent `restart()` - each call rebuilds from the baseline snapshot.

**Per test, reset between tests:**

- The base `testPage` fixture invokes `apiClient.e2eReset(workspaceId, keepWorkflowIds)` against both the seed workspace and `officeSeed.workspaceId`, which deletes per-test rows (tasks, sessions, messages, etc.) while preserving the seeded workflow and onboarding artifacts.
- Optimistic / runtime caches such as user settings are re-saved through `saveUserSettings` on each test's setup.

**Does NOT persist beyond the worker:**

- The tmpdir is removed in the fixture's `finally` block (`fs.rmSync(tmpDir, { recursive: true, force: true })`). The DB, worktrees, clones, and seeded sessions / messages are discarded at worker teardown.
- The backend and frontend processes are killed in `finally`; nothing survives the Playwright run.
- No cross-shard or cross-worker caching: each worker mints its own port range and tmpdir, so seeded data from one worker is invisible to another.

**Interaction with this spec's seed routes:**

- Rows inserted by `POST /api/v1/_test/task-sessions` and `POST /api/v1/_test/messages` live in the worker's SQLite DB. They survive a `backend.restart()` exactly like rows inserted by production routes - the seed routes are persistence-equivalent, only the API gate differs.
- Seeded sessions never have a matching in-memory `lifecycle.Manager` execution, before OR after a restart, because no real executor was launched. Tests asserting "the live spinner is gone" after restart should rely on this invariant.

## Scenarios

- **GIVEN** `KANDEV_E2E_MOCK=true`, **WHEN** a test calls `POST /api/v1/_test/task-sessions` with a valid task ID and `state: "COMPLETED"`, **THEN** a row is created in `task_sessions` with the requested state and timestamps, and a `session.state_changed` WS event fires.

- **GIVEN** the env var is unset (production build), **WHEN** the same request is made, **THEN** the response is 404 - the route group is not mounted.

- **GIVEN** a Playwright test using the `office-fixture`, **WHEN** the test calls `apiClient.seedTaskSession(taskId, opts)`, **THEN** the helper hits the test route, returns the session ID, and the UI subscriber updates via WS.

- **GIVEN** a completed session seeded with `command_count: 3` and `started_at` 30s ago, **WHEN** the timeline renders, **THEN** the entry is collapsed with "... worked for 30s . ran 3 commands". Clicking the entry expands the embedded chat panel.

- **GIVEN** 3 completed sessions seeded with monotonically increasing `started_at`, **WHEN** the timeline renders, **THEN** the entries appear in that chronological order in the DOM.

- **GIVEN** existing E2E tests not using the seed routes, **WHEN** the suite runs with `KANDEV_E2E_MOCK=true`, **THEN** they continue to pass - the test routes do not pollute non-test code paths.

## Out of scope

- A general "fake everything" mode. Only sessions and messages get test routes. Tasks already have public APIs that work fine.
- Routes for arbitrary DB writes. Each route validates inputs (real task ID exists, state is in the canonical set) so tests cannot accidentally corrupt the DB.
- Production-time behavior. The env var is set only by the e2e harness; production deployments never set it.
- Replacing the existing mock-agent setup (`apps/backend/cmd/mock-agent/`). That drives real executor protocols; this bypasses executors entirely for UI tests.
