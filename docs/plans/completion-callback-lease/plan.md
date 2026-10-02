---
created: 2026-10-02
status: implemented
requirements:
  - REQ-TASKS-SESSION-TURN-SETTLEMENT-001
system_design:
  - ../../specs/tasks/system-design/session-turn-settlement.md
legacy_specs: []
---

# Fix plan: Completion callback lease

## Overview

Repair [issue #4149](https://github.com/kdlbs/kandev/issues/4149) by reusing the
startup callback lease during synthetic turn completion. One sequential work
order covers the regression, minimal correction, and compatibility checks.

The issue is assigned to `carlosflorencio`, verified through the GitHub API on
2026-10-02. The implementation and required verification are complete. The
source and delivery documents remain uncommitted.

## Evidence and root cause

Inspected revision: `d22fbaab1fcdf35add3272c0a6fcafebb4ed8e2b`.
The issue contains a goroutine dump and timeline, with no image attachments.

The current source contains this call chain:

```text
handleAgentEventWithStartupGeneration
  withStartupAttempt: startupCallbackMu.RLock
    handleCompleteEventLeased
      finishPromptCompletion
        handleCompleteEventMarkState
          MarkReady
            markReadyEventWithStartupGeneration
              withStartupAttempt: second startupCallbackMu.RLock
```

`BindResumeAttempt` can request the exclusive lock between the read locks.
The second reader then waits for the writer, while the writer waits for the
outer reader. Neither can release its dependency.

A temporary `TestReproIssue4149` reproduced this against the real lifecycle
manager. It created an initialized Running execution with no pending dispatch,
held `withStartupAttempt`, and started `BindResumeAttempt` in another goroutine.
It used `TryRLock` with a deadline to prove the writer was pending, then called
`handleCompleteEventLeased` with generation zero and the captured attempt ID.

Command, from `apps/backend`:

```bash
go test ./internal/agent/runtime/lifecycle -run '^TestReproIssue4149$' -count=1 -timeout=8s -v
```

Result: expected failure after 8.022 seconds. The timeout stack showed the
second read lock at `types.go:826`, `MarkReady` at `manager_events.go:225`,
and the writer at `manager_resume_attempt.go:27`. The temporary test was removed.
No production file changed, and no live instance was accessed.

The issue's global idle-reaper impact follows from the session lifecycle lock
held across resume and acquired by `reclaimIdleSession`. That impact was traced
in source, not reproduced against a running installation.

## Requirement reconciliation

Tasks owns this package because it owns session execution lifecycle and
follow-up admission. Adjacent agent runtime recovery, task cancellation,
runtime publication, and task completion specifications cover other contracts.
No precise active acceptance criterion covers synthetic completion progress
under concurrent resume.

The [requirements](../../specs/tasks/requirements/session-turn-settlement.md)
make the existing ADR 0035 synthetic-turn behavior explicit. The paired
[design](../../specs/tasks/system-design/session-turn-settlement.md) preserves
that ADR's ownership and synchronous-publication decisions. No new ADR is
needed for reuse of the existing lease-free internal helper.

Confirmed scope: repair issue #4149 by preserving the outer callback lease and
reusing its captured attempt ID for readiness publication. No material product
choice remained unresolved during implementation.

## Scope

### In scope

- Replace the nested readiness lease with the existing internal readiness helper.
- Establish deterministic regression coverage and preserve attempt identity.
- Audit direct event-dispatch descendants for the same nested startup lease.

### Out of scope

- Issue #4150 and dispatch-only completion eligibility.
- Asynchronous workspace refresh, idle-reaper redesign, or new stall telemetry.
- Broad lock refactoring, new event scheduling, or already-deadlocked process recovery.
- Rendered UI changes, localization, and public API changes.

## Technical approach

In `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`, change
`handleCompleteEventMarkState` to call `markReadyEventForExecution` with
`context.Background()`, the current execution, `events.AgentReady`, `false`,
and `event.AttemptID`. Preserve the completion signal before Ready publication.
Document the held-lease precondition and align direct test callers with it.

Keep `manager_interaction.go` and `manager_resume_attempt.go` behavior intact.
Do not drop the outer lease, substitute a fresh attempt lookup, or defer
ordinary Ready publication to a goroutine.

### Related path audit

| Path inspected | Result |
| --- | --- |
| Dispatcher completion, direct completion, context-reset replay | All enter the leased completion path; the generation-zero success branch calls `MarkReady` recursively |
| Numbered successful completion | Uses the captured claim payload, without the readiness wrapper |
| Error event and error completion | Uses `handleCompleteEventLeased` and `markCompletedWithTurnIDAndAttempt`; no second startup lease in that terminal helper |
| Foreground-idle and stream disconnect | Use `signalPromptCompletionForStartupGenerationLeased` |
| Activity and event publication | Inspected direct helpers do not acquire `startupCallbackMu`; arbitrary subscriber behavior is outside this bounded audit |

### Compatibility matrix

| Provider / transport | Identity shape | Expected behavior | Evidence / fallback |
| --- | --- | --- | --- |
| pi-acp over agentctl stream | Current startup, generation zero, no pending dispatch | Synthetic completion settles under resume contention | Reported trigger plus provider-neutral regression |
| Other adapters through the same dispatcher | Same eligible generation-zero shape | Same correction, without provider checks | Shared lifecycle tests; no claim of live per-provider testing |
| Any adapter with numbered completion | Current prompt generation | Existing claim and publication path | Existing stale/current prompt tests |
| Stale startup or pending dispatched prompt | Obsolete startup or unnumbered terminal frame | Existing rejection | Negative regression cases; no new fallback |

## Tests

New tests belong in
`apps/backend/internal/agent/runtime/lifecycle/manager_events_completion_lease_test.go`.

| Acceptance | Planned test |
| --- | --- |
| 001.1, 001.2 | `TestCompletionStartupLease_PendingResumeWriter`: real dispatcher, bounded child process, original attempt in Ready payload, resume writer returns |
| 001.1, 001.3 | `TestCompletionStartupLease_SyntheticStates`: Running and Ready inputs; signal before Ready; one Ready event; subsequent `BeginPrompt` succeeds |
| 001.4 | `TestCompletionStartupLease_StaleAndPending`: stale startup, stale numbered completion, unnumbered completion with pending dispatch |
| 001.2, 001.4 | Existing startup lease, adopted attempt, error, and foreground-idle tests |

## E2E tests

The regression exercises the runtime event-to-readiness-to-next-prompt boundary
in Go. No browser interaction participates in the mutex cycle. Add no artificial
Playwright test or new browser project for this backend-only correction.
The work order also runs existing orchestrator Ready-handler tests to check
the downstream queue and workflow boundary.

## Work orders

- [x] [Task 01: Preserve the completion callback lease](task-01-preserve-completion-lease.md)

## Verification results

- Temporary contention reproduction: expected deadlock timeout, as recorded above.
- Existing baseline: four focused lifecycle tests passed in 0.132 seconds.
  These cover callback lease, disconnect lease, adopted attempt identity, and
  pending-dispatch rejection. The exact baseline command is in the work order.
- RED: the new bounded child-process regression failed after proving that
  `BindResumeAttempt` was pending; the parent timed out and reaped the child
  after 8.02 seconds while completion remained blocked.
- GREEN: `go test ./internal/agent/runtime/lifecycle -run
  '^TestCompletionStartupLease_' -count=1 -timeout=60s -v` passed.
- Race repetition: `go test -race ./internal/agent/runtime/lifecycle -run
  '^TestCompletionStartupLease_' -count=20 -timeout=180s` passed in 22.539
  seconds.
- Full lifecycle race suite: `go test -race
  ./internal/agent/runtime/lifecycle -count=1 -timeout=300s` passed in 97.664
  seconds.
- Downstream Ready handlers: `go test -race ./internal/orchestrator -run
  '^TestHandleAgentReady' -count=1 -timeout=180s` passed.
- `make build` from `apps/backend`: passed for backend and helper binaries.
- `python3 scripts/list-docs.py validate`: passed (340 decisions, 1297 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local `validateCoverage` preflight: `covered`, with no reference errors.
- `git diff --check` and trailing-whitespace checks passed. Build artifacts are
  ignored by Git.

## Documentation impact

Internal specifications and delivery records only. The `/docs-maintainer`
assessment found no new operator action, UI control, configuration, or public
contract requiring a public documentation change in this design package.

## Risks

- A sleep-only test can pass without reaching writer contention.
- A deadlock test in the main test process can leak blocked goroutines.
- Dropping the outer lease can let an old callback mutate a successor.
- Fresh identity reads or asynchronous publication can misattribute completion.
- Synchronous workspace refresh still affects completion latency after this repair.
