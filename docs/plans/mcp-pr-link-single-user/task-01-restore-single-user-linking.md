---
id: "01-restore-single-user-linking"
title: "Restore single-user GitHub linking"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001
acceptance_criteria:
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.1
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.2
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.3
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.4
system_design:
  - ../../specs/integrations/system-design/task-change-link-mcp.md
---

# Task 01: Restore single-user GitHub linking

## Summary

Supply the trusted single-user identity for GitHub associations when the host
explicitly has authentication disabled. Preserve existing authenticated
identities and the generic MCP scope contract.

## In scope

- Host identity resolver and coordinator wiring described in the design.
- Test-first mode matrix and real MCP handler/coordinator regression.
- Link, replacement, rejection, and existing rollback coverage.

## Out of scope

Global auth or MCP scope changes, credentials, migrations, UI, GitLab changes,
live instance operations, and publishing.

## Acceptance

1. `TestTaskChangeLinkMCPSingleUser` first fails with the observed missing
   identity error, then passes through actual handler/coordinator dispatch and
   the production identity resolver, returning the canonical link and passing
   the correct workspace and `default-user` to the fake provider.
2. Setup/enabled/unavailable auth and present-empty identities never receive a
   synthetic fallback. Existing real identities survive unchanged; forged
   callers, foreign workspaces, and unattached repositories fail before provider
   calls. The generic disabled-mode scope regression remains unchanged.
3. Single-user replacement works, identical replacement stays a no-op, failed
   incoming links preserve old associations, and existing rollback checks pass.

## Verification

Run from the repository root. During Red, run the new MCP regression alone;
after implementation run this complete targeted block:

```bash
(cd apps/backend && go test ./internal/backendapp -run 'Test(TaskChange|GitHubChangeURL)' -count=1)
(cd apps/backend && go test ./internal/mcp/handlers -run 'Test(TaskChange|ValidateTaskChange)' -count=1)
(cd apps/backend && go test ./internal/mcp/scope -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/backendapp/task_change_link_coordinator.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/task_change_link_coordinator_test.go`
- `apps/backend/internal/backendapp/task_change_link_identity.go` (host resolver)
- `apps/backend/internal/backendapp/task_change_link_identity_test.go` (mode and replacement tests)
- `apps/backend/internal/backendapp/task_change_link_mcp_test.go` (new)
- This work order and `plan.md` for execution results.

## Dependencies

None. Read `apps/backend/AGENTS.md` before implementation.

## Risks

The trusted mode resolver must default to denying fallback when unavailable.
Do not substitute workspace automation credentials or relax provider checks.
Use temporary SQLite fixtures and fake providers, never the live database.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/task-change-link-mcp.md).
- [Design](../../specs/integrations/system-design/task-change-link-mcp.md#github-caller-identity).
- Existing coordinator harness and MCP `task_change_link_test.go` principal tests.
- `internal/mcp/scope/scope.go` and `internal/auth/httpmw/middleware.go`.

## Results

Completed 2026-09-15.

- Red: `go test ./internal/backendapp -run '^TestTaskChangeLinkMCPSingleUser$' -count=1`
  failed with the recorded `VALIDATION_ERROR` and missing GitHub user identity.
- Green: `go test ./internal/backendapp -run '^TestTaskChangeLinkMCPSingleUser' -count=1`
  passed, including reach rejection cases.
- `go test ./internal/backendapp -run 'Test(TaskChange|GitHubChangeURL)' -count=1`:
  passed (8.174s). The mode matrix covers ten cases; replacement covers three.
- `go test ./internal/mcp/handlers -run 'Test(TaskChange|ValidateTaskChange)' -count=1`:
  passed (0.924s).
- `go test ./internal/mcp/scope -count=1`: passed (0.055s).
- `python3 scripts/list-docs.py validate`: passed (269 decisions, 923 specs).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

Go commands ran from `apps/backend` with sandbox escalation for the existing
build cache. The first sandboxed attempt could not write that cache. A setup-mode
fixture initially omitted the users table; the fixture now initializes both
stores and the final backend run above passes.

The production host resolver is used by the new tests. Provider I/O is faked;
the MCP handler and coordinator and task SQLite repository are real. No live
PR mutation, deployment, commit, or push was performed. Public documentation
already describes the restored contract; internal design and delivery records
are updated. No subagents were used.
