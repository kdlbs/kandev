---
id: "01-align-discovery-contract"
title: "Align relation discovery with existing access rules"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RELATED-DISCOVERY-001
acceptance_criteria:
  - AC-TASKS-RELATED-DISCOVERY-001.1
  - AC-TASKS-RELATED-DISCOVERY-001.2
  - AC-TASKS-RELATED-DISCOVERY-001.3
  - AC-TASKS-RELATED-DISCOVERY-001.4
  - AC-TASKS-RELATED-DISCOVERY-001.5
system_design:
  - ../../specs/tasks/system-design/related-task-discovery.md
---

# Task 01: Align relation discovery with existing access rules

## Summary

Correct the tool description and public guidance for issue #4921.
Add focused coverage that preserves the existing relation boundary and distinguishes compact output from permission.

## In scope

- Tool description and `task_id` / `verbose` help in `registerRelatedTasksTool`.
- Registered-tool tests for contract text, caller identity, and verbose error propagation.
- The exact independent-root service regression.
- Public coordination and MCP wording.

## Out of scope

- Backend authorization or rich-projection changes.
- New permission grants, metadata tools, configuration, settings, or migrations.
- Document, write, conversation, or messaging permission changes.
- Unrelated source refactors and browser tests.

## Acceptance

1. The registered metadata satisfies `.001.1` and `.001.5` within the existing 500-character description limit.
2. Regression tests satisfy `.001.2` through `.001.4` without changing the service gate or output schema.
3. The three public pages describe the supported restriction and acknowledge that compact output does not grant unrelated-task access.

The abbreviated criteria use the prefix `AC-TASKS-RELATED-DISCOVERY`.

## Implementation and TDD

Read the requirement, design, source registration, nearby tests, and scoped backend guidance.
Mark this work order `in_progress` and synchronize the plan before implementation.

First add `TestListRelatedTasks_ToolDescriptionStatesAccessBoundary` in `handoff_handlers_test.go`.
Assert the permitted relation vocabulary, same-workspace restriction, unrelated-task denial, and argument-help semantics.
Avoid comparing the entire description as a fixed string.
Run it before changing registration text. It must fail because the relation gate is absent.

Add these compatibility controls in the same file:

- `TestListRelatedTasks_CallerBinding`: omitted `task_id`, `self`, and an explicit target retain `s.taskID` as caller.
- `TestListRelatedTasks_RejectsCallerOverride`: `caller_task_id` is absent from the schema, and an attempted override fails before dispatch.
- `TestListRelatedTasks_VerboseDoesNotBypassForbidden`: omitted, false, and true `verbose` preserve a backend forbidden error.
  Use `testBackend.err` and assert an error result without relation data.

Add `TestListRelatedForCaller_RootTasksAreUnrelated` in `handoff_related_test.go`.
Create two roots with empty parents in the same synthetic workspace.
Require `ErrAccessDenied` and a nil result. No state or description is required to trigger the rejection.
Retain the existing positive related-task controls and cross-workspace blocker controls.

Then correct the registration text and property help as described in the design.
Update the three public pages in this work order.
Preserve existing navigation, examples, provider-neutral fields, and messaging behavior.
Do not recommend manipulating parent or dependency edges to acquire permission.

## Verification

Run this block from the repository root. Each backend command owns its working directory.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -p 1 ./internal/mcp/server -run '^TestListRelatedTasks_ToolDescriptionStatesAccessBoundary$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -p 1 ./internal/mcp/server -run '^(TestListRelatedTasks_.*|TestCoreToolDescriptionsStayFocused)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -p 1 ./internal/task/service -run '^(TestListRelatedForCaller_.*|TestListRelated_.*|TestCanReadDocuments_.*)$' -count=1 -v)
bash scripts/lint-go-changed apps/backend/internal/mcp/server/handoff_handlers.go apps/backend/internal/mcp/server/handoff_handlers_test.go apps/backend/internal/task/service/handoff_related_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The first command supplies RED before the wording edit and GREEN afterward.
The other commands supply the final targeted implementation checks.
Run the package coverage preflight from the repository root:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/plans/related-task-access-contract/plan.md',
  'docs/plans/related-task-access-contract/task-01-align-discovery-contract.md',
  'docs/specs/tasks/requirements/related-task-discovery.md',
  'docs/specs/tasks/system-design/related-task-discovery.md',
];
const fileContents = Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')]));
const changedFiles = paths.map(filename => ({ filename, status: 'added' }));
changedFiles.push({ filename: 'apps/backend/internal/mcp/server/handoff_handlers.go', status: 'modified' });
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ status: result.status, errors: result.errors }, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

This preflight evaluates the planned source change and package references.
It does not claim that the runtime change exists or passes implementation checks.

Review public wording against the new description and `canReadDocuments`.
Search `README.md`, `docs/screenshots.md`, and public docs for conflicting same-workspace relation promises.
No browser check is required for this API discovery correction.

## Files likely touched

- `apps/backend/internal/mcp/server/handoff_handlers.go`
- `apps/backend/internal/mcp/server/handoff_handlers_test.go`
- `apps/backend/internal/task/service/handoff_related_test.go`
- `docs/public/coordination.md`
- `docs/public/agent-communication.md`
- `docs/public/automation-and-mcp.md`
- This work order and `plan.md`, for status and results.

These are the owned files. The task service implementation and document guards are reference inputs only.

## Dependencies

None. This is the only work order.

## Risks

- A guard bypass for compact mode changes permissions and must not enter this repair.
- The proposed delegated metadata capability remains unresolved after this work order.
- Cached client tool catalogs can retain the earlier wording until refresh.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/related-task-discovery.md), `REQ-TASKS-RELATED-DISCOVERY-001` and all five criteria.
- [System design](../../specs/tasks/system-design/related-task-discovery.md), all sections.
- [Plan](plan.md), investigation evidence and repair scope.
- Existing `handoff_handlers_test.go`, `tool_metadata_test.go`, `handoff_related_test.go`, and `handoff_access_test.go`.
- `handlers_test.go:TestArchiveTaskHandlerInjectsBoundCallerID`, for the existing caller-binding test pattern.

## Results

Implemented and verified on 2026-10-28.

### RED

`TestListRelatedTasks_ToolDescriptionStatesAccessBoundary` failed before the
registration text changed. The old description lacked the default-to-self
statement, `ancestor`, `descendant`, `sibling with a shared non-empty parent`,
and `Unrelated tasks are denied`. The old `task_id` help lacked the
same-workspace relation rule, and the old `verbose` help did not state that full
descriptions never widen access.

The compatibility controls passed before the change, which confirms they pin
existing behavior rather than require new authorization code:
`TestListRelatedTasks_CallerBinding`, `TestListRelatedTasks_RejectsCallerOverride`,
`TestListRelatedTasks_VerboseDoesNotBypassForbidden`, and
`TestListRelatedForCaller_RootTasksAreUnrelated`.

### GREEN

The registered tool now advertises the relation gate in the main description
(388 runes, under the 500-rune focus limit) and in the `task_id` and `verbose`
help. `ListRelatedForCaller` and `canReadDocuments` are unchanged, and no
frontend, schema, migration, flag, setting, metric, or browser test was added.

Public guidance was updated on all three pages: the coordination paragraph,
**Discovering task IDs** in agent communication, and the relation-tool
reference near change-request fields. Each page states the permitted relations,
same-workspace denial for unrelated targets, and that compact output is a
presentation option rather than a separate metadata permission.

### Verification

All commands ran from the repository root, with the backend commands using
`GOMAXPROCS=2 GOMEMLIMIT=512MiB`:

```bash
go test -trimpath -tags fts5 -p 1 ./internal/mcp/server -run '^(TestListRelatedTasks_.*|TestCoreToolDescriptionsStayFocused)$' -count=1 -v
go test -trimpath -tags fts5 -p 1 ./internal/task/service -run '^(TestListRelatedForCaller_.*|TestListRelated_.*|TestCanReadDocuments_.*)$' -count=1 -v
```

Both commands passed. Additional results:

- Full packages `./internal/mcp/server` and `./internal/task/service`: PASS.
- `go build -trimpath ./...` from `apps/backend`: PASS, no output.
- `bash scripts/lint-go-changed` on the three Go files: `0 issues.`
- `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed.
- `node scripts/validate-public-docs.mjs`: 54 published pages validated.
- `python3 scripts/list-docs.py validate`: 415 decisions and 1,826 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Package coverage preflight: `covered`, no reference errors.
- `git diff --check`: clean.

The temporary reproduction files from the investigation remain removed. No
browser check ran because this repair changes no rendered product UI.
