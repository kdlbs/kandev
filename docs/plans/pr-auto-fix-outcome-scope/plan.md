---
created: 2026-09-11
status: complete
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
system_design:
  - ../../specs/ui/system-design/ci-pr-automation-02.md
legacy_specs: []
---

# Fix plan: PR auto-fix outcome scope

## Overview

Clarify when an agent must report a PR auto-fix outcome.
First correct the tool and dispatch guidance. Then explain rejected calls and
prove that ordinary turns cannot change auto-fix state.
The implementation applies the approved guidance and rejection-scope package
without changing automation eligibility or retry behavior.

## Diagnosis and evidence

Affected task: `93a74cfc-6135-452e-85d1-ccd6d7af78cd`, linked PR #3590.
Session: `2edf3b96-66b5-4409-9238-36c97f4d3a63`.
Turn: `71c201a0-4115-4ca3-bb61-bdc60dd48d6c`.

- The original user requested implementation, PR creation, a 15-minute wait,
  and manual PR fixup. This request did not enable automatic PR repair.
- The affected turn started at `2026-09-11T09:28:21Z` from a sibling review
  message, `9f209c9c-d7d3-54b6-bee4-f3b6a7501a2d`.
- At `2026-09-11T10:12:12Z`, message
  `5865c632-ea68-4140-a507-6a4b6391180d` called
  `report_pr_auto_fix_outcome_kandev` with `action_taken`.
  Its summary described manual review fixes and head `46ce139`.
- The tool returned `VALIDATION_ERROR` with
  `CI auto-fix attempt not found or no longer matches`.
- Read-only inspection of `/root/.kandev/data/kandev.db` found zero automation
  option rows and zero attempt-state rows for this task. PR #3590 remained linked.
  The legacy task settings row was absent too. `GetTaskPRAutomationOptions`
  returns disabled defaults for a missing row.
- The recorded turn metadata had no auto-fix binding. The complete primary
  conversation contained the original user prompt and sibling review prompt,
  with no Kandev auto-fix dispatch message.

Current database state alone cannot prove every historical switch value.
The transcript, turn record, rejected call, and disabled defaults agree with the
user's report. No evidence indicates a scheduler dispatch with auto-fix disabled.
No settings, sessions, GitHub objects, or live records changed during diagnosis.

## Root cause

The agent used an automatic-attempt reporting tool to summarize manual fixup.
`registerPRAutomationTools` exposes the tool to GitHub-capable task sessions,
independent of automation settings or an active attempt. Its description names
an auto-fix turn but does not explicitly exclude ordinary PR work.
The existing dispatch protocol also lacks an explicit expiry across later turns.
That latter omission is a preventive concern, not a proven trigger in this incident.

The conditional store write correctly rejected the call. This repair addresses
agent guidance and error recovery, not an observed authorization bypass.
The exact internal reason for the model's choice is not independently provable.

## Requirement reconciliation

`AC-UI-CI-PR-AUTOMATION-001.10` already requires exact turn binding.
The implementation preserved that boundary in this incident.
Criteria `.16` and `.17` clarify guidance and rejection behavior that were missing.
The existing UI-owned automation pair remains authoritative during migration.
The integration index owns adjacent provider signals, not a second copy of this
attempt contract. No new ADR is needed because the existing outcome ADR already
separates ordinary turns from automatic attempts.

## Scope

### In scope

- Explicit current-turn eligibility in tool and immutable dispatch guidance.
- A specific explanation for an unmatched outcome report.
- Regression coverage for valid and invalid report contexts.

### Out of scope

- Dynamic catalog removal, a new eligibility endpoint, or session restarts.
- Automatic settings changes or conversion of manual work into an attempt.
- Changes to retry counts, provider progress, queue delivery, or GitLab behavior.
- UI layout, translated browser copy, and modification of repository skills.
- Repairing or changing the affected task or PR.

## Technical approach

Keep the existing provider-scoped catalog and backend identity checks.
Update the reporting tool description in `internal/mcp/server/server.go`.
Update `ciAutomationOutcomeProtocol` in
`internal/orchestrator/event_handlers_github_ci_automation.go`.
Map the missing-attempt sentinel separately in
`internal/mcp/handlers/task_pr_automation.go`.
Retain the sentinel, error code, schema, and all failure semantics.

Proposed rejection meaning: no matching unresolved auto-fix attempt exists for
this turn. Use this tool only for a current Kandev auto-fix dispatch.
For ordinary PR work, finish normally without retrying this report or enabling
auto-fix. Do not claim that the switch is off from this error alone.

## Tests

| Criteria | Planned evidence |
| --- | --- |
| `.16` | `TestReportPRAutoFixOutcomeToolDescriptionScopesCurrentTurn` in new `internal/mcp/server/pr_auto_fix_outcome_scope_test.go` |
| `.16` | `TestCIAutomationOutcomeProtocolScopesCurrentTurn` in new `internal/orchestrator/ci_automation_outcome_scope_test.go` |
| `.17` | `TestReportPRAutoFixOutcomeExplainsUnmatchedTurn` in new `internal/mcp/handlers/task_pr_automation_outcome_scope_test.go` |
| `.10`, `.17` | `TestStorePRAutoFixOutcomeOrdinaryTurnHasNoSideEffects` in new `internal/github/store_ci_outcome_scope_test.go` |
| `.10` | `TestStorePRAutoFixOutcomeAcceptsAllValidOutcomes` in new `internal/github/store_ci_outcome_scope_test.go` |

The first three tests must fail before the planned correction.
The store matrix protects the already-working rejection boundary.
Exercise a real temporary SQLite store for state assertions.
Cover absent and disabled settings, enabled-without-attempt, another turn's
attempt, an already-reported attempt, and all three valid outcomes.
Include two PRs with different settings and a foreign bound attempt.

## End-to-end evidence

The observable surface is the agent tool protocol. Use the existing in-process
MCP client pattern (`callTool`) to verify the returned error and caller binding.
Store tests prove persistent effects. No browser rendering changes require
Playwright, and deterministic tests do not claim to prove every model choice.

## Work orders

- [x] [Task 01: Scope agent reporting guidance](task-01-scope-reporting-guidance.md)
- [x] [Task 02: Explain unmatched outcome reports](task-02-explain-unmatched-reports.md)

Execute sequentially. The existing
[outcome retry package](../pr-auto-fix-outcome-retries/plan.md) is implemented.
This package adds clarification without reopening or rewriting its results.

## Verification results

Implementation: complete. Both work orders are done.

- `rtk python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `rtk python3 scripts/lint-spec-files.py --all`: passed.
- `rtk git diff --check`: passed.
- `rtk go test ./internal/mcp/handlers -run 'Test.*Report.*PRAutoFix' -count=1`: passed, 7 tests.
- `rtk go test ./internal/mcp/server -run 'TestReportPRAutoFixOutcome|TestTaskPRAutomationTools' -count=1`: passed, 5 tests.
- `rtk go test ./internal/github -run 'TestStorePRAutoFixOutcomeOrdinaryTurnHasNoSideEffects|TestStoreTaskCIAutoFixAttempt' -count=1`: passed, 9 tests.
- `rtk go test ./internal/orchestrator -run 'TestCIAutomationOutcomeProtocol' -count=1`: passed, 4 tests.
- `rtk make -C apps/backend build`: passed; the environment reported only the existing missing codesign/rcodesign warning for Darwin artifacts.
- `rtk make -C apps/backend test`: the changed packages passed, but the full suite exited 2 in unrelated existing areas: process-tree probing, config/home discovery, launcher service configuration, and an Office SQLite migration.
- `rtk gofmt -l` on all changed Go files: passed.
- Work-order references and existing code symbols checked against the workspace.

## Risks

- Clear guidance reduces misuse but cannot guarantee a model never calls the tool.
  The backend guard remains mandatory.
- A static catalog must remain usable when a later auto-fix turn reuses a session.
- Historical instructions can remain in model context. The protocol must state
  its current-turn scope without weakening reporting for valid dispatches.
- A missing attempt can mean stale or completed work, not only disabled settings.

## Documentation impact

Implementation changes are limited to agent-facing backend guidance, rejection
classification, regression coverage, and the delivery record. Public guidance
remains accurate because automatic retries and controls do not change. The
implementation retains the distinction between manual fixup and
Kandev-dispatched auto-fix in agent-facing text.
