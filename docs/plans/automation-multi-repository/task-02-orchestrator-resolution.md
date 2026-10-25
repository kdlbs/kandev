---
id: "02-orchestrator-resolution"
title: "Orchestrator: resolve multiple explicit repositories per automation firing"
status: pending
wave: 2
depends_on: ["01-backend-data-model"]
plan: "plan.md"
spec: "../../specs/office/requirements/automations-settings.md"
---

# Task 02: Orchestrator multi-repository resolution

## Inputs

- `docs/specs/office/requirements/automations-settings.md` — Scenarios covering multiple
  explicit repositories, empty-list fallback, and `github_pr` override.
- `apps/backend/internal/orchestrator/event_handlers_automation.go` —
  `resolveAutomationRepository`, `resolveExplicitRepository`,
  `resolveGitHubPRTriggerRepository`, `resolveWorkspaceRepository`,
  `createAutomationTask`, `associateAutomationPR`. This file's two
  `a.RepositoryID` usages (in `resolveAutomationRepository`) are the only
  `automation.Automation.RepositoryID` call sites outside
  `internal/automation` itself (confirmed by a repo-wide grep during
  planning) — task 01 intentionally scopes its "no singular field remains"
  acceptance to the `internal/automation` package; this task is what retires
  the orchestrator's two usages.
- `apps/backend/internal/orchestrator/event_handlers_github.go` —
  `ReviewTaskRepository` struct definition.
- Task 01's output: `automation.Automation.RepositoryIDs []string` (replaces
  `RepositoryID string`).

## Change

1. In `resolveAutomationRepository`, replace the `a.RepositoryID != ""` branch
   with `len(a.RepositoryIDs) > 0`, calling a new
   `s.resolveExplicitRepositories(ctx, a.RepositoryIDs)`.
2. Add `resolveExplicitRepositories(ctx context.Context, repositoryIDs
   []string) []ReviewTaskRepository`:
   - For each ID, load the repository via `store.GetRepository` (same store
     access pattern as the existing `resolveExplicitRepository`).
   - On success, append `ReviewTaskRepository{RepositoryID: repo.ID,
     BaseBranch: defaultBranch, CheckoutBranch: defaultBranch}` where
     `defaultBranch` falls back to `automationDefaultBaseBranch` when
     `repo.DefaultBranch` is empty (same logic as today).
   - On failure to load a given ID, log a warning (same style as the existing
     failure log) and skip that ID — do not abort the whole resolution. A
     completely empty result (all IDs failed) is a valid "no repository
     available" outcome, which `createAutomationTask` already handles (see
     the `len(repositories) == 0` check).
3. Decide whether `resolveExplicitRepository` (singular) is now dead code:
   run `lsp references` on it. If unreferenced elsewhere, delete it; if kept
   for another caller, leave it and have `resolveExplicitRepositories` call it
   in a loop instead of duplicating the load logic.
4. No changes needed to `createAutomationTask`'s `Repositories:
   repositories` wiring — it already accepts the full slice.
5. `associateAutomationPR(ctx, task.ID, repositories[0].RepositoryID, ...)` —
   unchanged; `github_pr` triggers always resolve via
   `resolveGitHubPRTriggerRepository`, which is single-repo and untouched by
   this task.

## Acceptance

- A scheduled/webhook automation with 2+ `RepositoryIDs` creates one task with
  all of them attached, each on its own default branch — verified via a test
  asserting the `ReviewTaskRequest.Repositories` slice passed to
  `CreateReviewTask`.
- One unresolvable ID among several does not prevent the task from being
  created with the remaining, resolvable repositories.
- An automation with an empty `RepositoryIDs` list still falls back to
  `resolveWorkspaceRepository` exactly as before (no behavior change for the
  existing single/zero-repo path).
- `github_pr` trigger behavior is unchanged (still ignores `RepositoryIDs`
  entirely).
- No remaining `a.RepositoryID` (singular) reference in
  `internal/orchestrator`.

## Verification

```
make -C apps/backend test
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_automation_test.go`
  (create if none exists for this file — check `apps/backend/internal/orchestrator/*automation*test*` first)

## Dependencies

Task 01 (needs `Automation.RepositoryIDs` to exist).

## Parallelism

`parallel-safe` with task 03 — disjoint files (backend orchestrator vs.
frontend components), both depend only on task 01's finished contract, no
shared schema/migration/lockfile touched by either.

## Output contract

Summary of changes, whether `resolveExplicitRepository` was deleted or
retained (and why), exact test command output, and a note updating
`plan.md`'s Wave 2 checkbox and this file's `status` to `done`.
