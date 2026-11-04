---
id: "01-restore-startup"
title: "Restore startup and failed-candidate convergence"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.2
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.5
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.5
system_design:
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 01: Restore startup and failed-candidate convergence

## Summary

Repair the early successor-generation timeout and settle failed native startup
candidates. Preserve existing conversation identity and durable admission proof.
Read the [plan](plan.md) for observed evidence and its limits.

## Scope and dependencies

No work-order dependency. Execute sequentially in the primary session.
Refresh the implementation base before editing. Preserve fresh-start successor
handling from PR #4523. Use the existing requirements without duplicating them.

Exclude production UI changes, new configuration, migrations, live instance
repair, and automatic prompt replay.

## Acceptance

1. An eligible native load that emits setup events before readiness can exceed
   the former event deadline and finish within the operation deadline. No event
   reaches receipt, projection, or acknowledgement before committed authority.
2. Failed or cancelled startup leaves a bounded, inspectable recovery state.
   Reconciliation can recover a proven-dead owned candidate after restart.
   A live unrelated or newer execution remains untouched.
3. Desktop and phone recover the same task session and native conversation.
   Reload retains the recovered state. A distinct new instruction runs once,
   the interrupted instruction is not replayed, and a healthy sibling remains usable.

## Implementation and coverage

`durable_silent_restore_startup_test.go` couples the real event processor with
fake time and a native-load response that requires its stream to remain open.
The response arrives after 40 seconds. Receipt and projection remain empty until
the successor commits. Additional cases cover owner cancellation, a bounded
wait, and cancellation during an in-flight SQL read after the commit.

`silent_restore_failed_startup_test.go` drives the real service and executor
startup error return. It covers failed and cancelled startup, termination
checkpoint persistence, inventory removal, and candidate rotation after restart.
A separate case preserves a ready candidate for commit retry.

`silent_restore_cleanup_test.go` exercises the lifecycle manager against a real
SQLite repository. It checks stop-before-checkpoint-before-removal ordering,
failed stop, failed checkpoint, replaced execution, unknown runtime, and cleanup
admission after joining a concurrent workspace operation. Existing negative
ownership and missing-evidence regressions remain in the selected suites.

The desktop and phone restart scenarios use the existing mock profile's native
load delay. They assert the same session, conversation, and worktree after a
40-second restore; no replay; one new instruction after reload and backend
restart; and a healthy sibling that keeps its execution and conversation.
No mock handler change or production timing seam was required.

## Changed paths

- Lifecycle: `durable_silent_restore_generation.go`, `manager_launch.go`,
  `manager_lifecycle.go`, `types.go`, and new `silent_restore_cleanup.go`.
- Backend wiring: `internal/backendapp/adapters.go`.
- Orchestrator: `resume_attempt.go`, `silent_restart_recovery.go`, and new
  `silent_restore_failed_startup.go`.
- Three new Go regression files described above.
- `apps/web/e2e/helpers/restart-reconciliation.ts` and the desktop and phone
  `agent-survival-restart` specs.
- Existing restart system design and the operations public guide.

## Documentation impact

Public docs updated: `docs/public/operations.md` (explanation). The guide states
the bounded startup behavior and the requirement for exact termination proof.
Internal docs updated: the existing restart design, this plan, and this work order.
There are no rendered UI changes, so PR screenshots are not required.

## Verification

Run from the repository root. Install web dependencies once if missing.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestSilentRestore|TestStreamEventProcessor' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator ./internal/orchestrator/executor -run 'TestSilentRestore|TestSilentRestart|TestNativeRestore|TestStreamEventProcessor|TestContinuationNative|TestInterruptedNativeResume|TestFreshStartResumeRequest' -count=1)
(cd apps/backend && go test -race -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/orchestrator/executor -run 'TestSilentRestore|TestSilentRestart|TestNativeRestore|TestStreamEventProcessor|TestContinuationNative|TestInterruptedNativeResume|TestFreshStartResumeRequest' -count=1)
(cd apps/backend && go test -race -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestFreshStart(Replacement|SQLWait)' -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/session/agent-survival-restart.spec.ts -- --grep 'slow native restore')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-agent-survival-restart.spec.ts -- --grep 'slow native restore')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths. No mock handler changed; the browser tests exercise its existing delay support.
Run browser suites sequentially. Confirm the selected tests were discovered.

## Risks

The stream processor context currently outlives parts of startup. Explicitly
bind its pending-generation wait to owned startup cancellation. Retain bounded
queues and the enclosing operation deadline. Do not weaken receipt authority,
extend waits indefinitely, or treat process liveness as readiness.

## Results

Each new production behavior was preceded by its failing regression.
Final-source checks:

- Focused Go race tests: lifecycle, orchestrator, and executor passed.
- Fresh-start replacement and SQL-wait race regressions passed.
- Changed-package Go lint passed with zero issues. The command was
  `bash scripts/lint-go-changed` with every changed and new Go path. This host
  used `GOMEMLIMIT=6GiB`, `GOMAXPROCS=2`, and golangci-lint 2.9.0.
- Prettier and ESLint passed for the three changed E2E TypeScript files.
- Desktop slow restore: one test passed on the final source.
- Phone slow restore: one test passed on the final source.
- Documentation catalog: 392 decisions and 1668 specifications validated.
  Specification lint and public-doc validation (54 pages) passed.

A small helper extraction resolved the cleanup function's complexity warning.
The final-source race and browser runs follow that production edit.

On this NixOS host, isolated browser runs need the installed browser library
paths in `LD_LIBRARY_PATH`. They set `KANDEV_SERVER_HOST=127.0.0.1` and omit the
`KANDEV_INTERNAL_CONFIG_FILE`, `KANDEV_INTERNAL_CONFIG_HOME_FILE`, and
`KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG` to avoid binding the disposable backend to the live instance address.
These are process-local test settings. No repository configuration or live
instance was changed.

## PR review remediation

The commit-retry regression now injects a real SQLite commit failure, closes
the candidate's stream, and checks that a later pass reconnects the same native
candidate and commits generation 2 without stopping or relaunching it. Lifecycle
coverage checks expired-owner rebinding, exact initialized identity, and replay
of a complete, unacknowledged setup prefix. Pruned, acknowledged, foreign,
uninitialized, and submission-bearing evidence stays blocked.

The E2E helper restores scoped backend settings in a nested finalizer, even when
task or profile deletion fails. Two helper regressions cover these failures.
It also deletes the healthy sibling explicitly. Cleanup assertions now run on
the test goroutine after the singleflight worker returns.

The PR documentation evaluator found a missing design reference for requirement
002. The plan and work order now reference `session-recovery-convergence.md`.
The same evaluator passes against the corrected local documents.

Validation commands for this remediation:

```bash
(cd apps/backend && go test -race -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/orchestrator/executor -run 'TestSilentRestore|TestSilentRestart|TestStreamEventProcessor|TestRecoverAgentPromptStream|TestDeliveryRetention|TestFreshStart(ResumeRequest|Replacement|SQLWait)' -count=1)
(cd apps/web && pnpm exec vitest run e2e/helpers/restart-reconciliation.test.ts)
```

Final remediation checks passed: the three-package Go race run, changed-package
Go lint (zero issues), both cleanup unit tests, TypeScript lint and formatting,
and one slow-restore browser test each on desktop and phone. The documentation
coverage evaluator, catalog validation, and specification lint also passed.
Remote checks and review resolution remain pending until the remediation commit
is pushed.

## Healthy-sibling setup diagnostics (2026-10-27)

PR #4534 head `0bbebdcf6afa` failed before the restore assertions: the healthy
sibling reached FAILED instead of WAITING_FOR_INPUT. The failed attempt has no
trace. Retained log errors are not bound to that sibling, so they do not prove
its startup cause. Five local repetitions completed successfully: three before
a host interruption and two after it. The interrupted fourth attempt is not a
completed result. No startup budget, action, assertion, or product logic changes.

Retain first-failure traces and attach the sibling session plus both backend
log sources before cleanup, including failures outside the inner restore block.
Attachment errors cannot replace the original test failure. The original CI
cause remains unresolved until a retained failure supplies that evidence.
