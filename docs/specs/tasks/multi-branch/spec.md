# Multi-branch tasks

**Status:** shipped
**Owner:** Kandev backend
**Date:** 2026-06-01
**Related:** [ADR 0013](../../decisions/0013-multi-branch-tasks.md)

## What

A single Kandev task can hold N `(repository, branch)` pairs — including multiple branches on the *same* repository. Each pair gets its own worktree under the task directory. PRs, sessions, changes, and review surfaces all key on the pair so the agent can fan out work across branches without fragmenting into sibling tasks.

## Why

The previous model forced one task = one branch per repository. Users wanted:

- **Stacked PRs** — work that naturally splits across two branches against the same repo while staying in one conversation with the agent.
- **Feature-flag rollouts** — same repo, "with flag on" and "with flag off" branches reviewed side-by-side.
- **A/B experiments** — two implementations of the same change, opened as two PRs, compared from one task.
- **Multi-repo, but stronger** — the existing multi-repo (different repos in one task) was a natural neighbour of "same repo, different branches". Unifying them removed an arbitrary asymmetry.

Workarounds (sibling tasks, manually managing two worktrees) lost shared context: the agent's history, the kanban view, the chat thread.

## Surface

### Backend API

- **`Service.AddBranchToTask(task_id, repository_id, base_branch?, checkout_branch?)`** — appends a new `task_repositories` row to a live task. Enforces uniqueness on the canonical 4-column key `(task_id, repository_id, base_branch, checkout_branch)` so both branch fields disambiguate siblings.
- **`Service.CreateTask`** — already accepted `[]TaskRepositoryInput`; now accepts duplicate `repository_id` entries when `checkout_branch` differs.
- **`task_repositories.UNIQUE(task_id, repository_id, base_branch, checkout_branch)`** — relaxed from the legacy `UNIQUE(task_id, repository_id)` via the `migrateTaskRepositoriesAllowMultiBranch` migration. Both branch columns participate because the worktree executor anchors the branch in `base_branch` (leaving `checkout_branch` empty) while the local executor inverts the split. Either column alone would miss one of the two shapes.

### MCP

- **`add_branch_to_task_kandev`** — new tool that takes `task_id`, `repository_id`, `checkout_branch`, optional `base_branch`. Backed by `ws.ActionMCPAddBranchToTask` and `handleAddBranchToTask`.
- **`create_task_kandev`** — unchanged externally; agents can already submit multiple repository entries.

### Worktrees

- Single-branch tasks: `~/.kandev/tasks/<task-dir>/<repo>/` (unchanged).
- Multi-branch tasks: first occurrence of each repo keeps `~/.kandev/tasks/<task-dir>/<repo>/`; additional occurrences sit as siblings at `~/.kandev/tasks/<task-dir>/<repo>-<branch-slug>/`. The slug is derived deterministically from `CheckoutBranch` (or `BaseBranch` when the checkout branch is empty) via `worktree.SanitizeBranchSlug`.
- The orchestrator (`buildRepoSpecs`) detects same-repo duplicates and tags additional rows with a `BranchSlug`; the worktree manager applies the slug as a suffix at the task-root level. Sibling siting (rather than nesting inside the primary) keeps each worktree's git scope isolated.
- Subsequent sessions on the same task, including handoffs and additional agents, reuse the task's existing worktree IDs for every `(repository, branch)` pair. They do not create a new task directory or sibling worktree set unless the task itself gains a new branch/repository pair.

### PRs

- `task_prs.UNIQUE(task_id, repository_id, pr_number)` already permitted multiple PRs per (task, repo). `task_prs.head_branch` already disambiguates which branch the PR tracks. No schema change.

### Frontend

- `TaskRepository.checkout_branch` was already on the http type.
- Worktrees are keyed by `worktree.id` in the Zustand store, so two worktrees with the same `repository_id` already coexist.
- Repo chips in chat-message renderers now key on `(repository_id, checkout_branch)` so multi-branch tasks render distinct chips instead of collapsing.
- In the task-creation dialog, every workspace, discovered-on-disk, and remote provider repository picker shows an accent-colored check (with an accessible `Already added` label) when another repository row already selects that repository. The current row does not mark its own selection.
- Marked options remain selectable so users can intentionally create a multi-branch task from one repository. Removing or changing the other row removes the marker immediately.
- Repository-chip tooltips keep long local paths inside a viewport-safe, wrapping surface. Closing a repository picker does not reveal its tooltip until the pointer leaves and deliberately hovers the chip again.
- Review surfaces expose one linked pull request at a time when a task has multiple PRs. A task-scoped selector defaults to the primary (oldest) PR, remembers an in-session override, and falls back to the primary PR when that override disappears.
- Selecting a PR changes the remote PR diff contribution while preserving the existing source precedence: uncommitted worktree changes, then cumulative committed changes, then the selected PR. PR-only views and PR timeline rows resolve the exact PR rather than the task primary.
- The selector is available on desktop, phone, and coarse-pointer tablet. Phone uses a touch-sized bottom-menu treatment inside the existing Review surface; switching keeps Review open and exposes selected-PR loading, empty, and retry states.
- Full "+ Branch" UI affordance and grouped repo > branch tabs are deferred — agents drive multi-branch via the MCP tool today.

### Task-creation scenarios

- **GIVEN** a task-creation repository row selects a workspace or discovered-on-disk repository, **WHEN** the user opens another repository selector, **THEN** that repository remains selectable and is visibly marked by a compact accent-colored check whose accessible label is `Already added`.
- **GIVEN** a task-creation Remote row selects a provider-backed repository, **WHEN** the user opens another Remote repository selector, **THEN** the same provider repository remains selectable and is visibly marked by the same compact accent-colored check.
- **GIVEN** a repository is marked because another row selects it, **WHEN** the user changes or removes that other row, **THEN** the marker disappears from the open or next-opened selector.
- **GIVEN** a row already selects a repository and no other row selects it, **WHEN** the user reopens that row's selector, **THEN** its current repository is not marked as a duplicate.
- **GIVEN** a selected repository has a long unbroken local path, **WHEN** its tooltip opens, **THEN** the tooltip wraps the path and remains within the viewport instead of covering adjacent repository controls.
- **GIVEN** a user selects a repository from a repository-chip picker, **WHEN** the picker closes while the pointer remains over the chip, **THEN** no repository tooltip opens until the pointer leaves and deliberately hovers the chip again.

## Non-goals

- **Auto-stack PRs.** Multi-branch lets you open N PRs; it does not detect base/branch relationships and stack them. Users do that themselves.
- **Cross-branch merge orchestration.** Each branch's PR lifecycle is independent.
- **Branch deletion / cleanup automation.** A `RemoveBranchFromTask` symmetric service method is planned but not in v1.
- **Aggregate multi-PR review.** Review does not merge sibling PR diffs into one file list because two PRs can carry different revisions of the same repository path.
- **Independent per-PR review history.** Reviewed-file and pending-comment identity remain session/repository/path scoped. Switching PRs treats a different diff hash as a new visible revision; PR-qualified persistence is separate data-model work.

## Risks

- **Slug collision.** Two distinct branches that sanitize to the same slug (e.g. `feat/a` vs `feat-a`) would collide on disk. The service-layer dedup catches matching `CheckoutBranch` exactly; near-identical names trip `git worktree add` with a clean error.
- **Repo-lock contention.** Worktrees for different branches of the same repo serialize on the per-repo lock in `worktree.Manager`. Multiple concurrent agents on the same task = lock queue, not parallelism. Acceptable for safety; revisit if it becomes a bottleneck.
- **Migration replay.** The constraint relaxation migration is idempotent and triggers on a substring of the legacy DDL. Databases that already migrated skip cleanly.

## Acceptance tests

- `TestSanitizeBranchSlug` — slug determinism + handling of slashes, dots, special chars.
- `TestTaskWorktreePath_BranchSlugNesting` — empty slug stays flat, non-empty slug nests.
- `TestCreateTask_AllowsSameRepoDifferentBranches` — a task can be born with two rows on the same repo.
- `TestCreateTask_RejectsSameRepoSameBranch` — dedup guard still rejects exact duplicates.
- `TestAddBranchToTask_HappyPath` — second branch appended after the fact lands as a new row.
- `TestAddBranchToTask_RejectsDuplicate` — re-adding the same `(repo, branch)` errors.
- `TestLaunchPreparedSession_MultiBranch_ReusesWorktreeIDsByBranchSlug` — a follow-on session for the same task reuses each existing branch worktree instead of preparing a new task directory.
- Task-creation component and mobile E2E coverage prove the accessible, compact selected-repository marker for workspace/on-disk and Remote provider selectors while preserving option selection, viewport-safe long-path tooltips, and post-selection tooltip suppression.
- Web unit tests prove selected-PR default, override, task isolation, and removed-PR fallback behavior.
- Desktop and mobile Playwright tests prove a two-PR task can switch Review from the primary PR to a sibling PR without stale files, overflow, or closing the surface.

## Open questions

- "Primary" branch for the kanban card / task title rendering — currently the lowest-position row. Acceptable until users complain.
- Whether `update_task_kandev` should accept the multi-branch shape for bulk edits, or whether `add_branch_to_task_kandev` + a future `remove_branch_from_task_kandev` is enough. Deferred to feedback.
- "+ Branch" UI button — design and placement open. The MCP tool is enough for the agent-driven flow today.
