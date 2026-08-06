# ADR-2026-08-05-nested-submodules-as-repository-scopes: Model Nested Submodules as Repository Scopes

**Status:** accepted
**Date:** 2026-08-05
**Area:** backend, frontend, protocol

## Context

Kandev initializes recursive Git submodules in task worktrees, but agentctl tracks only the parent repository or immediate sibling repositories. Review therefore receives the parent repository's gitlink change while the changed files inside initialized submodules remain invisible. Flattening those files into the parent repository would make them visible, but stage, discard, commit, base-resolution, and file-at-ref operations would still execute against the wrong Git repository.

## Decision

Initialized submodules are represented by the existing repository-scoped Git status, log, cumulative-diff, file, and mutation contracts.

- Each submodule tracker uses a task-workspace-relative, slash-delimited `repository_name`, such as `vendor/parser` or `frontend/vendor/parser`. The workspace root keeps the existing empty repository name. Immediate sibling task repositories keep their existing top-level names.
- A submodule's comparison anchor is the gitlink commit recorded by its parent repository at the parent's comparison anchor. Nested anchors are resolved recursively, so committed submodule work remains reviewable after an agentctl restart.
- Mixed root and named repository statuses are valid. Consumers must not discard the empty root status merely because named statuses exist.
- Review presents the repository-scope hierarchy as directories and marks submodule boundaries. When child file diffs are available, the parent gitlink-only row is suppressed from the review list.
- Git mutations that span nested scopes execute in dependency waves from deepest submodule to parent. Independent sibling scopes may execute in parallel within a wave. This is required for commit-all to create child commits before recording their new gitlinks in parent commits.
- Discovery and all resulting Git work use the existing class-aware subprocess admission system. An inaccessible or uninitialized submodule is omitted as a child scope; its parent repository remains usable and may still expose the gitlink change.

The existing routes and payload fields remain in place. The protocol change is semantic: `repository_name` may identify a nested path, and an empty root entry may coexist with named entries.

## Consequences

The current multi-repository infrastructure remains the single implementation path for repository-aware review and Git operations. Backend tracker discovery, aggregate API fan-out, frontend status normalization, review identities, and Git mutation ordering all need to understand a repository tree instead of assuming either one unnamed repository or several named siblings.

Each initialized submodule adds polling and diff work, but Git subprocess concurrency remains bounded and fair through the existing admission controller. Git-host pull requests remain repository-specific: showing and committing nested changes does not create or coordinate pull requests for submodule remotes.

## Alternatives Considered

### Flatten submodule files into the parent tracker

Rejected because Git treats a submodule as one gitlink. Parent-scoped stage, discard, ref lookup, and commit commands cannot correctly mutate files in the child repository.

### Run `git submodule foreach --recursive` for each Review request

Rejected because it would introduce a second parsing and aggregation path, would not provide live repository-scoped status events, and would still need custom routing for every mutation.

### Add a submodule-specific API and frontend state model

Rejected because the existing multi-repository contracts already carry repository-relative file paths, per-repository bases, comments, status, commits, and mutations. A parallel contract would duplicate those semantics and drift.

### Keep showing only gitlink SHAs

Rejected because it leaves reviewers unable to inspect or comment on the code changed inside the submodule, which is the user problem this decision addresses.
