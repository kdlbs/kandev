---
id: "01-correct-mixed-graph-search"
title: "Correct mixed-graph search"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/task-workspace-content-search.md"
---

# Task 01: Correct Mixed-Graph Search

## Acceptance

- File-name and content search each return root and submodule matches from a
  mixed unnamed-root/named-child tracker graph.
- Bare task roots with named sibling repositories remain excluded, with no
  duplicate results or search failure.
- Parent repository Gitlinks are excluded from file results; real files owned
  by initialized child trackers remain searchable.
- Response identities, paths, ranking, limits, and existing single/multi-repo
  behavior remain unchanged.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/process ./internal/agentctl/server/api -run '^(TestManagerSearchWorkspaceFileResultsIncludesRootAndSubmodule|TestManagerSearchWorkspaceFileResultsExcludesSubmoduleGitlink|TestManagerSearchWorkspaceContentIncludesRootAndSubmodule|TestManagerSearchWorkspaceContentGroupsResultsByRepository|TestGetFileList_HidesOnlyRootOwnershipMarker|TestHandleFileSearchIncludesEveryTaskRepository)$' -count=1 -v
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_files.go`
- `apps/backend/internal/agentctl/server/process/workspace_content_search.go`
- `apps/backend/internal/agentctl/server/process/workspace_search_submodule_test.go` (new)

## Dependencies

None.

## Parallelism

`sequential` — both search paths share the same tracker-graph invariant and
regression fixture.

## Inputs

- Spec bullets and scenarios for mixed root/submodule and bare multi-repo search.
- `Manager.RepositoryScopes` as the existing real-Git-root predicate.
- `manager_submodule_test.go` and `workspace_tracker_test.go` Git fixture helpers.
- Confirmed failing repro: searching a parent file plus submodule file returned
  only the submodule path.

## Output contract

Report RED and GREEN commands/results, root-selection change, exact files
changed, bare-root preservation, cleanup, blockers/risks, and synchronized
task/plan status.

## Results

Implemented mixed-graph root retention for file-name and content search by
using the same real-Git-root predicate as `Manager.RepositoryScopes`. The new
fixture builds real parent/child repositories and an initialized submodule;
existing bare multi-repository tests preserve the exclusion boundary.

RED:

- `go test ./internal/agentctl/server/process -run '^TestManagerSearchWorkspaceFileResultsIncludesRootAndSubmodule$' -count=1 -v`
  failed as expected: only `vendor/lib|vendor/lib/child-search-target.txt`
  was returned.
- `go test ./internal/agentctl/server/process -run '^TestManagerSearchWorkspaceContentIncludesRootAndSubmodule$' -count=1 -v`
  failed as expected: only `vendor/lib|child-search-target.txt` was returned.

GREEN:

- Each focused regression passed after its corresponding one-line production
  change.
- Final task gate:
  `go test ./internal/agentctl/server/process ./internal/agentctl/server/api -run '^(TestManagerSearchWorkspaceFileResultsIncludesRootAndSubmodule|TestManagerSearchWorkspaceContentIncludesRootAndSubmodule|TestManagerSearchWorkspaceContentGroupsResultsByRepository|TestHandleFileSearchIncludesEveryTaskRepository)$' -count=1 -v`
  passed 3 process tests and 1 API test.
- Final package audit:
  `go test ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1`
  passed both touched packages.
- Independent-review remediation RED:
  `go test ./internal/agentctl/server/process -run '^TestManagerSearchWorkspaceFileResultsExcludesSubmoduleGitlink$' -count=1 -v`
  failed because root search returned `{Path:vendor/lib}` as a file.
- Independent-review remediation GREEN: the same focused regression passed
  after file enumeration retained tracked modes and excluded mode-160000
  Gitlinks. The four-test behavior gate passed, including tracked root/child,
  content-search, and untracked-file coverage. Both touched packages passed
  again, and the CI-style changed-file lint reported 0 issues against base
  `66eb87ac307db76b8cb3ba5fcfae73ff6d7d3e6c`.

Production and regression files match **Files likely touched**; the spec and
plan were synchronized with the remediation. Temporary artifacts: none.
Blockers/remaining Task 01 risks: none.
