---
id: "01-resolve-project-sources"
title: "Resolve project repository defaults"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PROJECT-REPOSITORIES-001
acceptance_criteria:
  - AC-TASKS-PROJECT-REPOSITORIES-001.1
  - AC-TASKS-PROJECT-REPOSITORIES-001.2
  - AC-TASKS-PROJECT-REPOSITORIES-001.3
  - AC-TASKS-PROJECT-REPOSITORIES-001.4
  - AC-TASKS-PROJECT-REPOSITORIES-001.5
  - AC-TASKS-PROJECT-REPOSITORIES-001.6
  - AC-TASKS-PROJECT-REPOSITORIES-001.7
system_design:
  - ../../specs/tasks/system-design/project-repository-context.md
---

# Task 01: Resolve project repository defaults

## Summary

Add root project defaults to the shared task creation boundary.
Persist normal task attachments before the creation event can trigger Office work.

## In scope

- Neutral reader interface, Office read adapter, and production wiring.
- Nil-versus-empty root selection, explicit workspace preservation, and parent precedence.
- Validated path/URL conversion, ordered source resolution, and deduplication by resolved repository identity.
- Focused service and composition regressions.
- Existing fixtures with fake projects or absent reader wiring.

## Out of scope

- Launch implementation, browser markup, project mutation, existing-task repair, and new providers.

## Acceptance

- Write `TestCreateTaskRootProjectRepositories` first and observe its zero-attachment failure. Use a real project and local Git fixture.
- Cover every listed criterion with the test matrix in the plan. Mixed trusted and untrusted origins must fail in either order without a task row or `task.created` event.
- A local path and its matching remote URL must attach one registered repository in either order; explicit multi-branch duplicate rules remain unchanged.
- A production wiring test must reach the shared task service through the real composition seam. A manually installed test adapter alone is insufficient.

## Verification

Run the focused regression before implementation. After the correction, run the
affected package tests and specification gates from the repository root.

```bash
(cd apps/backend && go test -tags fts5 -run '^TestCreateTaskRootProjectRepositories$' ./internal/task/service/ -count=1 -v)
(cd apps/backend && go test -tags fts5 -race ./internal/task/service ./internal/backendapp)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/project_repository_sources.go` (new)
- `apps/backend/internal/task/service/service_project_repositories_test.go` (new)
- `apps/backend/internal/task/service/service_office_test.go`
- `apps/backend/internal/task/service/service_office_integration_test.go`
- `apps/backend/internal/backendapp/adapters_office.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/office_project_repositories_test.go` (new)
- `apps/backend/internal/backendapp/adapters_office_test.go`
- Related existing test harnesses requiring the newly mandatory reader.

## Dependencies

None.

## Risks

Keep the reader's types neutral. Refuse foreign-workspace projects before resolver
side effects. Retain existing task-create idempotency and explicit multi-branch rules.
Do not suppress missing-reader errors to preserve old test fixtures.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/project-repository-context.md)
- [Design](../../specs/tasks/system-design/project-repository-context.md)
- `setupOfficeTest` and `createOfficeIntegrationServiceWithDB` test harnesses.
- Existing `inheritParentRepositories`, `resolveTaskCreationReferences`, and local/remote resolvers.

## Results

Completed. Root tasks with an omitted repository selection resolve every selected Office project source through the shared task service before deduplicating by the resolved repository ID. Explicit selections, workspace paths, shared-group tasks, and subtasks retain their existing precedence. Mixed GitLab HTTPS/HTTP source orders fail before task creation and publication. Local-path/remote-URL aliases attach one registered repository in either order. Supported scheme-less host/path remotes such as `github.com/acme/api` use the existing provider parser and attach normally. Foreign workspace IDs are omitted from the public creation error. The Office adapter performs an exact project lookup and production task-service wiring is covered. The HTTP and WebSocket create paths preserve omitted versus explicitly empty repository lists.

Validation passed:

- `go test -trimpath -tags fts5 -race ./internal/task/service ./internal/backendapp ./internal/orchestrator -count=1`
- `go test -trimpath -tags fts5 ./internal/task/handlers -count=1`
- `go test -trimpath -tags fts5 -race ./internal/task/service -count=1` (after PR review remediation)
- `go test -trimpath -tags fts5 -race ./internal/mcp/handlers -count=1` (after making project-backed legacy fixtures explicit about their empty repository selection)
- `make -C apps/backend build` (after PR review remediation)
- `golangci-lint run ./... --new-from-rev=330e02a47808c11ca315ae30456fcce7f4806db5 --timeout=5m` (0 issues)
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
