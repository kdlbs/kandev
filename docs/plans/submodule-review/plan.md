---
spec: docs/specs/ui/requirements/submodule-review.md
created: 2026-08-05
status: implemented
---

# Implementation Plan: Nested Submodule Review

## Overview

Extend agentctl's existing repository tracker graph so initialized submodules are repository-scoped children with stable comparison anchors, then include the mixed root/child graph in the existing status, log, and cumulative-diff fan-outs. Adapt frontend review normalization, hierarchy, and Git mutations to preserve the root alongside named children and execute dependent mutations from deepest scope to parent. Finish with real desktop/mobile Review coverage and public documentation.

The approach follows [ADR-2026-08-05-nested-submodules-as-repository-scopes](../../decisions/2026-08-05-nested-submodules-as-repository-scopes.md). It does not add persistence, new routes, or a submodule-specific state store.

---

## Backend

### Repository graph discovery

- Add a focused repository-graph helper beside `apps/backend/internal/agentctl/server/process/manager.go`. It discovers only initialized gitlinks declared by each tracked repository, validates every task-root-relative child path, and recurses through initialized nested submodules.
- Use classified Git lifecycle commands through `internal/common/subproc` to read gitlink paths and parent-tree commit IDs. Do not execute raw `git` commands or walk arbitrary nested directories.
- Represent every child with its task-root-relative repository name, absolute working directory, and comparison ref. For a top-level repository named `frontend`, `vendor/parser` becomes `frontend/vendor/parser`; for a Git workspace root it remains `vendor/parser`.
- Set each submodule tracker's base branch/ref to the gitlink SHA resolved from its parent's comparison tree. Build the root tracker plus child trackers even when the root itself is a Git repository.
- Teach construction, rebind, rescan, reconciliation, polling-mode propagation, subscriptions, file/content search, and teardown to retain a valid root tracker alongside child trackers. Preserve the current bare multi-repository task-root behavior.

### Review-facing Git fan-out

- Add a manager snapshot method for ordered Git repository scopes. It includes `""` only when the workspace root tracker owns a real Git repository and includes every named sibling/submodule scope in stable path order.
- Update `apps/backend/internal/agentctl/server/api/git.go` so status, log, and cumulative-diff aggregation use that scope list. The root and submodule results must coexist; per-file cumulative payloads keep repository-relative `path`, nested `repository_name`, and exact `base_ref`.
- Keep `RepoSubpaths` for callers that need named paths, but audit `workspace.go`, content search, LSP, and file search assumptions so adding a submodule does not make root file access disappear or cause a bare task root to be treated as a Git repository.
- Preserve partial-success behavior, deterministic merge order, request cancellation, and the shared class-aware Git admission bound.

---

## Frontend

### Repository-scope normalization

- Update `apps/web/hooks/domains/session/use-review-sources.ts`, `apps/web/components/task/use-review-dialog.ts`, and their helpers so a valid empty root status is retained when named submodule statuses exist.
- Derive an internal repository-scope hierarchy from task repository count plus slash-delimited `repository_name` values. A named scope below `""`, or below another named scope, is a submodule; top-level sibling task repositories remain workspace repositories.
- Keep review identities as `(repository_name, path)`. Suppress a parent file whose workspace-relative path exactly matches an available child scope, while retaining the gitlink row when no child file source is available.
- Apply the same source precedence and normalization in `buildReviewSources` and `buildAllFiles` so the Changes panel and expanded Review dialog cannot disagree.

### Review presentation

- Rework `buildFileTree` in `apps/web/components/review/types.ts` to compose one hierarchy from repository-scope paths plus repository-relative file paths. Preserve top-level multi-repository roots, pin submodule boundary nodes, and keep duplicate relative filenames isolated by the existing composite review key.
- Render a small submodule indicator in `review-file-tree.tsx` and the repository group/diff headers used on phone. Add new accessible copy through i18n and regenerate the pseudo locale.
- Preserve the current desktop split Review surface. On phone, reuse the existing full-height Review dialog and sticky diff headers; do not introduce a second navigator or desktop-only control.

### Nested Git mutation ordering

- Update `apps/web/hooks/domains/session/use-session-git.ts` to include the empty root repository in file derivations and control summaries when it is a real status source.
- Replace flat concurrent fan-out for dependency-sensitive operations with repository-depth waves. Stage/commit/discard operations that affect descendants run deepest scopes first; sibling scopes at the same depth may keep the existing parallel and partial-result behavior.
- For commit-all with unstaged nested work, select scopes from all changed files, run child `stage + commit` before parent `stage + commit`, and stop an ancestor chain after a child failure so a parent never records an unintended gitlink.
- Keep explicit single-repository operations and unrelated sibling multi-repository behavior unchanged.

---

## Tests

- **Repository discovery and baselines:** `apps/backend/internal/agentctl/server/process/manager_submodule_test.go` creates a parent repository with a nested initialized submodule graph and verifies tracker names, comparison refs, root retention, subscription events, rescan/rebind behavior, invalid-path rejection, and teardown.
- **Review API aggregation:** extend `apps/backend/internal/agentctl/server/api/git_multi_repo_review_test.go` to verify root plus direct/nested submodule status, commit log, cumulative file payloads, stable per-scope bases, duplicate relative paths, and partial failure.
- **Source normalization and tree hierarchy:** extend `apps/web/hooks/domains/session/use-review-sources.test.ts`, `apps/web/components/review/review-dialog.build-files.test.ts`, `apps/web/components/review/types.multi-repo.test.ts`, and `apps/web/components/review/review-file-tree.test.tsx` for mixed root/named entries, nested boundary nodes, gitlink suppression/fallback, duplicate filenames, and accessible boundary labels.
- **Mutation order:** add a focused pure-helper test beside `use-session-git-grouping.test.ts` that proves deepest-first waves, sibling grouping, root inclusion, child-failure ancestor blocking, and unchanged single/sibling behavior.

## E2E Tests

- **Desktop scenario:** `apps/web/e2e/tests/review/submodule-review.spec.ts` creates a disposable parent repository with direct and nested initialized submodules, changes parent and child files, opens Review, and verifies the hierarchy, submodule indicators, textual diffs, distinct duplicate paths, and absence of duplicate gitlink-only rows. It then exercises the commit-all flow and verifies child commits precede parent gitlink commits.
- **Mobile scenario:** `apps/web/e2e/tests/review/mobile-submodule-review.spec.ts` opens the same kind of task in the `mobile-chrome` project and verifies the submodule scope in the sticky header, textual diff access, touch-reachable review controls, viewport containment, and no document horizontal overflow.
- Use a shared sibling helper for disposable Git setup and cleanup. Seed preconditions through Git/API helpers and assert outcomes through the UI; do not mutate the worker-scoped seed repository permanently.

## Public documentation

- Update `docs/public/sessions-and-review.md` (how-to/explanation) to state that initialized nested submodule files participate in repository-aware Review and that submodule PRs remain separate repository workflows.
- Update the **Changes and cumulative Review** entry in `docs/public/feature-status.md` (reference) to mention nested initialized submodule support.
- Validate public docs with `node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs`.

## Verification Results

- Backend graph, aggregation, and mutation-support changes are implemented. `go test ./internal/agentctl/server/process -count=1` passed 584 tests after the post-rebase task-root rescan regression; the nested graph subset passed 4 tests; the nested API aggregation subset passed 3 tests. `make -C apps/backend test` passed, `make -C apps/backend lint` reported 0 issues, and `make -C apps/backend build` completed.
- Frontend source, hierarchy, and mutation tests passed: 9 Vitest files, 118 tests. `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` completed successfully.
- `pnpm run build:vite` completed. The managed desktop Chromium E2E passed 1 test and the managed `mobile-chrome` E2E passed 1 test; each fixture removes its disposable repository tree in `finally`.
- The host-mode SSH workspace-source E2E passed after the task-root rescan fix, and the six-test workflow-start-step suite passed after a fresh backend/Vite build.
- Public documentation validation passed 58 tests and validated 41 published pages.

## Implementation Waves And Parallel Candidates

The default execution order is sequential in the primary conversation. The frontend presentation and mutation tasks are parallel-safe after the backend contract is complete because they own disjoint files; using subagents still requires explicit user authorization.

Wave 1:

- [x] [Task 01: Discover nested repository scopes](task-01-discover-nested-repository-scopes.md)
- [x] [Task 02: Aggregate root and submodule Git data](task-02-aggregate-submodule-git-data.md)

Wave 2 (parallel candidates after Task 02; user authorization required):

- [x] [Task 03: Present submodule review hierarchy](task-03-present-submodule-review-hierarchy.md)
- [x] [Task 04: Order nested Git mutations](task-04-order-nested-git-mutations.md)

Wave 3:

- [x] [Task 05: Prove responsive submodule review](task-05-prove-submodule-review.md)
- [x] [Task 06: Document nested submodule review](task-06-document-submodule-review.md)

## Open Questions

None.
