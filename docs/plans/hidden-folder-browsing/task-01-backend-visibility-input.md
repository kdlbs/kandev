---
id: "01-backend-visibility-input"
title: "Backend visibility input for hidden directories"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-HIDDEN-FOLDERS-001
acceptance_criteria:
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.1
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.2
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.3
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.6
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.7
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.11
system_design:
  - ../../specs/workspaces/system-design/hidden-folder-browsing.md
---

# Task 01: Backend visibility input for hidden directories

## Summary

Turn the hard-coded hidden-entry exclusion into a caller-supplied visibility
decision. `GET /api/v1/fs/list-dir` accepts `include_hidden`. Directory creation
keeps its existing request and response contract. The default listing stays
unchanged.

## Scope

- `ListDirectory` takes the flag explicitly and passes it to `collectSubdirs`.
- The list-dir handler reads `include_hidden` with a strict `true` comparison and
  no error path.
- Go tests for both flag values, the default, the directory-only rule under the
  reveal, revealed sort position, handler parsing, and an unlistable path
  answering identically with and without the reveal.

## Exclusions

- No new response field, no pagination, and no cap.
- No listing of symbolic-link directories and no name-validation change.
- No `os.Root`, path-resolution, parent-computation, or logging change.
- No discovery, repository-validation, or scan change.
- No visibility input on `create-dir`. Implementation testing showed the endpoint
  returns a listing of the folder it just created, that folder is always empty,
  and the browser re-lists it through `list-dir`, so such a field could never
  change an observable result. The requirement and system design were corrected
  to match.

## Implementation acceptance conditions

1. A request without `include_hidden`, or with any value other than `true`,
   returns a listing with no hidden entry, matching the current contract.
2. A request with `include_hidden=true` returns hidden directories in the same
   listing. The existing comparator can place punctuation-prefixed names before
   dot-prefixed names. Ordinary entries keep their relative order, and files
   remain excluded.
3. A dot-prefixed folder created through `create-dir` is accepted, is enterable,
   and appears in its parent's listing when the reveal is active.

## Verification commands

```bash
cd apps/backend
gofmt -l internal/task/service internal/task/handlers
go test ./internal/task/service/ -run 'TestListDirectory|TestCreateDirectory' -count=1
go test ./internal/task/handlers/ -run 'TestHTTP(ListDirectoryHiddenEntryVisibilityFollowsRequestValue|ListDirectoryFailureIsIdenticalWithAndWithoutTheReveal|CreateDirectoryDotPrefixedChildIsEnterableAndRevealable)' -count=1
make lint
```

## Likely files

- `apps/backend/internal/task/service/directory_listing.go`
- `apps/backend/internal/task/service/directory_listing_test.go`
- `apps/backend/internal/task/service/directory_listing_windows_test.go`
- `apps/backend/internal/task/handlers/repository_handlers.go`
- `apps/backend/internal/task/handlers/repository_handlers_test.go`
- `apps/backend/internal/task/handlers/repository_handlers_hidden_folders_test.go`

## Dependencies and risks

- The existing test that asserted a hidden directory is excluded was kept as the
  default-case assertion rather than deleted.
- `ListDirectory` has exactly two production call sites, the list-dir handler
  and `CreateDirectory`. `CreateDirectory` passes `false` because it lists a
  freshly created, always-empty folder.
- `golangci-lint` reported `nestif` on the first table-test draft; the assertion
  was extracted into `entryNames` rather than suppressed.

## Results

Done.

Code review added the missing coverage for AC-WORKSPACES-HIDDEN-FOLDERS-001.11:
`TestHTTPListDirectoryFailureIsIdenticalWithAndWithoutTheReveal` requests a path
that is a file and a path that does not exist, with and without the reveal, and
asserts the status and body are byte-identical and that the body never echoes the
requested host path. This is test-only contract coverage of the existing error
branch, which the flag cannot reach; no production code changed for it. The review
also removed a fixture name that a generically named test helper had absorbed, and
reused the shared `listDirEntry` type in the create-directory case.

RED was observed at the HTTP boundary before any production change:
`exact_true_reveals` failed with `entries = [alpha gamma], want [.hidden-dir
alpha gamma]`, and the create-dir case failed with `entries: []`. The four
default-contract cases passed, documenting the unchanged behavior.

Changed files: `directory_listing.go`, `repository_handlers.go`,
`directory_listing_test.go`, `repository_handlers_test.go`,
`repository_handlers_hidden_folders_test.go`,
`directory_listing_windows_test.go` (call-site updates only).

Verification:

| Command | Result |
| --- | --- |
| `gofmt -l internal/task/service internal/task/handlers` | clean |
| `go build ./...` | ok |
| `go test ./internal/task/service/ ./internal/task/handlers/ -count=1` | ok (52.7s / 19.0s) |
| `golangci-lint run ./internal/task/... --timeout=10m` | 0 issues |

`golangci-lint` v2.9.0 cannot read Go 1.27 export data, so it was run against the
Go 1.26.0 toolchain that CI pins.
