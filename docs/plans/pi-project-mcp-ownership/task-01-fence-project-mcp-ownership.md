---
id: "01-fence-project-mcp-ownership"
title: "Fence shared project MCP ownership"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-PREP-005
acceptance_criteria:
  - AC-AGENTS-MCP-PREP-005.1
  - AC-AGENTS-MCP-PREP-005.2
  - AC-AGENTS-MCP-PREP-005.3
  - AC-AGENTS-MCP-PREP-005.4
  - AC-AGENTS-MCP-PREP-005.5
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
---

# Task 01: Fence shared project MCP ownership

## Summary

Implement shared project MCP claims and generation-aware cleanup for Pi's
`.pi/mcp.json` while preserving the existing strategy and merge contract. The
same lifecycle boundary must cover fresh launch, workspace promotion, resume,
recovery, backend restart, and launch rollback.

## In scope

- Add the workspace/file coordination fence and ownership state.
- Record per-execution path/fingerprint claims and persist them through the
  existing execution metadata allowlist.
- Transfer known Kandev ownership to a successor generation before the
  successor is registered.
- Guard cleanup with active claims, current fingerprint, regular-file and
  containment checks.
- Release claims on pre-registration and registered launch rollback.
- Add deterministic regression tests and race coverage in the listed backend
  packages.

## Out of scope

- Changing Pi's generated JSON schema, transport mapping, or eager behavior.
- User-level Pi configuration, credential handling, or profile MCP semantics.
- Cursor native import ownership.

## Acceptance

- An old execution can be cleaned after a new execution materializes the same
  workspace path, and the file remains present with the new execution's port.
- A Kandev-created file is removed only after its final claim ends and its
  fingerprint is unchanged; a user-existing or user-modified file remains.
- Concurrent and restart-shaped paths are covered with deterministic barriers,
  and the focused package tests pass with the race detector.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/mcpconfig ./internal/agent/runtime/lifecycle -count=1)
(cd apps/backend && go test -race ./internal/agent/mcpconfig ./internal/agent/runtime/lifecycle -count=1)
(cd apps/backend && make fmt)
(cd apps/backend && make typecheck test lint)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/decisions docs/specs docs/plans/pi-project-mcp-ownership
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_project_mcp.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/persistence.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_mcpfiles_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_project_mcp_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/passthrough_mcp_ownership_test.go`

## Dependencies

None.

## Risks

- A missing or mismatched fingerprint must leave a file untouched rather than
  attempt an unsafe cleanup.
- Metadata must tolerate both in-memory Go shapes and JSON-decoded restart
  shapes without copying secrets or live maps across locks.

## Parallelism

`sequential`

## Inputs

- [REQ-AGENTS-MCP-PREP-005](../../specs/agents/requirements/agent-mcp-preparation.md)
- [Agent MCP preparation system design](../../specs/agents/system-design/agent-mcp-preparation.md)
- [ADR-2026-10-08-pi-project-mcp-ownership](../../decisions/2026-10-08-pi-project-mcp-ownership.md)
- Existing Pi injection contract in [ADR 0020](../../decisions/0020-pi-project-mcp-config-injection.md)

## Results

Implemented with a manager-local mutex plus a cross-process advisory lock,
per-execution path/fingerprint claims, generation adoption, persistent
session-scoped metadata, and fail-closed cleanup. Added deterministic tests for
successor replacement, cleanup ordering, final owned-file cleanup, user-file
merge/retention, user edits, recovery-shaped metadata, and concurrent
materialization. Focused race and vet checks pass. The full package/backend
suite still reports unrelated pre-existing failures documented in the plan's
verification results.
