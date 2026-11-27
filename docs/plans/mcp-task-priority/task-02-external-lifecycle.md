---
id: "02-external-lifecycle"
title: "Complete external MCP priority lifecycle"
status: done
wave: 2
depends_on:
  - "01-priority-fields"
plan: "plan.md"
requirements:
  - REQ-TASKS-MCP-PRIORITY-001
acceptance_criteria:
  - AC-TASKS-MCP-PRIORITY-001.1
  - AC-TASKS-MCP-PRIORITY-001.2
  - AC-TASKS-MCP-PRIORITY-001.3
  - AC-TASKS-MCP-PRIORITY-001.4
  - AC-TASKS-MCP-PRIORITY-001.6
  - AC-TASKS-MCP-PRIORITY-001.7
  - AC-TASKS-MCP-PRIORITY-001.8
  - AC-TASKS-MCP-PRIORITY-001.9
system_design:
  - ../../specs/tasks/system-design/mcp-task-priority.md
---

# Task 02: Complete external MCP priority lifecycle

## Summary

Expose the existing generic update tool on external MCP and prove the complete
priority lifecycle over its HTTP transport. Document the supported calls and
retain configuration, Office, and automation authority boundaries.

## In scope

- Extract `registerUpdateTaskTool` from the Kanban registration and call it
  there plus an external-only `profileToolGroups` entry. Keep one schema.
- Add `TestMCPTaskPriorityCatalog` and update external catalog expectations.
  Retain the configuration assertion that generic create/update are absent.
- Add `TestMCPTaskPriorityAutomationScope` through the guarded dispatcher.
  Include a permitted target, a foreign workspace, and the denied self-target.
- Add `task_priority_integration_test.go` using the existing HTTP test helpers,
  real MCP handlers, real task service, temporary SQLite, and event bus.
- `TestExternalMCPTaskPriorityJourney` creates a high-priority task, lists it,
  updates priority without state, and lists the new value. Cover default
  creation, every enum token, and an external-ID retry after reprioritization.
- Verify the response, stored task, and task-created/updated event priority.
  Preserve task fields and a seeded waiting session during priority-only update.
- `TestExternalMCPTaskPriorityAuthorization` threads authenticated identity
  through production transport/dispatch context. Allow an owned task and deny a
  foreign user's task without mutation. Include authorized Kanban and Office
  targets using the four canonical priority tokens, not Office-only `none`.
- `TestExternalMCPViewerCannotChangeDeferredLaunchPrompt` verifies that an
  authorized owner can refresh a pending prompt and a workspace viewer cannot
  replace it through the external update tool. The task service also checks
  executor authority before rewriting a prompt, with a regression for a task
  writer targeting another user's private executor.
- Document default creation, priority-only update, invalid values, and retry
  semantics in the public MCP reference and coordination guide.
- Replace the visibility requirement's factual note that MCP cannot write
  priority with a link to the new contract. Keep its UI scope unchanged.
- After all checks pass, synchronize results and promote the paired draft
  requirement/design to active/current. Mark this plan implemented.

## Out of scope

- New permission maps, raw WebSocket access, or new backend action names.
- Office-session tools, CLI priority support, generic configuration updates,
  rendered UI, or scheduling changes.

## Acceptance

- External clients discover and call the same generic update schema as task
  clients. Configuration, Office, coordinator, and managed conversations gain
  no generic update tool.
- The real external HTTP journey proves stored and listed priority, event
  propagation, omission, retries, and unchanged task/session fields.
- Existing authorization denies foreign and automation self-target writes.
  Documentation and catalog assertions match the final supported surface.

## Verification

Run from the repository root. Add and run the external regression before
registration changes, then run this complete block after implementation.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/mcp/server ./internal/mcp/handlers -run 'Test(MCPTaskPriority|ExternalMCPTaskPriority)' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -run '^TestUpdateDeferredLaunchPromptRejectsForeignExecutor$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/mcp/server ./internal/mcp/handlers -count=1)
bash scripts/lint-go-changed apps/backend/internal/mcp/server/server.go apps/backend/internal/mcp/server/server_test.go apps/backend/internal/mcp/server/task_priority_test.go apps/backend/internal/mcp/server/task_priority_integration_test.go apps/backend/internal/mcp/handlers/handlers.go apps/backend/internal/mcp/handlers/deferred_launch_prompt_test.go apps/backend/internal/mcp/handlers/task_priority_test.go apps/backend/internal/task/service/service_deferred_launch.go apps/backend/internal/task/service/service_deferred_launch_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/mcp-task-priority docs/specs/tasks docs/specs/integrations
```

Run the pure PR-documentation preflight from the repository root:

```bash
node <<'NODE'
const fs = require('node:fs');
const {execFileSync} = require('node:child_process');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const artifacts = [
  'docs/plans/mcp-task-priority/plan.md',
  'docs/plans/mcp-task-priority/task-01-priority-fields.md',
  'docs/plans/mcp-task-priority/task-02-external-lifecycle.md',
  'docs/specs/tasks/requirements/mcp-task-priority.md',
  'docs/specs/tasks/system-design/mcp-task-priority.md',
];
const tracked = execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], {encoding:'utf8'});
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], {encoding:'utf8'});
const paths = [...new Set((tracked + untracked).split('\0').filter(Boolean))];
const changedFiles = paths.map(filename => ({filename, status:'modified'}));
const fileContents = Object.fromEntries(artifacts.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({changedFiles, fileContents});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

This local command does not publish a GitHub status. Run it before committing
so the actual implementation paths and both work orders remain in the diff.

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/mcp/server/task_priority_test.go`
- `apps/backend/internal/mcp/server/task_priority_integration_test.go` (new)
- `apps/backend/internal/mcp/handlers/task_priority_test.go`
- `docs/public/automation-and-mcp.md`
- `docs/public/coordination.md`
- `docs/specs/tasks/requirements/task-priority-visibility.md`
- The paired MCP priority requirement/design and this delivery package.

## Dependencies

Task 01. Both work orders share registration and test files.

## Risks

- A test using a recording backend alone cannot prove persistence or auth.
- External generic update exposes its existing metadata fields too. Keep
  service authorization intact and verify the real request context.
- Re-registering the same tool within a profile can hide catalog mistakes.
- Office-only `none` is outside the canonical task priority enum.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/mcp-task-priority.md)
- [Design](../../specs/tasks/system-design/mcp-task-priority.md), especially
  Tool contracts and Compatibility and authorization.
- Existing `external_integration_test.go` HTTP helpers and
  `create_task_self_placement_integration_test.go` service composition.
- Existing automation authorization and task-scope fixtures.

## Results

- The external catalog regression failed before registration changes because
  `update_task_kandev` was absent. It passes with the shared registration
  extracted and enabled for external MCP only.
- Catalog tests pass for task, automation, external, configuration, Office,
  coordinator, and managed-conversation surfaces. Configuration retains its
  state-only update tool.
- Guarded automation tests pass for an authorized target and deny foreign and
  self-target writes without changing priority.
- The real HTTP/SQLite journey passes for all four create and update tokens,
  medium defaults, update omission, task fields and waiting-session retention,
  task listing, events, and external-ID retries.
- HTTP authorization tests pass for owned Kanban and Office tasks and deny a
  foreign user's task without mutation.
- Deferred-prompt writes require task-write scope and executor authority.
  External MCP denies a viewer before changing the queued prompt, and the task
  service denies a task writer who cannot access the configured private
  executor. Handler tests reject invalid task state and overlong title before
  changing the queued prompt.
- Follow-up MCP handler, external MCP/SQLite, and task-service deferred-prompt
  focused tests passed. `make -C apps/backend build` and changed-file Go lint
  passed.
- A non-race full handler package run timed out at ten minutes while SQLite
  initialized a repository in `TestHandleMessageTask_CancelledPrimaryFallsBackToTheLiveSession`.
  Focused changed-handler tests passed; the earlier full race-suite timeout
  remains recorded above.
- The full server package test, focused MCP tests, and focused MCP race tests
  pass. The full server/handler race command exposed stale catalog assertions,
  which were fixed, then the handler package reached its 10-minute timeout
  during existing SQLite schema initialization in
  `TestHandleCreateTask_SubtaskBaseBranchOverride_ExplicitReposWin`. No race
  report occurred; focused priority race tests pass in both packages.
- Public docs validation, spec catalog validation, all 36 spec-linter tests,
  full spec lint, backend build, changed-file Go lint, and `git diff --check`
  pass.
- PR documentation coverage preflight: `covered`, with both work orders and
  their requirement/design references accepted.
