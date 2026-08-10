---
id: "01-gitlab-repo-content-client"
title: "GitLab client repository-content methods"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/gitlab-workflow-sync/spec.md"
---

# Task 01: GitLab Client Repository-Content Methods

## Acceptance

1. `gitlab.Client` exposes `ListRepoTree(ctx, projectPath, path, ref string) ([]RepoTreeEntry, error)`
   and `GetRepoFileContent(ctx, projectPath, path, ref string) ([]byte, error)`,
   implemented by `PATClient` (REST), `MockClient` (seedable), and
   `NoopClient` (not-configured error). `GLabClient` inherits via its embedded
   `*PATClient` and needs no change.
2. `PATClient.ListRepoTree` targets `GET /projects/:id/repository/tree` with a
   non-recursive listing, percent-encodes the full `projectPath` as one path
   segment (so `group/subgroup/project` resolves), and follows pagination until
   exhausted. `GetRepoFileContent` targets
   `GET /projects/:id/repository/files/:file_path/raw?ref=<ref>` with the file
   path percent-encoded.
3. `RepoTreeEntry` carries at least `Name`, `Path`, and `Type` (`"blob"` /
   `"tree"`), and 404/403 responses surface as errors distinguishable enough for
   callers to wrap with context.

## Verification

```bash
cd apps/backend && go test ./internal/gitlab/... -race
```

## Files Likely Touched

- `apps/backend/internal/gitlab/models.go` — `RepoTreeEntry`.
- `apps/backend/internal/gitlab/client.go` — interface methods.
- `apps/backend/internal/gitlab/pat_client.go` — REST implementation.
- `apps/backend/internal/gitlab/mock_client.go` — in-memory implementation.
- `apps/backend/internal/gitlab/noop_client.go` — stubs.
- `apps/backend/internal/gitlab/pat_client_test.go` (or a new
  `pat_client_repo_contents_test.go`).

## Inputs

- Spec `## API Surface` → `### gitlab.Client`.
- `ListProjectBranches` at `pat_client.go:531` is the closest existing REST
  method — mirror its endpoint construction, `projectRef()` helper usage,
  pagination, and error handling.
- All `Client` methods take `projectPath` (never owner+repo); match that.

## Risks

- Nested subgroup paths silently 404 if `projectRef()` does not encode the
  full path as a single segment. Cover with an explicit multi-segment test.
- A directory larger than one page truncates the synced workflow set. Paginate
  and assert it in a test.

## Output Contract

Four GitLab client implementations compile against the extended interface, with
tests covering nested paths, pagination, and the not-found path. No consumer
outside `internal/gitlab` changes in this task.
