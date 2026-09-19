---
id: "03-bound-fallback-resume-history"
title: "Bound fallback resume history"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/automation-continuity.md"
---

# Task 03: Bound Fallback Resume History

## Acceptance

- `GenerateResumeContext` includes at most the newest 50 non-empty user or assistant text messages,
  in chronological order with existing per-message truncation.
- Tool calls, tool results, status events, unknown types, and contentless messages are excluded
  before selection. They do not appear and do not consume the limit.
- The new prompt remains a separate current request and does not consume the message limit.
- Generating fallback context does not delete or rewrite durable history, and native ACP resume and
  agent-managed compaction behavior remain unchanged.

## TDD scenarios

1. RED: Cover 49, 50, and 51 user/assistant messages and assert exact first/last messages.
2. RED: Replace the existing every-entry-kind expectation. Put 150 tool calls and results after an
   assistant message. Prove that no tool event appears or consumes a slot and the message remains
   eligible.
3. RED: Interleave unknown, status, and contentless entries. Prove that 50 conversation messages
   still appear.
4. RED: Prove chronological output, current message truncation, current-request placement, and
   unchanged source JSONL bytes.
5. GREEN: Add one named message limit and newest-window selection after message-type filtering.
6. REFACTOR: Keep selection independent from formatting so ordering and truncation stay testable.

## Verification

- `cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'TestGenerateResumeContext|TestTruncateForContext'`
- `git diff --check`

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/session_history.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_history_resume_test.go`

## Dependencies

None.

## Inputs

- Continuation fallback-history contract in the automation settings spec and ADR.
- Existing `SessionHistoryManager.GenerateResumeContext` behavior.

## Parallelism

Parallel-safe with Task 01: ownership is limited to agent lifecycle history files.

## Output contract

Report boundary fixtures, included message identities/order, excluded tool-event counts,
source-file preservation, files changed, and exact tests.

## Risks

- Counting tool events can displace the conversation messages that the fallback needs.

## Results

Implemented newest-50 filtering for non-empty user/assistant messages, tool/status exclusion, chronological reconstruction, per-message truncation, and source-history preservation. Lifecycle verification passed with 1,947 tests.
