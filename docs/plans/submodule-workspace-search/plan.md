---
spec: docs/specs/ui/requirements/task-workspace-content-search.md
created: 2026-08-11
status: implemented
---

# Implementation Plan: Preserve Root Search With Submodules

## Overview

Fix workspace search tracker selection so a real Git root remains eligible when
initialized submodules add named trackers. Prove the behavior first at the
agentctl manager boundary, preserve bare multi-repository task-root behavior,
then exercise Cmd/Ctrl+Shift+K through the existing command palette.

## Confirmed Root Cause

`SearchWorkspaceFileResults` and `contentSearchTrackers` include the root in a
mixed tracker graph only when `RepositoryName()` is non-empty. A real workspace
Git root intentionally uses the empty repository identity, so adding the first
submodule changes both searches from root-only to child-only. The correct
distinction is whether the root owns Git metadata, which `RepositoryScopes`
already represents with `gitIndexPath != ""`.

Retaining the root exposed a second file-enumeration defect: the root's
`git ls-files --cached --others --exclude-standard` output includes mode-160000
submodule Gitlinks, but every output line was classified as a file. Searching
the submodule directory name could therefore return a directory as a root file.

---

## Backend

### Search tracker selection

- Update `apps/backend/internal/agentctl/server/process/workspace_files.go` and
  `workspace_content_search.go` to include the unnamed root when it owns a real
  Git repository, even when named submodule trackers coexist.
- Keep the current single-repository fallback and named sibling-repository
  aggregation unchanged. Do not search a bare task root or change response
  paths, repository identities, ordering, limits, or fuzzy scoring.
- Use the same real-Git-root predicate as `Manager.RepositoryScopes`; do not
  infer repository ownership from the display/transport name.
- Preserve tracked Git modes while enumerating files and exclude mode-160000
  Gitlinks. Continue including untracked, non-ignored files.

## Frontend

No production frontend change is planned. The command palette already renders
the structured `results` returned by `workspace.files.search` without filtering
the unnamed root, and content search already groups the same empty root identity.

## Tests

- **What:** File-name search returns matches from a Git root and initialized
  submodule in one response.
  **File:** `apps/backend/internal/agentctl/server/process/workspace_search_submodule_test.go`.
  **How:** Construct real parent/child Git repositories, add the child as an
  initialized submodule, refresh tracker file lists, and assert both structured
  repository identities and task-root-relative paths.
- **What:** Content search returns matches from the same mixed root/submodule
  graph.
  **File:** `apps/backend/internal/agentctl/server/process/workspace_search_submodule_test.go`.
  **How:** Search matching tracked content through `Manager.SearchWorkspaceContent`
  and assert one result from each scope.
- **What:** Bare task roots containing named sibling repositories remain
  excluded.
  **Files:** Existing `workspace_file_search_test.go` and
  `workspace_content_search_test.go` coverage.
  **How:** Rerun the existing multi-repository tests beside the new regressions.
- **What:** Searching the exact submodule directory name does not return the
  parent Gitlink as a file, while the child tracker's real files remain eligible.
  **File:** `apps/backend/internal/agentctl/server/process/workspace_search_submodule_test.go`.
  **How:** Search the real initialized fixture for `lib`, reject the root
  `vendor/lib` pseudo-file, and retain the existing child-file assertion.

## E2E Tests

- **Scenario:** Given a task whose Git root and initialized submodules each
  contain `README.md`, when the user opens Files with Cmd/Ctrl+Shift+K and
  searches for that name, root and child repository groups all appear.
- **File:** `apps/web/e2e/tests/command-panel.spec.ts`.
- **What to verify:** Reuse `createSubmoduleReviewFixture`, wait for the task
  session/worktree, search through the command palette, and assert the empty
  root plus direct and nested submodule group identities. Always remove the
  disposable source tree in `finally`.
- No mobile-specific E2E is added. This fix changes shared backend result data,
  not layout, touch behavior, navigation, scrolling, or viewport composition;
  the existing mobile file-search interaction consumes the same API contract.

## Verification Results

Task 01 passed its focused gate: 3 process tests and 1 API test. Both new
regressions failed for the expected child-only result before their production
changes, then passed; the final full run of both touched backend packages also
passed. Task 02 passed 1 Chromium test after its RED run showed the original
bug's 2 groups instead of the expected root plus 2 submodules. Review
remediation added and passed setup-failure cleanup coverage for the shared E2E
fixture, then revalidated the command-palette happy path with retries disabled.
PR CI remediation also removed workspace-picker fixture leakage and corrected
two order-dependent E2E expectations. The two affected specs passed together
with one worker, and exact Chromium/mobile shard 4/14 passed 137 tests with 7
intentional skips and retries disabled.
Independent review then found that retaining the root also exposed its
mode-160000 Gitlink as a pseudo-file. A focused regression failed on
`vendor/lib`, passed after mode-aware enumeration excluded Gitlinks, and the
four-test behavior gate, both touched backend packages, and CI-style
changed-file lint all passed.

## Implementation Waves And Parallel Candidates

Default execution is sequential in the primary conversation. No task is marked
parallel-safe because the browser proof depends on the backend behavior.

Wave 1:

- [x] [Task 01: Correct mixed-graph search](task-01-correct-mixed-graph-search.md)

Wave 2:

- [x] [Task 02: Prove command-palette search](task-02-prove-command-palette-search.md)

## Open Questions

None.
