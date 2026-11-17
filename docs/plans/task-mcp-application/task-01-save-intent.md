---
id: "01-save-intent"
title: "Persist human application intent"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-APPLY-001
  - REQ-AGENTS-MCP-APPLY-002
acceptance_criteria:
  - AC-AGENTS-MCP-APPLY-001.1
  - AC-AGENTS-MCP-APPLY-002.1
  - AC-AGENTS-MCP-APPLY-002.2
system_design:
  - ../../specs/agents/system-design/task-mcp-application.md
---

# Task 01: Persist human application intent

## Summary

Add atomic task-save and per-session intent persistence. Expose read-only application status without enabling replacement yet.

## In scope

Persist application rows with expected revision checks, provenance, effective selection comparison, and durable wake recovery.
Handle concurrent edits, failed commits, no-op saves, newer sessions, and permission changes.

## Out of scope

Runtime replacement and UI controls.

## Acceptance

1. A committed human save persists its application intent atomically. Rejected saves create no intent.
2. Equivalent selections do not request replacement. Concurrent saves retain the latest valid target.
3. Upgrade creates no historical restart requests. Existing callers remain compatible.

## Verification

Use TDD for changed logic. Run commands from the repository root. New test paths are targets created by this work order.

```bash
(cd apps/backend && go test ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/task/repository/sqlite ./internal/orchestrator -run 'TestBuiltin(Application|Policy|SessionView)' -count=1)
bash scripts/lint-go-changed $(git ls-files --modified --others --exclude-standard -- '*.go')
```

## Files likely touched

- `apps/backend/internal/agent/settings/store/builtin_mcp_policy_write.go`
- `apps/backend/internal/agent/settings/store/builtin_application.go`
- `apps/backend/internal/orchestrator/builtin_mcp_management.go`
- `apps/backend/internal/agent/settings/store/builtin_application_test.go`
- `apps/backend/internal/orchestrator/builtin_application_view_test.go`
- `apps/backend/internal/task/repository/sqlite/ (add migration and application transaction tests)`

## Dependencies

None

## Risks

Persistence must survive a lost wake and a cancelled HTTP waiter.

## Parallelism

Sequential. No delegation is authorized.

## Inputs

- [Requirements](../../specs/agents/requirements/task-mcp-application.md).
- [System design](../../specs/agents/system-design/task-mcp-application.md).
- [Plan and test mapping](plan.md).
- Scoped backend/web guidance and existing builtin MCP tests.

## Results

Implementation on 2026-11-15 adds an additive application table without historical requests.
Task preference saves write per-session intent in the same transaction. Effective no-op saves do not create requests.
New targets preserve an active attempt's identity. Workspace changes cannot reuse an old workspace's intent.
The session view exposes application state separately from the accepted binding and omits actor provenance.
No coordinator, automatic stop, or UI behavior is enabled by this work order.

TDD observed failures before the transaction, workspace isolation, and status projection changes.
Focused policy/application tests and the application storage race tests pass.
Final regression command passed after the last production edit:
`(cd apps/backend && go test ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/task/repository/sqlite ./internal/orchestrator -run 'TestBuiltin(Application|Policy|SessionView)' -count=1)`.
Storage race command passed:
`(cd apps/backend && go test -race ./internal/agent/settings/store ./internal/task/repository/sqlite -run TestBuiltinApplication -count=1)`.
The final claim-specific edit has a separate passing race run in work order 02.
Changed-package lint with the repository-pinned golangci-lint 2.9.0 passed with zero issues.
The default five-minute run timed out during dependency loading. The completed run used a 15-minute timeout,
`GOMEMLIMIT=3GiB`, `GOMAXPROCS=2`, and the same repository configuration.
A second pass explicitly included every new Go file and passed with zero issues:
`GOMEMLIMIT=3GiB GOMAXPROCS=2 golangci-lint run ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/orchestrator ./internal/task/models ./internal/task/repository/sqlite --new-from-patch=/tmp/kandev-mcp-tools/changes.patch --timeout=15m --concurrency=2 --allow-serial-runners`.
The temporary patch combined tracked changes with all untracked Go files and used backend-relative paths.
The binary was the repository-pinned 2.9.0 release. No repository lint configuration changed.
