---
id: "01-preserve-completion-lease"
title: "Preserve the completion callback lease"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SESSION-TURN-SETTLEMENT-001
acceptance_criteria:
  - AC-TASKS-SESSION-TURN-SETTLEMENT-001.1
  - AC-TASKS-SESSION-TURN-SETTLEMENT-001.2
  - AC-TASKS-SESSION-TURN-SETTLEMENT-001.3
  - AC-TASKS-SESSION-TURN-SETTLEMENT-001.4
system_design:
  - ../../specs/tasks/system-design/session-turn-settlement.md
---

# Task 01: Preserve the completion callback lease

## Summary

Remove the recursive startup callback read lock from synthetic completion.
Reuse the existing internal readiness helper and prove progress under a pending
resume writer, without changing generation or publication semantics.

## In scope

- Add deterministic red-green regression tests at the real dispatcher boundary.
- Apply the minimal `handleCompleteEventMarkState` correction described in the plan.
- Document its lease precondition and adjust direct test callers as necessary.
- Preserve synchronous event order, captured attempt identity, and negative admission cases.
- Record the bounded related-path audit and exact verification results.

## Out of scope

Issue #4150, provider-specific changes, reaper scheduling, workspace refresh,
new timeouts, database changes, and frontend changes.

## Acceptance

1. A proven pending `BindResumeAttempt` writer cannot deadlock an eligible
   generation-zero completion. Completion and writer both return, and the
   next `BeginPrompt` succeeds. Cover Running and Ready inputs.
2. The Ready payload carries the original attempt ID. The completion channel
   is signaled before Ready publication; the empty-turn fallback retains
   Running-before-Ready order and publishes Ready once.
3. Stale startup and prompt callbacks remain rejected. A pending dispatched
   prompt still rejects an unnumbered completion. Error and foreground-idle
   paths retain existing behavior.

## TDD procedure

1. Add `TestCompletionStartupLease_PendingResumeWriter` in the new test file.
   Enter through `handleAgentEventWithStartupGeneration` on an initialized
   execution. Use a test logger hook at `complete event processed` as a barrier
   before the readiness transition. No production test hook is needed.
2. While that callback holds its outer lease, start `BindResumeAttempt`.
   Establish that the writer is pending with a bounded `TryRLock` probe loop.
   Immediately release each successful probe. Do not infer contention from sleep.
3. Release the callback barrier. Before the repair, a bounded child test process
   must fail at the nested read lock. Use the test binary as a subprocess;
   terminate and reap it on timeout. Assert its contention marker so unrelated
   startup failures cannot count as a valid reproduction.
4. Apply the minimal correction. Both operations must now finish within the
   child deadline. Join both goroutines before reading mutable fields.
5. Add `TestCompletionStartupLease_SyntheticStates` and
   `TestCompletionStartupLease_StaleAndPending` with the plan's state matrix.
   Exercise the direct `handleCompleteEvent` entry point as well as the dispatcher.
6. Inspect the related paths listed in the plan. If another concrete recursive
   lease appears, add a regression and update this package before expanding scope.

Use the tracking event bus to assert payload identity and order. A synchronous
subscriber can check that the completion signal already exists when Ready
arrives. Keep test goroutine lifetimes bounded; the package uses goleak.

## Verification

Run from the repository root after implementation:

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run '^TestCompletionStartupLease_' -count=1 -timeout=60s -v)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run '^TestCompletionStartupLease_' -count=20 -timeout=180s)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -count=1 -timeout=300s)
(cd apps/backend && go test -race ./internal/orchestrator -run '^TestHandleAgentReady' -count=1 -timeout=180s)
(cd apps/backend && make build)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/completion-callback-lease
```

Run this local documentation-coverage preflight from the repository root.
It includes the planned production path so the validator checks all references
even before implementation exists:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/plans/completion-callback-lease/plan.md',
  'docs/plans/completion-callback-lease/task-01-preserve-completion-lease.md',
  'docs/specs/tasks/requirements/session-turn-settlement.md',
  'docs/specs/tasks/system-design/session-turn-settlement.md',
];
const codePaths = [
  'apps/backend/internal/agent/runtime/lifecycle/manager_events.go',
  'apps/backend/internal/agent/runtime/lifecycle/manager_events_completion_lease_test.go',
];
const result = validateCoverage({
  changedFiles: [...paths, ...codePaths],
  fileContents: Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exit(1);
NODE
```

The investigation ran this baseline successfully before any correction:

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test(StartupCallbackLeasePreventsReplacementDuringMutation|StartupDisconnectLeasePreventsReplacementDuringMutation|BindResumeAttemptCarriesIdentityAcrossAdoptedStartupCallbacks|HandleAgentEvent_UnnumberedCompleteCannotReleasePendingPrompt)$' -count=1 -timeout=60s)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events_completion_lease_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/manager_events_test.go` (existing direct state-helper callers only)
- `apps/backend/internal/agent/runtime/lifecycle/manager_events_startup_test.go` (existing direct state-helper callers only)
- `apps/backend/internal/agent/runtime/lifecycle/manager_events_shutdown_test.go` (existing direct state-helper callers only)
- This work order and its sibling plan, for results and status.

## Dependencies

None.

## Risks

Preserve the startup lease through publication. Do not replace immutable
`event.AttemptID` with a fresh mutable snapshot. Do not broaden this repair
into the separate dispatch-only completion eligibility defect.

## Parallelism

`sequential`

## Inputs

- [Session turn settlement requirements](../../specs/tasks/requirements/session-turn-settlement.md).
- [Session turn settlement design](../../specs/tasks/system-design/session-turn-settlement.md).
- [ADR 0035](../../decisions/0035-version-agent-ready-events-by-prompt-generation.md).
- `manager_events_startup_test.go`: callback barriers and attempt identity tests.
- `types.go`: `withStartupAttempt` and the leased completion-signal helper.
- `manager_interaction.go`: `markReadyEventForExecution`.
- `manager_resume_attempt.go`: exclusive writer and store-lock order.

## Results

Implemented the successful generation-zero completion path with
`markReadyEventForExecution`, retaining the outer startup callback lease,
synchronous Ready publication, and the originating `event.AttemptID`. The
internal state helper now documents its lease requirement. Direct state-helper
tests enter through a test helper that acquires the lease.

The new tests cover a pending `BindResumeAttempt` writer at the real startup
dispatcher, Running-with-content and Ready-empty completions, signal-before-Ready
ordering, attempt identity, one Ready publication, Running-before-Ready for an
empty turn, subsequent prompt admission, stale startup and prompt completion,
and unnumbered completion rejection during pending dispatch. The RED run proved
writer contention before callback release, then the parent timed out and reaped
the child after 8.02 seconds while the recursive read remained blocked.

Verification results:

- Focused regression: `go test ./internal/agent/runtime/lifecycle -run
  '^TestCompletionStartupLease_' -count=1 -timeout=60s -v` passed.
- Repeated race regression: `go test -race
  ./internal/agent/runtime/lifecycle -run '^TestCompletionStartupLease_' -count=20
  -timeout=180s` passed in 22.539 seconds.
- Full lifecycle race suite: `go test -race
  ./internal/agent/runtime/lifecycle -count=1 -timeout=300s` passed in 97.664
  seconds.
- Downstream Ready handlers: `go test -race ./internal/orchestrator -run
  '^TestHandleAgentReady' -count=1 -timeout=180s` passed.
- `make build` from `apps/backend` passed for the backend and helper binaries.
- Documentation catalog validated 340 decisions and 1297 specifications;
  specification lint passed.
- Documentation coverage preflight returned `covered` with no errors.
- `git diff --check` and the explicit trailing-whitespace scan passed.

The related-path audit from the plan found no additional recursive startup
lease in error completion, foreground-idle, disconnect, activity, or event
publication helpers. Issue #4150, workspace-refresh latency, and reaper policy
remain out of scope.
