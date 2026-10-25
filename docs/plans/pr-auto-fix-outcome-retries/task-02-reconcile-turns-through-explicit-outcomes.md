---
id: "02-reconcile-turns-through-explicit-outcomes"
title: "Reconcile turns through explicit outcomes"
status: done
wave: 2
depends_on:
  - "01-persist-pr-auto-fix-attempt-state"
plan: "plan.md"
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
acceptance_criteria:
  - AC-UI-CI-PR-AUTOMATION-001.9
  - AC-UI-CI-PR-AUTOMATION-001.10
  - AC-UI-CI-PR-AUTOMATION-001.11
  - AC-UI-CI-PR-AUTOMATION-001.12
  - AC-UI-CI-PR-AUTOMATION-001.13
  - AC-UI-CI-PR-AUTOMATION-001.14
  - AC-UI-CI-PR-AUTOMATION-001.15
system_design:
  - ../../specs/ui/system-design/ci-pr-automation-01.md
  - ../../specs/ui/system-design/ci-pr-automation-02.md
  - ../../specs/ui/system-design/ci-pr-automation-03.md
---

# Task 02: Reconcile Turns Through Explicit Outcomes

## Summary

Bind direct and queued GitHub auto-fix delivery to exact turns, add the
task-bound outcome tool, and make undispositioned turns retryable.

## In scope

- Extend the provider-neutral dispatch result with queue-entry and turn
  identity while keeping provider data in the GitHub adapter.
- Stamp GitHub auto-fix identity on the queued/user message and bind it after
  direct or queued prompt acceptance.
- Register `report_pr_auto_fix_outcome_kandev` only for GitHub-capable task MCP
  servers. Accept only `action_taken`, `non_actionable`, or `blocked` plus a
  bounded plain-text summary.
- Resolve the current turn server-side and reject stale, cross-session,
  already-dispositioned, or non-auto-fix calls.
- Append immutable outcome instructions for structured and passthrough auto-fix
  prompts.
- Reconcile matching attempts from normal `agent.ready`, recoverable failure,
  and watcher evaluation paths without changing user cancellation behavior.
- Preserve queue coalescing, session pinning, auto-merge ordering, and the
  10-round cap.

## Out of scope

- Trusting final response text as an outcome.
- A user-facing manual outcome button.
- GitLab tool registration or lifecycle changes.
- Workflow-step completion semantics.

## Acceptance

- A turn with no outcome becomes retryable and receives a later bounded round
  for an unchanged settled snapshot.
- Explicit non-actionable and blocked outcomes suppress the unchanged snapshot;
  action taken waits for progress and retries after the deadline if unchanged.
- Direct, queued, replaced, restarted, stale-turn, exhausted, disabled, and
  archived cases have focused regression coverage.

## Verification

```bash
make -C apps/backend test ARGS='./internal/orchestrator/... ./internal/mcp/handlers/... ./internal/mcp/server/...'
make -C apps/backend lint
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/ci_automation_dispatch.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
- `apps/backend/internal/orchestrator/event_handlers_queue_general_test.go`
- `apps/backend/internal/mcp/handlers/task_pr_automation.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/pkg/websocket/actions.go`

## Dependencies

- Task 01.

## Risks

- A completion callback can race the state write after prompt acceptance. Tests
  must cover both orderings and leave the attempt retryable, not acknowledged.
- Provider-neutral queue code must not gain GitHub service dependencies.

## Parallelism

`sequential`

## Results

Implemented direct and queued turn binding, task-bound outcome reporting,
immutable structured and passthrough protocol instructions, lifecycle
reconciliation, retry scheduling, provider-progress deadlines, and cap-aware
deduplication. Added orchestrator, MCP handler, and MCP server regression tests.
