---
id: "01-preserve-handoff-submission"
title: "Preserve durable ownership at foreground handoff"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.1
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.7
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 01: Preserve durable ownership at foreground handoff

## Summary

Keep a successful foreground handoff separate from durable terminal completion.
The original submission remains live until canonical projection receives its real terminal.
The lifecycle waiter still returns at the existing attested handoff boundary.

## In scope

- Add a failing regression at `finishPromptExecutorDispatch` with the real SQL repository.
- Retain `streams.EventTypeForegroundIdle` as a nonterminal result at that boundary.
- Preserve live submission state without creating an interruption block for a successful handoff.
- Cover terminal-before-return, later terminal, shutdown after handoff, and a newer prompt generation.
- Preserve legacy delivery, dispatch-only acknowledgement, deterministic rejection, and existing handoff admission behavior.

## Out of scope

- Historical completed SQL records and native retirement changes, owned by Task 02.
- Self-update policy, process survival, queue policy, UI, and provider notification changes.
- Runtime flag additions, default changes, migrations, or new terminal events.

## Acceptance

1. A handoff result returns successfully and leaves an otherwise dispatching durable submission unresolved with empty terminal fields.
2. A real terminal settles its exact submission once. A late original terminal cannot release or settle a successor.
3. Shutdown after handoff retains recoverable uncertainty. Existing known terminal outcomes and compatibility tests remain valid.

## TDD sequence

1. Add `TestForegroundHandoffPreservesDurableSubmission` in `agent_delivery_handoff_test.go`.
2. Admit a real durable submission and call the dispatch finalizer with `StopReason: streams.EventTypeForegroundIdle`.
3. Run the focused test. Expect the current code to write completed instead of dispatching, with no terminal evidence.
4. Prevent `markCompleted` for that successful nonterminal result at the finalizer boundary.
5. Add `TestForegroundHandoffLateTerminalSettlesOriginalSubmission` and `TestForegroundHandoffInterruptedBeforeTerminal`.
6. Drive real canonical terminal projection for late and already-projected terminals. Do not substitute a direct row update for terminal settlement.
7. Run the commands below and record their actual results.

Use existing lifecycle tests for handoff release and stale completion compatibility.
The temporary investigation test is documented in `plan.md` and is not present in the workspace.

## Verification

Run from the repository root. Every command has its own working directory.

```bash
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -tags fts5 -race -p=1 ./internal/orchestrator -run 'Test(ForegroundHandoff|PrepareAgentDeliverySubmission|AgentDeliverySubmission|PromptTask_(BackgroundWork|ClaudeExperiment|NonClaude))' -count=1 -timeout 180s)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -tags fts5 -race -p=1 ./internal/agent/runtime/lifecycle -run 'Test(SendPrompt_AuthoritativeHandoff|HandleAgentEvent_.*(ForegroundIdle|Handoff)|NativeResumeReconcilesJournalUnknownSQLDispatch)' -count=1 -timeout 180s)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/task_operations.go apps/backend/internal/orchestrator/agent_delivery_handoff_test.go
git diff --check
```

Run the new focused regression alone first to record behavioral RED.
The broader focused commands establish compatibility after the correction.
If implementation changes another Go file, include it in the changed-file lint command and its owning focused test command.

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`: `finishPromptExecutorDispatch`.
- `apps/backend/internal/orchestrator/agent_delivery_handoff_test.go` (new).

Read-only patterns:

- `apps/backend/internal/orchestrator/agent_delivery_submission.go` and its tests.
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`: `handlePromptHandoffEvent`.
- `apps/backend/internal/agent/runtime/lifecycle/session.go`: `waitForPromptDone`.
- `apps/backend/internal/agent/runtime/lifecycle/session_test.go`: `TestSendPrompt_AuthoritativeHandoffReleasesNextGeneration`.

## Dependencies

None. Implement this work order before Task 02 so new sessions stop acquiring the defect.

## Risks

- A terminal can project before the blocking prompt returns. Preserve that authoritative outcome.
- The handoff result does not change coarse session or workflow admission authority.
- A predecessor terminal must retain its submission and generation after successor dispatch.

## Parallelism

`sequential`

## Inputs

- [Owning requirement](../../specs/platform/requirements/durable-restart-reconciliation.md), AC 002.1 and 002.7.
- [Convergence design](../../specs/platform/system-design/session-recovery-convergence.md#foreground-handoff-and-unproven-sql-completion), producer boundary.
- `apps/backend/AGENTS.md` and `.agents/skills/tdd/references/backend-tests.md`.

## Results

Implemented the producer boundary in
`apps/backend/internal/orchestrator/task_operations.go`. A successful
`foreground_idle` handoff now leaves the durable submission dispatching; no
completion label or recovery block is created. A terminal projected before the
return keeps its outcome, and a later terminal settles only the original
submission.

Permanent regressions live in
`apps/backend/internal/orchestrator/agent_delivery_handoff_test.go`:

- `TestForegroundHandoffPreservesDurableSubmission`
- `TestForegroundHandoffLateTerminalSettlesOriginalSubmission`, including a
  successor started before the predecessor's late terminal. Only the
  predecessor settles; the successor stays dispatching until its own terminal.
- `TestForegroundHandoffInterruptedBeforeTerminal`

Behavioral RED was reproduced by disabling the nonterminal guard: the first
test failed with a completed submission and empty terminal fields.
The final focused orchestrator and lifecycle commands passed under `-race`,
and changed-file Go lint reported 0 issues.
