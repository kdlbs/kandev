---
created: 2026-10-26
status: done
requirements:
  - REQ-EXECUTORS-TASK-DISCOVERY-001
system_design:
  - ../../specs/executors/system-design/task-mcp-discovery.md
legacy_specs: []
---

# Implementation plan: Task MCP executor discovery

## Overview

Expose the existing executor catalogue tool to task-mode agents. This fixes
[issue #4708](https://github.com/kdlbs/kandev/issues/4708) with one sequential
work order. The issue is assigned to `carlosflorencio`.

## Confirmed cause and reproduction

`registerKanbanTools` registers only `list_executor_profiles_kandev`, whose
required executor ID prevents discovery of unknown custom IDs. Its description
lists built-in examples. `registerConfigExecutorTools` registers the complete
catalogue operation, but that group is excluded from the Kanban task surface.
The task-mode membership test explicitly asserts absence of the needed tool.

Read-only source tracing confirms the gap and the existing response path.
Focused baseline tests for task registration, automation registration, and
executor forwarding passed on 2026-10-26. No production or permanent test
changes were made during planning.

## Scope

### In scope

Task-mode read registration, tool guidance, compatibility tests, scoped catalogue
integration tests, and public reference documentation.

### Out of scope

Executor mutations, UI changes, new backend actions, permissions, migrations,
and removal or overloading of `list_executor_profiles_kandev`.

## Technical approach

Use the shared read registration described in the
[design](../../specs/executors/system-design/task-mcp-discovery.md).
Preserve `listExecutorsHandler`, `handleListExecutors`, service ownership filters,
and trusted caller context. Add no broad configuration capability to task mode.

Reuse beats consolidation here: the existing catalogue already contains profiles,
while the existing per-executor tool remains useful and compatible. No new
operation or response shape is necessary.

Compatibility: task mode gains one read; configuration/external and automation
retain their catalogues; Office/coordinator/managed conversation remain unchanged.
The handler works across executor types using existing DTOs. No provider runtime
launch is needed to prove discovery, and unavailable providers gain no new fallback.

## Tests

- AC .1/.4/.5: update registration and tool-count assertions in `server_test.go`;
  preserve all mutation exclusions and other surface membership tests.
- AC .1/.2/.3: new `TestTaskExecutorDiscovery_CustomProfileSelection` and
  `TestTaskExecutorDiscovery_Ownership` in
  `apps/backend/internal/mcp/server/task_executor_discovery_integration_test.go`.
- AC .6: new `TestTaskExecutorDiscovery_EmptyAndErrors` in that integration file.
- Retain service ownership coverage in `executor_ownership_test.go`, including
  independent admission checks and MCP actor behavior.

All abbreviated acceptance IDs above refer to
`AC-EXECUTORS-TASK-DISCOVERY-001`.

## End-to-end evidence

Use a Go protocol integration flow: task-profile MCP call -> scoped dispatcher ->
real task service -> custom executor/profile discovery -> `create_task_kandev`
with `start_agent: false`. Assert the selected executor profile on the created
record. Revoke eligibility before a second creation and assert rejection.
No browser flow changes, so no Playwright test is required.

## Work orders

- [x] [Task 01: Expose and verify executor discovery](task-01-executor-discovery.md) (completed)

## Verification results

Baseline passed:
`(cd apps/backend && go test ./internal/mcp/server -run 'TestServerModeTask_RegistersCorrectTools|TestServerSurfaceAutomationHasFixedCoordinatorCatalog|TestListExecutorsHandler_Success' -count=1)`.

Design validation passed on 2026-10-26:

- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: covered, using the four draft
  artifacts and the planned `server.go` change as preflight input.
- `git diff --check`: passed. `git status --short` confirms four new package
  files, all unstaged and uncommitted.

Implementation verification passed on 2026-10-26:

- Focused task executor discovery integration tests, the full MCP server
  package, task service executor ownership tests, and the focused existing
  executor-list handler dispatch test passed. The task-creation handler group
  passed after the service ownership guard was scoped to explicit caller
  selections; inherited profile behavior remains covered.
- `(cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers -count=1 -timeout=30m)`: passed; server 23.632s, handlers 957.328s.
- `(cd apps/backend && go test ./internal/task/service -run '^TestExecutorOwnership' -count=1)`: passed.
- The first combined run reached Go's default 10-minute timeout. An initial
  extended run exposed inherited-profile compatibility failures; after
  narrowing the guard and correcting the explicit-profile fixture, the complete
  server/handler run passed.
- Changed-Go lint reported zero issues. `make -C apps/backend build` passed.
- Public-doc tests and validation, docs catalogue validation, specification
  lint tests, all-spec lint, and the actual-path PR documentation coverage
  preflight passed.
- PR fixup after main advanced: the merged handler preserves task-priority
  updates and explicit executor-profile validation. The changed Go packages,
  changed-file lint, and full backend build passed. CI's only remaining leaf
  failure was one retried mobile breadcrumb-layout assertion; a final-directory
  readiness wait now stabilizes the measurement, and its focused mobile test
  passed four repetitions with retries disabled on the merged tree. Fresh PR
  checks for the pushed merge commit remain required.
- `gofmt` reported no changed Go files; `git diff --check` passed.

## Risks

Shared registration also runs during automation catalogue construction. Preserve
its exact membership and avoid duplicate semantic changes. A mocked forwarding
test alone cannot prove identity filtering; use the real scoped dispatcher.
Public documentation ships with implementation, not with this draft package.
