---
id: "04-scoped-mcp-callback"
title: "Add session-scoped cloud MCP callbacks"
status: complete
wave: 4
depends_on:
  - "03-durable-submissions"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-CURSOR-CLOUD-001
  - REQ-EXECUTORS-CURSOR-CLOUD-005
acceptance_criteria:
  - AC-EXECUTORS-CURSOR-CLOUD-005.1
  - AC-EXECUTORS-CURSOR-CLOUD-005.2
  - AC-EXECUTORS-CURSOR-CLOUD-005.3
  - AC-EXECUTORS-CURSOR-CLOUD-005.4
  - AC-EXECUTORS-CURSOR-CLOUD-001.3
system_design:
  - ../../specs/executors/system-design/cursor-cloud.md
---

# Task 04: Add session-scoped cloud MCP callbacks

## Summary

A valid grant exposes only the current task tool profile; it never acts as an external user PAT.
Use TDD for changed logic. Keep results pending until the listed checks pass.

## In scope

- Add proposed internal/mcp/managed transport and backend route composition with hashed, bounded-lifetime bearer grants.
- Resolve task, session, user, workspace, execution, generation, and MCP profile from trusted records. Reuse existing scope and tool handlers.
- Use only the managed task profile (questions, plans, rich output, step completion, and optional one-shot title); reject cross-task/session tool arguments and omit task creation, workspace administration, executor, repository-admin, and shell tools.
- Preserve title ownership, completion gates, and pending question behavior. Revalidate grants after long-running questions complete.
- Add callback configuration checks and connection diagnostics; distinguish local probe success from actual Cursor reachability.
- Revoke grants on terminal work and disable. Reject stale, expired, cross-session, cross-workspace, and caller-forged identity without side effects.

## Out of scope

- Work assigned to later tasks, unrelated refactors, and release promotion.
- Paid cloud execution during automated tests.

## Acceptance

- A valid grant exposes only the narrow current-task profile; it never acts as an external user PAT or authorizes another task/session.
- Question and completion handlers preserve their current barriers through remote transport, including grant revocation while a question waits.
- Credential headers and callback grants are redacted, and disabled or invalid callbacks cannot dispatch tools.

## Verification

Run from the repository root. New test paths are implementation outputs, not tests available during this planning turn.

```bash
(cd apps/backend && go test ./internal/mcp/managed ./internal/mcp/scope ./internal/mcp/profile ./internal/mcp/server -count=1)
(cd apps/backend && go test ./internal/backendapp -run 'TestCloud|TestCursorCloud' -count=1)
```

### Evidence mapping

- 005.1, 005.2, 001.3: `internal/mcp/managed/transport_test.go: TestGrantScope, TestGrantRevocationAfterQuestionWait, TestDisabledGrant, TestGrantRejectsCrossSessionBinding`; `internal/mcp/server/managed_http_test.go: task profile and cross-task argument checks`.
- 005.3, 005.4: `internal/backendapp/cursor_cloud_mcp_test.go: TestCloudQuestionBarrier, TestCloudCompletionGuard, TestCloudCallbackAdmission`.

## Files likely touched

- `apps/backend/internal/mcp/managed/ (new)`.
- `apps/backend/internal/mcp/scope/`.
- `apps/backend/internal/mcp/profile/`.
- `apps/backend/internal/mcp/server/`.
- `apps/backend/internal/backendapp/ (new managed MCP composition)`.
- `apps/backend/internal/task/repository/sqlite/managed_agent_grants.go (new)`.

## Dependencies

03-durable-submissions

## Risks

A local callback probe cannot prove remote reachability. A real cloud MCP call is a rollout gate, not something fixture tests establish.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/cursor-cloud.md).
- [System design](../../specs/executors/system-design/cursor-cloud.md).
- [Proposed runtime ADR](../../decisions/2026-09-25-managed-remote-agent-runtime.md).
- Source baseline and code patterns listed in the plan.

## Results

Implemented the scoped callback transport, operation-bound hashed grants, task-only MCP profile, post-question grant revalidation, configuration diagnostics, and backend route composition.

- Passed: `(cd apps/backend && go test ./internal/mcp/managed ./internal/mcp/scope ./internal/mcp/profile ./internal/mcp/server -count=1)`.
- Passed: `(cd apps/backend && go test ./internal/backendapp -run 'TestCloud|TestCursorCloud' -count=1)`.
- Passed: `make -C apps/backend lint`.
- Passed: `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.

### Review remediation

The issued callback URL now includes the grant path separator and is exercised through the actual issued URL, including bearer authorization and route dispatch. Passed: `(cd apps/backend && go test ./internal/mcp/managed ./internal/mcp/scope ./internal/mcp/profile ./internal/mcp/server -count=1)`.
