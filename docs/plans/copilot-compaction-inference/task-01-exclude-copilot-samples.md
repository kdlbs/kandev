---
id: "01-exclude-copilot-samples"
title: "Exclude ambiguous Copilot samples from inference"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CONTEXT-COMPACTION-COUNT-001
acceptance_criteria:
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.2
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.3
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.4
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.5
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.8
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.9
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.10
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.11
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.12
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.13
system_design:
  - ../../specs/ui/system-design/context-compaction-count.md
---

# Task 01: Exclude ambiguous Copilot samples from inference

## Summary

Carry backend-derived Copilot exclusion through context-window events and enforce it during atomic persistence.
Preserve latest usage and accounting while preventing comparisons across excluded baselines.

## In scope

- Add matching optional booleans to `ContextWindowEventPayload` and `ContextWindowData`.
- Classify observations in `EventPublisher.PublishContextWindow` from `AgentExecution.AgentID` and the existing `agents.CopilotACPAgentID` constant.
- Persist the marker through `handleContextWindowUpdated`, retaining ordered writes and generation guards.
- Guard incoming and previous eligibility in SQLite and PostgreSQL, within one returning statement using bound parameters.
- Cover first/equal/increasing/decreasing samples, duplicates, historical counts, and unrelated metadata.
- Cover eligible-to-excluded and excluded-to-eligible samples in one session, followed by a normal eligible decrease.
- Reopen the database with an excluded baseline and confirm that a smaller eligible sample does not increment.
- Cover old unmarked samples, new excluded samples, reset behavior, and restored older agentctl context events.

## Out of scope

- Raw agentctl event additions, ACP dialect changes, provider file reads, or context-owner guessing.
- Frontend copy, public docs, explicit compaction events, and historical count repair.
- New schema, flags, metrics, or generalized provider policy registries.

## Acceptance

1. `TestContextWindowCopilotUsageDoesNotInferCompaction` fails before implementation with counts `0,1,1,2,2`, then passes with five zeros.
2. That test traverses the real publisher, watcher conversion, handler, SQLite repository, and metadata publication. Every usage value survives.
3. Both SQL dialects satisfy the eligibility matrix, preserve history/keys and reset guards, and retain normal inference and accounting.

## Verification

Run from the repository root. Supply `KANDEV_TEST_POSTGRES_DSN` through the normal test environment when PostgreSQL is available.
Record an unavailable PostgreSQL fixture as incomplete parity evidence.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestPublishContextWindow' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator ./internal/orchestrator/watcher -run '^TestContextWindow' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^Test(UpdateSessionContextWindow|PostgresUpdateSessionContextWindow)' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^Test(UsageTracker|ConsumeUsageDelta|ConvertUsageUpdate|ConvertNotification_UsageUpdateFixture)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/events.go apps/backend/internal/agent/runtime/lifecycle/event_types.go apps/backend/internal/agent/runtime/lifecycle/events_context_window_test.go apps/backend/internal/orchestrator/watcher/watcher.go apps/backend/internal/orchestrator/watcher/context_window_test.go apps/backend/internal/orchestrator/event_handlers_git.go apps/backend/internal/orchestrator/context_window_compaction_test.go apps/backend/internal/task/repository/sqlite/session.go apps/backend/internal/task/repository/sqlite/session_context_window_test.go apps/backend/internal/task/repository/sqlite/postgres_context_window_test.go
git diff --check
```

Run the new integration regression first for RED, then rerun it after the correction.
Every command must discover its intended tests. No-tests output is not acceptance evidence.
If a new filename changes, update the lint command and the package together.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/events_context_window_test.go` (new)
- `apps/backend/internal/orchestrator/watcher/watcher.go`
- `apps/backend/internal/orchestrator/watcher/context_window_test.go` (new)
- `apps/backend/internal/orchestrator/event_handlers_git.go`
- `apps/backend/internal/orchestrator/context_window_compaction_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/session_context_window_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/postgres_context_window_test.go` (new)

## Dependencies

None.

## Risks

- Classification must use provider identity, including after restore, rather than an execution UUID.
- Dropping the boolean during serialization silently restores false increments.
- Both SQL branches must preserve legacy defaults and persist excluded samples without counting them.
- Genuine Copilot usage drops also stop increasing the inferred count.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/context-compaction-count.md), criteria in frontmatter.
- [System design](../../specs/ui/system-design/context-compaction-count.md), eligibility through recovery sections.
- [Package evidence and compatibility matrix](plan.md).
- Existing `events_git_status_test.go` publisher pattern, `createContextWindowHandler`, `setupTestRepo`, and `seedSession`.
- Existing context-window tests in `session_test.go`, `postgres_schema_test.go`, and `event_handlers_git_test.go`.
- `apps/backend/AGENTS.md` and ADR 0044.

## Results

Implemented. `ContextWindowEventPayload` and `watcher.ContextWindowData` carry the optional
`compaction_inference_disabled` marker. `EventPublisher.PublishContextWindow` derives it from
`AgentExecution.AgentID` and `agents.CopilotACPAgentID`; the handler persists it with the sample;
both SQL branches bind the incoming marker and inspect the previous stored marker in the same
statement.

- RED: after temporarily reverting the publisher classification and stored eligibility guard,
  `TestContextWindowCopilotUsageDoesNotInferCompaction` failed. The design-time SQLite overlay had
  already produced the pre-fix stored counts `0,1,1,2,2`.
- GREEN: `go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestContextWindow' -count=1`
  passed (five Copilot samples, five zero counts, retained usage values, matching metadata patches).
- GREEN: `go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestPublishContextWindow' -count=1`
  passed (Copilot, restored Copilot, other provider, and legacy execution cases).
- GREEN: `go test -trimpath -tags fts5 ./internal/orchestrator/watcher -run '^TestContextWindow' -count=1`
  passed (marker round trip and legacy unmarked event).
- GREEN: `go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^Test(UpdateSessionContextWindow|PostgresUpdateSessionContextWindow)' -count=1`
  passed the mixed eligible/excluded matrix, duplicates, reset, missing marker, reopen baseline,
  the existing strict-drop regression, and the PostgreSQL eligibility plan. PostgreSQL ran against
  a disposable `postgres:16-alpine` container through `KANDEV_TEST_POSTGRES_DSN`; all 8 targeted
  tests passed.
- Full package runs passed: `./internal/agent/runtime/lifecycle`, `./internal/orchestrator`,
  `./internal/orchestrator/watcher`, and `./internal/task/repository/sqlite`.
- `bash scripts/lint-go-changed ...` reported 0 issues.
- Files changed: the payload type, publisher, watcher data, handler, SQLite/PostgreSQL SQL, and five
  new test files. No schema, raw agentctl, frontend, or accounting change. PostgreSQL parity passed
  against a disposable PostgreSQL 16 container.
