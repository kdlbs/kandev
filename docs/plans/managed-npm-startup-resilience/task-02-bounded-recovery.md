---
id: "02-bounded-recovery"
title: "Recover setup without disrupting siblings"
status: completed
wave: 2
depends_on:
  - "01-startup-evidence"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.8
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 02: Recover setup without disrupting siblings

## Summary

Retry eligible managed npm setup once after confirmed cleanup and cancellable backoff.
Remove shared-tree deletion from automatic task and capability-probe recovery.

## In scope

- Typed phase/evidence eligibility, one replacement budget, randomized delay, and bounded deadline.
- Non-destructive ETARGET/transient retry and unchanged-command early-exit retry.
- Startup-event fencing, cancellation, same session identity, final typed cause, and single prompt delivery.
- Host-probe ETARGET recovery without deletion; preserve existing probe eligibility and admission ordering.

## Out of scope

- Generic post-initialize recovery, broader host-probe retries, explicit Settings repair changes, or UI rendering.

## Acceptance

1. Transient npm or confirmed early-exit faults retry once. Policy, permanent conditions, missing evidence, signal exits, and later-phase faults do not retry.
2. Failed cleanup, cancellation, or expired deadline prevents replacement. Every retry preserves IDs/settings; success sends the accepted prompt once.
3. Concurrent mixed outcomes preserve healthy sibling files/processes. Task and probe retries never call cache repair. Exhaustion reports the actual final cause once.

## TDD and implementation

Add `managed_runtime_startup_resilience_test.go` with `TestManagedStartupRecovery` subtests for every eligibility and exclusion branch.
Add deterministic `TestManagedStartupRecoveryCancellation`, `TestManagedStartupRecoveryMixedBurst`, and `TestManagedStartupRecoverySinglePrompt` suites.
Use barriers and injected delay selection, not timing sleeps. Assert two starts maximum, zero cache-repair calls, and no transient FAILED event.
Reproduce the issue-shaped empty-stderr exit with real process evidence from Task 01.
Extend hostutility tests with `TestManagedRuntimeProbeRecoveryPreservesSharedTree` and a concurrent task sentinel.

Reuse the one-replacement startup generation. Do not reset it during configure, reconnect, or failure classification.
Extract policy and evidence helpers from managed_runtime_startup.go rather than growing a single function.
Change the stop-error branch to prevent replacement. Keep teardown bounded and preserve cancellation semantics.
Add `managed_runtime_startup` to routingerr and orchestrator final failure recognition, without enabling generic provider retry/fallback policy.
Failure metadata includes reason, actual attempts, and sanitized final evidence.

Update old ETARGET tests that explicitly expect cache-repair calls. Their new assertions require online retry with preserved cache files.
Update mock-server fixtures to support explicit empty stderr and generation evidence without legacy defaults masking faults.

## Verification

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'TestManagedStartupRecovery|TestManagedRuntime|TestRetryManagedRuntime|TestStartup|TestHandle.*Startup' -count=1)
(cd apps/backend && go test -race ./internal/agent/hostutility -run 'TestManagedRuntime' -count=1)
(cd apps/backend && go test ./internal/agent/runtime/routingerr ./internal/orchestrator -run 'TestManagedStartup|TestManagedRuntime|TestClassifyManagedRuntime|TestCreateRecoveryStatusMessage' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record the actual test names and results. Include any newly created test names in these filters before marking done.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/managed_runtime_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/managed_runtime_startup_policy.go` and `managed_runtime_startup_resilience_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`, `manager_startup.go`, `manager_events.go`, and `types.go` only where integration requires changes
- `apps/backend/internal/agent/runtime/lifecycle/managed_runtime_startup_recovery_test.go` and `manager_interaction_test.go`
- `apps/backend/internal/agent/hostutility/manager.go` and `managed_runtime_recovery_test.go`
- `apps/backend/internal/agent/runtime/routingerr/routingerr.go` and focused policy/classification tests
- `apps/backend/internal/orchestrator/event_handlers_agent.go` and focused recovery tests

## Dependencies

Task 01.

## Risks

A retry after session creation can duplicate work. A failed Stop cannot be treated as successful cleanup.
Do not reintroduce automatic cache deletion through the host-probe path.

## Parallelism

`sequential`

## Inputs

- [Design: Retry lifecycle](../../specs/agents/system-design/managed-npm-runtime-recovery.md#retry-lifecycle).
- [Decision](../../decisions/2026-10-02-bounded-managed-npm-startup-retry.md).
- Existing managed runtime startup generation and recovery tests; hostutility retry admission tests.

## Results

Implemented a single managed-runtime replacement for strict package-resolution failures, recognized transient npm failures, and confirmed unexpected ordinary pre-initialize exits. Recovery requires typed ACP-initialize phase and matching process-generation evidence when available, confirms process stop before replacement, uses a cancellable 2–3 second delay under a 90-second recovery context, and never deletes the shared npm execution tree. Permanent errors, signals, stale/incomplete evidence, cancellation, unsupported runtimes, and post-initialize failures do not enter retry.

The host capability probe retains its existing strict ETARGET eligibility and retry ordering, with automatic cache deletion removed. Routing now recognizes `managed_runtime_startup` without enabling generic retry or provider fallback. Added tests cover empty stderr, typed and legacy ETARGET, transient npm errors, silent early exit, permanent errors, generation mismatch, signal exit, incomplete evidence, cleanup cancellation/failure, caller deadline, same-session single prompt, mixed concurrent outcomes, and shared-tree sentinel preservation.

Passed verification:

- `(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'TestManagedStartupRecovery|TestManagedRuntime|TestRetryManagedRuntime|TestStartup|TestHandle.*Startup' -count=1)`
- `(cd apps/backend && go test -race ./internal/agent/hostutility -run 'TestManagedRuntime' -count=1)`
- `(cd apps/backend && go test ./internal/agent/runtime/routingerr ./internal/orchestrator -run 'TestManagedStartup|TestManagedRuntime|TestClassifyManagedRuntime|TestCreateRecoveryStatusMessage' -count=1)`
- `(cd apps/backend && go test -race ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/common/npmresolution -run 'TestManagedStartup|TestManagedNpm|TestSafeManagedNpm|TestHandleWSInitializeRejectsMismatched' -count=1)`
- `(cd apps/backend && go test -race ./internal/agent/runtime/agentctl -run 'TestInitializeStartupEvidence' -count=1)`

Task 03 owns the end-to-end UI/executor proof, localization, and public documentation; its implementation results are recorded separately.

### Code-review follow-up (2026-10-02)

Addressed all three review findings:

- npm diagnostics reuse the already-pinned agentctl client. The regression queues a lifecycle writer behind that lease and proves the diagnostic finishes without a nested read lock.
- Retry exhaustion retains a bounded sanitized final diagnostic and the actual failure phase. Initialize authentication failures keep auth recovery, while session setup and process setup failures retain their own reasons and runtime error classification.
- Startup evidence records unknown npm codes separately from no npm evidence. Unknown codes, oversized diagnostics, and incomplete stderr collection fail closed; a complete ordinary exit with genuinely empty stderr remains eligible.

Passed validation:

- Full tests for `internal/agent/hostutility`, `internal/agent/runtime/lifecycle`, `internal/agentctl/server/process`, `internal/agentctl/server/api`, `internal/agent/runtime/agentctl`, `internal/agentctl/types`, `internal/common/npmresolution`, `internal/agent/runtime/routingerr`, and `internal/orchestrator`.
- Race tests for the queued-writer, final-cause/auth, unknown-or-incomplete-diagnostics, and startup-evidence regressions across lifecycle, process, agentctl, npm resolution, routing, and orchestrator packages.
- Scoped `golangci-lint` for all affected backend packages: zero issues.
- `make build` from `apps/backend`, including bundled agentctl targets for Linux and macOS.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.

The original incident's process-exit cause remains unconfirmed.

The subsequent PR review also verified generation-pinned npm diagnostics and retry replacement start responses. Regression coverage now checks that cleanup writers cannot deadlock diagnostics, the final retry cause is retained and classified for authentication, process-setup/session-setup failures keep their phase, and unknown or incomplete npm evidence cannot fall through to empty-stderr recovery. Startup metadata snapshots are synchronized across event construction. All 28 actionable PR review threads were addressed across the implementation and documentation.
