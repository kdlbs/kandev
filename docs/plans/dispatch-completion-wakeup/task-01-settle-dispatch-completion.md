---
id: "01-settle-dispatch-completion"
title: "Settle dispatch completion ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001
acceptance_criteria:
  - AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.1
  - AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.2
  - AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.3
  - AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.4
  - AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.5
  - AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.6
system_design:
  - ../../specs/platform/system-design/prompt-completion-ownership.md
---

# Task 01: Settle dispatch completion ownership

## Summary

Release a dispatch-only barrier when its numbered completion finishes processing.
Prevent acknowledgement from restoring the barrier after completion.
Prove that later autonomous completion permits another prompt on the same execution.

## In scope

- Implement the completion-finalization and acknowledgement rules in the referenced design.
- Add all six named regression groups in the plan before production changes.
- Update the early-completion assumption in `newDispatchCancelFixture` and directly affected tests.
- Cover successful/error completion, both acknowledgement orders, blocked finalization, and existing waiters.
- Cover stale/duplicate events, replacement execution, and an older completed generation beside a live successor.
- Cover error-before-acknowledgement with a synchronous failure subscriber, and error finalization paused before terminal status application while a successor is waiting.
- Run the exact compatibility checks below and record results.

## Out of scope

- Provider notification filtering and issue #4149's lock repair.
- New lifecycle policy, timeout, persisted field, API, or frontend behavior.
- Generic refactoring or broad verification gates.

## Acceptance

1. Both acknowledgement orders leave the completed generation non-pending; its later autonomous completion reaches Ready.
2. Stale, duplicate, and unnumbered events cannot release a live successor; predecessor finalization and completion consumption remain ordered.
3. The protocol integration test observes exactly one successor request on the same execution without restart or readiness timeout. Compatibility checks pass.

## TDD sequence

1. Add `TestDispatchCompletion_WakeupAfterNumberedCompletion` with the two orders from the temporary reproduction.
2. Run it before the correction and record both expected failures.
3. Add the remaining plan regressions with deterministic barriers and the existing WebSocket mock.
4. Implement generation-checked acknowledgement, terminal-state fencing, and post-unlock error/stop publication.
5. Update only obsolete fixture expectations, then run all verification commands.

Do not take `promptMu` in a completion handler. A waiting caller can already own that mutex.
Do not clear pending at claim entry. Finish transcript processing before release.
For errors, apply FAILED/STOPPED before signaling or clearing pending, then publish the captured terminal event after releasing `promptLifecycleMu`.
Do not create another consumer of `promptDoneCh`.

## Verification

Run each command from the repository root:

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run '^TestDispatchCompletion_' -count=1 -v)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run '^(TestDispatchCompletion_|TestHandleAgentEvent_(UnnumberedCompleteCannotReleasePendingPrompt|DelayedCompleteCannotFinishReplacementPrompt)|TestWaitForPendingDispatchedPrompt_|TestManager_CancelAgent_|TestManager_ResetAgentContext_|TestManager_RestartAgentProcess_Success|TestSendPrompt|TestWakeup_)' -count=1)
make -C apps/backend build
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run this documentation-coverage preflight from the repository root:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/plans/dispatch-completion-wakeup/plan.md',
  'docs/plans/dispatch-completion-wakeup/task-01-settle-dispatch-completion.md',
  'docs/specs/platform/README.md',
  'docs/specs/platform/requirements/prompt-completion-ownership.md',
  'docs/specs/platform/system-design/prompt-completion-ownership.md',
];
const result = validateCoverage({
  changedFiles: [...docs, 'apps/backend/internal/agent/runtime/lifecycle/manager_events.go', 'apps/backend/internal/agent/runtime/lifecycle/session.go', 'apps/backend/internal/agent/runtime/lifecycle/manager_events_dispatch_completion_test.go'],
  fileContents: Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(result);
if (!result.ok) process.exitCode = 1;
JS
```

## Files changed

All lifecycle paths below are under `apps/backend/internal/agent/runtime/lifecycle/`:

- `manager_events.go`: numbered completion finalization.
- `manager_interaction.go`: terminal error state capture and post-unlock failure/stop publication.
- `session.go`: dispatch acknowledgement bookkeeping.
- `manager_events_dispatch_completion_test.go`: new focused regression file.
- `manager_interaction_dispatch_cancel_test.go`: early-completion fixture expectations and existing cancellation assertions.
- `docs/specs/platform/README.md`: link the accepted requirement and current design.
- This package's documents: final results and statuses.

## Dependencies

None for implementation. Coordinate the release with the separate #4149 repair for contended startup-lock scenarios.

## Risks

Follow the plan's signal-ordering and pending-writer caveats.
Keep global timeout changes out of parallel tests.
No live provider replay is required for the deterministic lifecycle correction.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/prompt-completion-ownership.md), all six criteria.
- [System design](../../specs/platform/system-design/prompt-completion-ownership.md), all sections.
- [Plan](plan.md), evidence and regression matrix.
- [ADR 0035](../../decisions/0035-version-agent-ready-events-by-prompt-generation.md).
- Existing patterns: `manager_events_test.go`, `wakeup_simulated_test.go`, and `manager_interaction_dispatch_cancel_test.go`.

## Results

The first test run failed both acknowledgement-order cases at the expected pending-barrier assertion. After implementation, the numbered completion barrier releases after transcript finalization and completion-signal delivery. Dispatch acknowledgement now checks the active execution identity and generation while holding `promptLifecycleMu`; its callback still runs after the lock is released. The standalone `SessionManager` path remains covered, and the cancellation fixture now expects a completion-before-acknowledgement to remain non-pending.

The original six named regression groups pass. They cover both acknowledgement orders, same-execution successor delivery through the WebSocket mock, a waiter blocked behind transcript flushing, matching and stale errors, stale and duplicate events, and successor/replacement ownership.

The targeted Go test, the work-order race test, `make -C apps/backend build`, `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` all passed. The documentation-coverage preflight returned `covered` with no errors. The backend build emitted expected macOS helper signing warnings because no signing tool is installed. Issue #4149's independent startup-lock race remains outside this fix and requires coordinated release.

The two review regressions first failed against the reviewed implementation: error-before-ack blocked the acknowledgement callback behind the synchronous failure subscriber, and a successor reopened an execution while failure finalization was paused. After the correction, both passed. The full dispatch-completion group and the specified race-enabled lifecycle compatibility suite passed again, as did the backend build and documentation gates. The failure path now records FAILED or shutdown STOPPED before it signals completion or clears pending; it publishes the captured terminal identity and evidence after releasing `promptLifecycleMu`. Coordinate release with #4149 because its separate startup-lock race remains out of scope.

The full lifecycle package also passed with `go test ./internal/agent/runtime/lifecycle -count=1` (80.486s).
