---
id: "01-retire-stale-transient-retry-notices"
title: "Retire stale transient retry notices"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/provider-error-recovery.md"
requirements:
  - "AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.9"
  - "AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.10"
  - "AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.11"
---

# Task 01: Retire stale transient retry notices

## Acceptance

- Every true interactive retry ending path attempts to retire all persisted
  session messages whose metadata has boolean `retrying: true`; successful
  cleanup removes all matches, while failures remain eligible for later
  authorized cleanup.
- The task service owns message listing and deletion, and each deletion is
  published as `session.message.deleted` for connected viewers.
- Successful recovery and late authorized Cancel leave no yellow retry notice
  in the durable transcript when cleanup succeeds; the task-service-backed test
  proves the state after the session settles and reloads the transcript.
- Late authorized Cancel returns false, resolves stale notices, and creates no
  red recovery message.
- Active authorized Cancel returns true, resolves stale notices, and retains
  the red Resume and Start fresh recovery message.
- Authorization stays first. A denied or foreign task-session pair performs no
  cleanup and exposes no state.
- Attempt advancement does not clear the notice while retry ownership remains.
- Transcript cleanup failures are logged and swallowed, and durable I/O does
  not run while `taskRuntimeStateMu` is held.
- Ordinary successful turns without an owned retry entry skip the full
  transcript scan; terminal, stop, and Cancel paths force stale-notice cleanup.
- Desktop, mobile, and transport-loss Playwright flows prove that yellow and
  red retry cards do not become stale or coexist after cancellation.

## Files likely touched

- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_transient.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/orchestrator/event_handlers_transient_resolution_test.go`
- `apps/web/e2e/tests/session/transient-retry.spec.ts`
- `apps/web/e2e/tests/session/mobile-transient-retry.spec.ts`
- `apps/web/e2e/tests/session/transient-retry-transport-lost.spec.ts`

## Inputs

- `REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001`, especially acceptance criteria
  `.9` through `.11`.
- The interactive transient retry notice lifecycle in the paired system
  design.
- Existing `MessageCreator`, task-service `ListMessages` and `DeleteMessage`,
  event bus publication, frontend message deletion handler, and retry E2E
  fixtures.

## Implementation sequence

1. Add backend and Playwright assertions first. Run the focused tests and
   confirm they fail because persisted `retrying: true` messages survive
   success and late cancellation. Add `@covers` comments for the acceptance
   criteria to the new tests.
2. Define a narrow orchestrator message-resolution interface with
   `ListMessages` and `DeleteMessage`, add its service wiring, and inject the
   existing task service from `backendapp`.
3. Implement best-effort deletion of every boolean `retrying: true` message.
   Assert unrelated messages remain and deletion events are published.
4. Integrate durable resolution into success, exhaustion, terminal, stop,
   explicit cancellation, and shutdown paths without placing database work
   under `taskRuntimeStateMu`.
5. Make late cancellation idempotent after the authorization guard. Preserve
   the active-loop call to `handleRecoverableFailure`.
6. Run focused tests green, refactor within backend complexity and file-size
   limits, then run the complete verification gate.

## Output contract

Report the exact ending paths changed, the task-service/event-bus evidence,
late and active Cancel return behavior, the attempt-advancement regression,
desktop and mobile E2E results, files changed, exact commands and results,
remaining risks, and synchronized work-order/plan status.

## Verification

From `apps/backend`:

```bash
rtk go test ./internal/orchestrator -run 'Test.*TransientRetry.*(Resolve|Cancel|Advance|Success)' -count=1
```

From the repository root:

```bash
rtk make -C apps/backend test
rtk make -C apps/backend lint
```

From `apps/backend`, using the PR base SHA:

```bash
rtk golangci-lint run ./... --new-from-rev="$(rtk git merge-base HEAD origin/main)" --timeout=5m
```

From `apps/web`:

```bash
rtk pnpm e2e:run --project chromium tests/session/transient-retry.spec.ts tests/session/transient-retry-transport-lost.spec.ts
rtk pnpm e2e:run --project mobile-chrome tests/session/mobile-transient-retry.spec.ts
```

If `apps/node_modules` is absent, run `rtk pnpm install --frozen-lockfile` from
`apps` before the Playwright commands.

## Risks

- The coordinator stop path currently resets retry state while holding
  `taskRuntimeStateMu`; durable cleanup must occur after that critical section.
- Multiple attempts can leave multiple messages, so resolving only the newest
  row does not satisfy the contract.
- Test fixtures must subscribe to the event bus before deletion and must not
  infer durable behavior only from a mock method call.

## Results

Implemented. The orchestrator now clears in-memory retry ownership and deletes
all persisted `retrying: true` status messages through the task service. The
task service publishes `session.message.deleted`, so connected viewers and
later transcript loads agree. Coordinator stop performs durable cleanup after
releasing `taskRuntimeStateMu`; attempt advancement only cancels its superseded
timer. Authorized late Cancel is idempotent and active Cancel still publishes
the existing manual recovery actions. Denied task-session pairs return before
any retry-state read or transcript mutation.

Verification passed:

- Focused retry-resolution tests: 8 passed, including race mode.
- Full orchestrator package: 2,168 tests passed.
- Full backend test suite passed with managed launcher configuration variables
  unset; the unmodified managed run was blocked by its injected config file.
- Backend lint and changed-code `golangci-lint` passed.
- Desktop Chromium transient-retry and transport-loss specs: 7 passed.
- Mobile Chromium transient-retry spec: 1 passed.
- Web typecheck/lint, specification lint, gofmt, and diff checks passed.

The current mock-agent fixture does not provide a stable fail-then-recover
browser response after relaunch, so successful recovery durability is covered
by the task-service-backed transcript test. Existing desktop, mobile, and
transport-loss flows cover the user-visible cancellation behavior.
