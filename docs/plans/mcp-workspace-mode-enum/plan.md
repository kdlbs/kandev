---
created: 2026-09-13
status: implemented
requirements:
  - REQ-TASKS-MCP-WORKSPACE-MODE-004
system_design:
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
legacy_specs: []
---

# Implementation plan: MCP workspace mode enum

## Overview

Expose the accepted materialization choices in `create_task_kandev` discovery.
One sequential work order delivers schema metadata, regression evidence, and
public caller guidance. This is a follow-up to the completed
[creation-admission package](../mcp-workspace-mode/plan.md); its earlier work
orders and verification results remain historical and are not reopened.

## Scope

In scope: the optional enum, conditional-default description, task/external
registration and propagation checks, input compatibility evidence, and docs.

Out of scope: new workspace modes, handler normalization changes, execution
behavior, Office admission, custom prompts, UI, dependency changes, running
instance changes, delegation, and PR merge.

## Technical approach

Use `mcp.Enum` in `registerCreateTaskTool` alongside `mcp.WithString` and
`mcp.Description`. Leave required/default metadata absent. Preserve
`resolveMCPWorkspacePolicy`. See the
[requirements](../../specs/tasks/requirements/mcp-workspace-mode.md) and
[design's materialization section](../../specs/tasks/system-design/mcp-workspace-mode.md#materialization-schema-discoverability).

The task system owns the task-creation input contract. The requested values and
optional/default semantics are confirmed by the task brief and local source.
No material design question remains. A new ADR is unnecessary: the existing
schema-validation ADR already defines enforcement, and the paired design
records this small field-level compatibility consequence.

## Tests

- AC 004.1/004.2: `TestCreateTaskWorkspaceModeSchema` in
  `server/create_task_workspace_mode_test.go`, table-driven over task/external
  modes, checks serialized catalog schema and `mcpToolInputSchema` output.
- AC 004.3/004.4: `TestCreateTaskWorkspaceModeArguments` drives the
  existing `callTool` harness through schema validation into a recording
  backend; failures must leave dispatch untouched.
- AC 004.3: existing handler tests
  `TestHandleCreateTask_SubtaskDefaultsToParentWorkspaceAndWorkflow` and
  `TestHandleCreateTask_SubtaskCanRequestNewWorkspaceMode`; proposed
  `TestResolveMCPWorkspacePolicyCompatibility` in a dedicated handler test file
  covers unchanged direct policy semantics.

AC suffixes refer to `AC-TASKS-MCP-WORKSPACE-MODE-`.
Go protocol/dispatch coverage is the end-to-end evidence for this MCP-only
change. No browser flow changes.

## Documentation

Updated `docs/public/automation-and-mcp.md` (reference) and
`docs/public/coordination.md` (coordination guide) to name exact choices, the
parent requirement, materialization semantics, and omission instead of blanks.
Searches found no
corresponding field guidance in the root README or screenshot catalog.

## Work orders

- [x] [01: Expose workspace mode enum](task-01-expose-workspace-mode-enum.md)

No dependencies on parent checkout changes. Completed sequentially after the
user's explicit implementation request.

## Verification results

Completed on 2026-09-13. The schema and dispatch regressions failed before
the schema edit, then passed afterward. The direct handler compatibility
matrix and existing subtask default/new-workspace tests passed. Config/Office
exclusion and fixed automation catalog tests passed. Public-doc tests and
validation (46 pages), catalog validation (267 decisions/868 specs),
specification tests (36), specification lint, gofmt, and diff checks passed.

All exact commands and results are recorded in the
[completed work order](task-01-expose-workspace-mode-enum.md#results).
Go verification used `GOCACHE=/tmp/kandev-workspace-enum-go-cache` because the
default cache is read-only in this workspace. Production changes are limited
to the optional enum and its description; backend validation/defaulting remain
unchanged. Requirements are active and the paired design is current.

Files are uncommitted. The parent's only session remains cancelled, preventing
delivery of the completion message.

## Risks

- Empty strings, whitespace-only strings, and padded values previously reached
  the trimming handler but now fail MCP enum validation. Callers must omit
  the key for defaulting; this narrowing must be explicit in delivery notes.
- A schema default would incorrectly imply inheritance for top-level tasks.
- Testing only description text misses missing enum metadata and optionality.
- Attachment size limits may omit whole schemas; no inspected transform strips
  enum selectively. External client behavior is outside the local guarantee.
