---
id: "02-creation-preparation-and-bootstrap"
title: "Creation preparation and bootstrap"
status: complete
wave: 2
depends_on: ["01-destination-model-and-github-resolution"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/improve-kandev.md"
---

# Task 02: Creation Preparation and Bootstrap

## Acceptance

- Improve Kandev implementation task creation under managed task access resolves direct write or an exact
  fork destination after deduplication and validation but before inserting the task or launching a session.
- The binding travels only through a server-owned, JSON-excluded service field and is persisted on the
  canonical task-repository attachment; ordinary and issue-only tasks remain unchanged.
- Managed bootstrap uses the same workspace automation resolver and reports direct, ready, creatable, or
  blocked capability for the identity that will supply managed task credentials, including the canonical
  provider repository ID. Executor-owned access retains a separate capability path and receives no
  server-authored binding. Blocked responses expose a stable reason code for localized clients.

## Verification

```bash
cd apps/backend
rtk go test ./internal/task/service ./internal/improvekandev ./internal/backendapp -run 'Test.*(ContributionDestination|ImproveKandev.*Fork|ForkCapability|CreateTask.*Contribution)'
```

## Files likely touched

- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/service_requests.go`
- `apps/backend/internal/task/service/service_tasks.go`
- focused task creation and metadata tests
- `apps/backend/internal/improvekandev/github.go`
- `apps/backend/internal/improvekandev/handler.go`
- `apps/backend/internal/improvekandev/handler_test.go`
- `apps/backend/internal/backendapp/helpers.go`
- focused backend wiring tests

## Dependencies

Task 01's validated binding and workspace GitHub resolver.

## Parallelism

Sequential. Creation ordering is the authority boundary for every later runtime lease.

## Inputs

- Spec: managed preparation before first launch, bootstrap identity alignment, executor-owned separation,
  and issue-only bypass.
- ADR: no caller-authored destination and no partly launched task on provider failure.
- Existing patterns: external-ID create settlement, `RemoteContribution`'s internal service field, hidden
  workflow template IDs, and Improve Kandev bootstrap connection copying.

## Risks

- Run provider preparation after external-ID lookup so retries cannot create or poll the fork twice.
- Match the immutable workflow template ID, not a mutable workflow name.
- Do not make generic task creation depend on GitHub when the optional resolver is absent or the workflow is
  unrelated.
- Resolve task Git policy before enrichment and never infer an executor identity from workspace automation.
- Preserve all required create compensation/settlement behavior and attachment ordering.

## TDD sequence

1. Add failing ordering, deduplication, enrichment-scope, persistence, and bootstrap actor tests.
2. Add the optional task service seam and server-only input field.
3. Wire the Improve resolver and replace ambient-host bootstrap probing.
4. Refactor duplicated capability mapping, then rerun focused suites.

## Output contract

Report create ordering, trusted/untrusted boundaries, bootstrap response changes, files changed, red/green
commands, remaining risks, divergence, and task/plan status updates.

## Completion

Completed 2026-08-13. Task creation now prepares the destination after deduplication and repository/workflow
validation but before insertion, carries it through a JSON-excluded service field, and persists it only as
server-authored task-repository metadata. The Improve Kandev adapter is limited to the immutable template
and managed credential mode. Bootstrap uses the same workspace-scoped capability resolver, while
executor-owned access remains explicitly separate and managed failures cannot fall back to ambient `gh`.
Bootstrap also records the canonical provider ID and returns stable fork reason codes, which the UI
localizes instead of rendering backend prose.

Verification: `rtk go test ./internal/task/service ./internal/improvekandev ./internal/backendapp -count=1`
passed, including ordering, persistence, failure, capability, and wiring cases.
