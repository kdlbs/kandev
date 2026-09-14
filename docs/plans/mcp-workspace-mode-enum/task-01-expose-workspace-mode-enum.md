---
id: "01-expose-workspace-mode-enum"
title: "Expose workspace mode enum"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MCP-WORKSPACE-MODE-004
acceptance_criteria:
  - AC-TASKS-MCP-WORKSPACE-MODE-004.1
  - AC-TASKS-MCP-WORKSPACE-MODE-004.2
  - AC-TASKS-MCP-WORKSPACE-MODE-004.3
  - AC-TASKS-MCP-WORKSPACE-MODE-004.4
system_design:
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
---

# Task 01: Expose workspace mode enum

## Summary

Advertise supported materialization choices and prove the enum survives the
shared task/external registration. Document the stricter MCP input boundary
without changing backend policy handling.

## In scope

- Follow `/tdd`: write the schema and wrapped-call regression first, confirm
  missing-enum failures, then add the enum/description and rerun.
- Serialize the registered tool and attachment schema in task/external modes;
  assert string type, exact ordered enum, optionality, no default, and the
  required description semantics. Reuse `newTaskModeServer`, `New`, and
  `mcpToolInputSchema`; do not introduce a second schema implementation.
- Drive accepted values and omission through `callTool` with a recording
  backend. Reject `shared`, `shared_group`, empty/whitespace strings, padded
  values, and an unknown value without backend dispatch. Use fresh backends
  per case. Do not mistake a recording backend for proof of parent validation.
- Add a direct `resolveMCPWorkspacePolicy` matrix for omission/blank with and
  without parent, both modes, explicit inheritance without parent, whitespace
  trimming, and unsupported values. Keep production handler code untouched.
- Update the two public pages and record compatibility and validation results.

## Out of scope

New modes, normalization exceptions, execution changes, Office tools, UI,
custom prompts, running instances, parent changes, subagents, and PR merge.

## Acceptance

1. Both catalogs and attachment schema expose the exact optional enum without
   a default, with materialization and parent-dependent omission guidance.
2. Wrapped-call tests prove accepted and rejected schema input; direct handler
   tests preserve defaulting, trimming, and the parent requirement.
3. Public docs and delivery results explicitly describe blank-input narrowing;
   all focused tests and document checks pass.

## Verification

Run from repository root; each Go command has its own working directory:

```bash
(cd apps/backend && GOCACHE=/tmp/kandev-workspace-enum-go-cache go test ./internal/mcp/server -run 'TestCreateTaskWorkspaceMode|TestCreateTask_ToolSchema_HasParentID|TestToolArgumentValidation|TestMCPAttachmentObserverPublishesStructuredInputSchema' -count=1)
(cd apps/backend && GOCACHE=/tmp/kandev-workspace-enum-go-cache go test ./internal/mcp/handlers -run 'TestResolveMCPWorkspacePolicyCompatibility|TestHandleCreateTask_Subtask(DefaultsToParentWorkspaceAndWorkflow|CanRequestNewWorkspaceMode)$' -count=1)
(cd apps/backend && GOCACHE=/tmp/kandev-workspace-enum-go-cache go test ./internal/mcp/server -run 'TestServerMode(Config|Office)_RegistersCorrectTools|TestServerSurfaceAutomationHasFixedCoordinatorCatalog' -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the new schema test before production edits to establish red, then all
commands above after implementation. If Go caches need a writable location,
set `GOCACHE` to an isolated directory under `/tmp`.

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/create_task_workspace_mode_test.go` (new)
- `apps/backend/internal/mcp/handlers/workspace_policy_test.go` (new)
- `docs/public/automation-and-mcp.md`, `docs/public/coordination.md`
- This plan/work order and paired requirements/design status/results.

## Dependencies

None. Existing admission package is implemented in this checkout.

## Risks

Enum validation precedes handler trimming and rejects formerly accepted blank
and padded values. Preserve this documented consequence; do not silently
add normalization. Existing large `handlers_test.go` files should not grow.

## Parallelism

`sequential`; no delegation authorized.

## Inputs

- [Requirements](../../specs/tasks/requirements/mcp-workspace-mode.md), REQ 004.
- [System design](../../specs/tasks/system-design/mcp-workspace-mode.md),
  materialization schema discoverability and compatibility sections.
- `server/tool_argument_validation.go`, `server/tool_argument_validation_test.go`,
  `server/handlers_test.go`, and `handlers/handlers.go`.

## Results

Completed 2026-09-13 after the explicit implementation request.

- Red: `go test ./internal/mcp/server -run TestCreateTaskWorkspaceMode -count=1`
  with the isolated cache failed on missing enum/description metadata and
  out-of-enum values reaching the recording backend.
- Green: both focused Go commands in Verification passed after the single
  production schema edit. Both enum values and omission dispatch correctly;
  blank/unsupported values fail at the enum boundary. Catalog and attachment
  serialization retain the enum and optionality. The direct handler matrix
  and existing parent-default/new-workspace tests passed unchanged.
- The additional catalog command passed, preserving config/Office exclusions
  and the fixed automation catalog.
- `node --test scripts/validate-public-docs.test.mjs`: passed.
- `node scripts/validate-public-docs.mjs`: passed, 46 pages.
- `python3 scripts/list-docs.py validate`: passed, 267 decisions and 868 specs.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `gofmt -l` on the three changed/new Go files: no output.
- `git diff --check`: passed.

Both public pages now explain exact values and omission instead of blank
strings. The handler production code is unchanged. Empty, whitespace-only,
and padded inputs now fail MCP validation as documented in the design.
No observed registration, validator, or attachment transform strips enum
metadata. No live-instance changes, delegation, commit, push, or PR merge.
The parent's only session is cancelled, so a completion message cannot be
delivered; completion evidence remains in these artifacts.
