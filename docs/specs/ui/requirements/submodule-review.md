---
status: active
system: ui
created: 2026-08-05
owners:
  - kandev
---
# Nested Submodule Review Requirements

## Overview

Repositories that use Git submodules currently show only a changed gitlink commit in Changes and Review. A reviewer cannot inspect, comment on, or approve the actual files changed inside the submodule without leaving Kandev and reconstructing the nested diff manually.

## Requirements

### REQ-UI-SUBMODULE-REVIEW-001: Nested Submodule Review

**Intent:** Repositories that use Git submodules currently show only a changed gitlink commit in Changes and Review. A reviewer cannot inspect, comment on, or approve the actual files changed inside the submodule without leaving Kandev and reconstructing the nested diff manually.

#### Acceptance criteria

- **AC-UI-SUBMODULE-REVIEW-001.1:** Changes and Review include changed files from every initialized submodule present when the task repository graph is discovered, including recursively nested submodules.
- **AC-UI-SUBMODULE-REVIEW-001.2:** Parent-repository files and submodule files can appear in the same Review session. Each file keeps a repository-relative path and an unambiguous repository scope.
- **AC-UI-SUBMODULE-REVIEW-001.3:** Review renders submodule scopes in their task-workspace directory hierarchy and gives each submodule boundary a small visible and accessible indication. A nested submodule appears beneath its parent submodule rather than as an unrelated top-level repository.
- **AC-UI-SUBMODULE-REVIEW-001.4:** Review keeps newly discovered directory and submodule scope nodes reachable when review sources arrive in separate updates. It expands only directories introduced by the update and preserves any collapsed state the reviewer chose for existing directories.
- **AC-UI-SUBMODULE-REVIEW-001.5:** When Kandev can show the child files for a changed submodule, Review omits the parent's raw gitlink-only row. If the child is unavailable, Review retains the parent gitlink change instead of hiding the only evidence of the change.
- **AC-UI-SUBMODULE-REVIEW-001.6:** Uncommitted and committed submodule changes use the same source precedence, diff limits, reviewed hashes, stale detection, anchored comments, findings, filtering, status markers, and file-opening behavior as other Review files.
- **AC-UI-SUBMODULE-REVIEW-001.7:** Each submodule compares against the gitlink commit recorded by its parent at the parent's comparison anchor. The comparison does not expand to unrelated history from the submodule's default branch.
- **AC-UI-SUBMODULE-REVIEW-001.8:** Stage, unstage, discard, file-at-ref, and per-file editing actions execute in the file's owning repository scope.

## Migrated source detail

Decision: [ADR-2026-08-05-nested-submodules-as-repository-scopes](../../../decisions/2026-08-05-nested-submodules-as-repository-scopes.md)

## Why

Repositories that use Git submodules currently show only a changed gitlink commit in Changes and Review. A reviewer cannot inspect, comment on, or approve the actual files changed inside the submodule without leaving Kandev and reconstructing the nested diff manually.

## What

- Changes and Review include changed files from every initialized submodule present when the task repository graph is discovered, including recursively nested submodules.
- Parent-repository files and submodule files can appear in the same Review session. Each file keeps a repository-relative path and an unambiguous repository scope.
- Review renders submodule scopes in their task-workspace directory hierarchy and gives each submodule boundary a small visible and accessible indication. A nested submodule appears beneath its parent submodule rather than as an unrelated top-level repository.
- Review keeps newly discovered directory and submodule scope nodes reachable when review sources arrive in separate updates. It expands only directories introduced by the update and preserves any collapsed state the reviewer chose for existing directories.
- When Kandev can show the child files for a changed submodule, Review omits the parent's raw gitlink-only row. If the child is unavailable, Review retains the parent gitlink change instead of hiding the only evidence of the change.
- Uncommitted and committed submodule changes use the same source precedence, diff limits, reviewed hashes, stale detection, anchored comments, findings, filtering, status markers, and file-opening behavior as other Review files.
- Each submodule compares against the gitlink commit recorded by its parent at the parent's comparison anchor. The comparison does not expand to unrelated history from the submodule's default branch.
- Stage, unstage, discard, file-at-ref, and per-file editing actions execute in the file's owning repository scope.
- A commit-all operation that covers nested repositories commits the deepest changed submodules before their parents so every parent commit records the resulting child gitlink. Independent sibling repositories at the same depth may proceed in parallel.
- Desktop keeps the existing Review split surface and file tree. Phone Review keeps its existing full-height diff surface; submodule identity is visible in the sticky repository/diff header, so no required cue depends on the desktop-only file tree or hover.
- When repository-scope and file headers are both sticky, they occupy separate vertical lanes. A scope header, including **Other changes** for the workspace root, never covers the current file identity or its controls on desktop or phone.

## API surface

No route names or top-level payload shapes change.

- `GET /api/v1/git/status/multi?fresh=<bool>` returns one status entry for each tracked repository scope. A Git workspace root uses `repository_name = ""`; initialized submodules use their slash-delimited task-workspace-relative path.
- `GET /api/v1/git/log` and `GET /api/v1/git/cumulative-diff` include the workspace root and initialized submodule scopes when no `repo` query parameter pins one scope.
- Existing `repo=<subpath>` mutation and file routes accept the same slash-delimited submodule scope and continue to receive repository-relative file paths.
- Existing workspace-stream Git events use the same `repository_name` convention. Mixed empty and named repository entries are valid for one session.

For a multi-repository task, a top-level attached repository keeps its existing scope name, for example `frontend`; its nested submodule uses the combined task-root-relative scope, for example `frontend/vendor/parser`.

## Failure modes

| Condition                                                                           | Observable behavior                                                                                                                                                                       |
| ----------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A declared submodule is uninitialized, inaccessible, or its Git metadata is invalid | The child scope is skipped, the rest of the repository graph remains reviewable, and the parent gitlink change remains visible when changed.                                              |
| One submodule status, log, or cumulative-diff request fails                         | Other repository scopes remain available and the existing per-repository partial-failure behavior applies.                                                                                |
| A submodule comparison gitlink cannot be resolved from the parent comparison tree   | Uncommitted child changes remain visible; committed-range collection follows the existing unresolved-base failure behavior for that scope without substituting another repository's base. |
| The same relative file path changes in a parent, sibling repository, and submodule  | Review keeps separate identities and comments for every `(repository_name, path)` pair.                                                                                                   |
| Commit-all fails in one child scope                                                 | Ancestor commits that depend on that child are not attempted; independent scopes report their own results through the existing partial-success UI.                                        |
| A submodule is added or removed after agentctl's repository graph is built          | It becomes part of nested review after the existing workspace rescan or session restart; automatic hot discovery of arbitrary graph changes is not required in this iteration.            |

## Scenarios

- **GIVEN** a task repository with an initialized submodule containing an uncommitted file edit and a changed parent file, **WHEN** Review opens, **THEN** both textual diffs appear and the submodule file sits beneath a marked submodule boundary.
- **GIVEN** a submodule containing another initialized submodule, **WHEN** files change in both children, **THEN** Review shows both nested directory boundaries and each file's repository-relative diff.
- **GIVEN** Review first receives a parent submodule file and later receives a file from an initialized nested submodule, **WHEN** the later review-source update is rendered, **THEN** the intermediate directory and nested boundary are expanded automatically so the nested scope is reachable without an extra expansion step, while previously collapsed existing directories remain collapsed.
- **GIVEN** a submodule commit made after the task comparison point, **WHEN** the session reloads or agentctl restarts and Review opens, **THEN** the committed file diff is computed from the parent-recorded gitlink commit and remains visible.
- **GIVEN** a parent gitlink change whose child file diffs are available, **WHEN** Review builds its file list, **THEN** it shows the child files without a duplicate raw gitlink-only row.
- **GIVEN** an unavailable submodule whose gitlink changed, **WHEN** Review opens, **THEN** the parent gitlink row remains visible and other repositories continue to render.
- **GIVEN** `README.md` changes in the parent and `README.md` changes in a nested submodule, **WHEN** the user reviews, comments on, stages, or discards either file, **THEN** the action and review state apply only to the selected repository scope.
- **GIVEN** staged changes in a nested submodule and its parent, **WHEN** the user commits all reviewed work, **THEN** Kandev commits the child first and the parent commit records the child's new commit ID.
- **GIVEN** a phone-sized viewport, **WHEN** the user opens Review for a submodule file, **THEN** the sticky diff header identifies the submodule scope and the file remains reviewable without document-level horizontal overflow.
- **GIVEN** changed files in the workspace root and a nested submodule, **WHEN** the reviewer scrolls a root file beneath the sticky **Other changes** scope header on desktop or phone, **THEN** the scope header remains above the file header without intersecting its file identity or control hit targets.

## Out of scope

- Fetching or initializing a submodule that the existing hardened worktree setup could not initialize.
- Automatically creating, linking, pushing, or merging pull requests for submodule remotes. Those remain separate Git-host repository workflows.
- Treating arbitrary nested Git repositories that are not declared submodules as part of the parent repository graph.
- Automatically hot-reloading newly added or removed submodules without the existing workspace rescan or a session restart.
- Adding a new mobile file-tree navigator; phone Review keeps its current focused diff composition.

## Implementation plan

See [Nested Submodule Review plan](../../../plans/submodule-review/plan.md).

The late-source tree expansion repair is tracked in the [Review tree late expansion plan](../../../plans/review-tree-late-expansion/plan.md).
