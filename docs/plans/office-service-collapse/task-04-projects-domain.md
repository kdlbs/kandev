---
id: "04-projects-domain"
title: "Delete the facade's projects mirror"
status: pending
wave: 2
depends_on: ["01-duplication-detector"]
plan: "plan.md"
spec: none
parallel-safe: true
---

# Task 04: Projects Domain

A leaf domain: `office/projects` (367 LOC) owns the routes
(`office/routes.go:45-46`) and the `office/runtime` `Projects` action interface
resolves to `svcs.Projects` (`office/routes.go:32`). Nothing calls the facade's
project methods.

## Scope

Delete from `internal/office/service/service.go` (roughly lines 520–620):
`CreateProject` `UpdateProject` `DeleteProject` `validateProject`
`validateRepositories`, and `GetProjectFromConfig` /
`ListProjectsFromConfig` / `ListProjectsWithCountsFromConfig` from
`config_read.go` if task 02 left them.

Four are byte-identical (`DeleteProject` `UpdateProject` `GetProjectFromConfig`
`validateRepositories`). The two that read as drifted are **not**:

- `CreateProject` (0.985) and `validateProject` (0.974) differ only because
  `projects/models.go:7` declares `type Project = models.Project` and
  `projects/models.go:15-18,28` alias the status constants. `models.Project` and
  `Project` are the *same type*; `models.ValidProjectStatuses` and
  `ValidProjectStatuses` are the *same variable*. Confirm those alias
  declarations still exist before relying on this.

## Test migration

`office/projects` has **zero test files**. `service/service_project_test.go`
(144 LOC) must move to `projects/service_test.go`, adapting the receiver to
`*ProjectService`. Because of the type aliases, the assertions themselves should
need no change — if one does, that is a real difference the inventory missed and
should be reported.

## Acceptance

1. Detector Section A drops by **4** groups; Section B same-name pairs drop by 2.
2. `office/projects` has non-zero test coverage.
3. No change to `/api/.../projects` request or response shapes.

## Verification

```bash
cd apps/backend && go test ./internal/office/projects/... -count=1 -v
cd apps/backend && go test ./internal/office/... -count=1
make -C apps/backend test
make -C apps/backend lint
cd apps/backend && golangci-lint run ./... --new-from-rev=main --timeout=5m
```

## Files likely touched

- `internal/office/service/service.go` (delete the project block)
- `internal/office/service/config_read.go` (project readers, if still present)
- deleted: `internal/office/service/service_project_test.go`
- new: `internal/office/projects/service_test.go`

## Dependencies

Task 01.

## Parallelism

`parallel-safe` with tasks 02 and 06. All three touch
`internal/office/service/service.go`, so if run concurrently they **will**
conflict there — run them sequentially or accept a trivial rebase. Files are
otherwise disjoint.

## Rollback position

Single revert; restores dead code only.

## Output contract

Summary, files changed, detector delta, and confirmation that the type aliases in
`projects/models.go` still hold (or the list of assertions that had to change).

## Results

Pending.
