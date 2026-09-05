---
created: 2026-09-05
status: completed
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-UI-TASK-CLEANUP-CONFIRMATION-001
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-deletion.md
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
legacy_specs: []
---

# Implementation Plan: Confirm Dirty Worktree Deletion

## Overview

Prevent task deletion from becoming an invisible, repeated cleanup failure when
an owned worktree contains local changes. Check every target before task
mutation, require explicit discard consent, and carry that consent into the
audited cleanup job. Use the shared task-delete dialog for the choice on desktop
and mobile.

## Confirmed root cause

- `Service.DeleteTask` prepares durable cleanup, deletes the task row, publishes
  `task.deleted`, and starts cleanup after the successful response path.
- `verifyCleanRedundantCheckout` rejects tracked or untracked changes. This
  safety check runs in the asynchronous cleanup worker, after the task has
  disappeared from the UI.
- The cleanup refusal preserves the checkout and branch, but the deleted task no
  longer gives the user a place to consent or recover.
- Current `main` does not retry forever. Cleanup is bounded to eight attempts
  with backoff. Repeating a deterministic dirty-worktree refusal is still noisy
  and cannot make progress.
- `TestDeleteTaskCleanupRetriesWhenAuditedWorktreeRemovalIsUnsafe` records the
  current behavior: deletion succeeds, the dirty checkout remains, and the job
  enters `retry_wait`.

## Scope

### In scope

- Preflight direct and cascade task deletion before any selected task changes.
- A typed dirty-worktree conflict that lists every affected worktree.
- Explicit discard consent in the HTTP, service, and durable cleanup contracts.
- Forced cleanup that bypasses only the clean-checkout refusal.
- Immediate terminal classification if an unconsented dirty checkout is found
  after task mutation.
- One shared, localized discard choice for every task-delete surface.
- Desktop and phone browser proof, including bulk and cascade behavior.
- Public documentation for dirty-worktree deletion behavior.

### Out of scope

- Automatic commits, pushes, patches, or backups of local changes.
- Force-reclaiming uncertain paths or remote SSH worktrees.
- Deleting a branch whose commits are not safely redundant.
- A general UI for inspecting or replaying terminal cleanup jobs.
- Changes to archive, session deletion, workspace reset, or quick-chat expiry.

## Technical approach

### Admit deletion before mutation

- Add one delete options value that keeps `cascade` and
  `discard_worktree_changes` independent.
- Resolve the full direct or cascade target set, gather its owned worktree
  snapshots, and inspect all local checkouts before task rows or cleanup jobs
  change.
- Return HTTP 409 with `error_code: task_delete_dirty_worktree` and the complete
  dirty-worktree list when consent is absent. Keep the service error typed for
  non-HTTP callers.
- Store consent in the additive cleanup snapshot JSON. Existing snapshots decode
  as unconsented.

### Preserve fail-closed cleanup

- Let a consented cleanup skip only the dirty-checkout rejection.
- Keep the pinned no-follow handle, exact path, owner, registration, expected
  commit, shared-reference, and branch ancestry checks mandatory.
- Keep the existing rule that preserves a branch with unique commits.
- Treat an unconsented dirty refusal found after admission as terminal after its
  first worker claim. Do not schedule retries for a condition that needs user
  consent.

### Surface the choice in the shared dialog

- Extend `TaskDeleteConfirmDialog` to return `discardWorktreeChanges` with the
  existing cascade value.
- Show an unchecked discard choice for a worktree executor, for a bulk selection
  that contains one, and when an enabled cascade can include child worktrees.
- State that tracked and untracked local files are permanently removed. Disable
  Delete until the user makes the explicit choice.
- Keep a task visible when the typed conflict is returned. Show the same
  localized retry guidance at every single and bulk delete entry point.

### Mobile design contract

- Keep the existing centered `AlertDialog`; no new navigation or drawer is
  needed for this blocking decision.
- Put the choice in the existing scrolling body and give its label a 44 CSS px
  touch target.
- Keep the responsive full-width footer actions and current focus, Escape,
  overflow, and safe-area behavior.
- Prove phone containment and choice reachability through the real task drawer
  entry point. Prove the equivalent shared behavior on desktop.

## Test strategy

- Backend unit and handler tests cover preflight atomicity, direct and cascade
  conflicts, consent snapshot round trips, constrained force cleanup, and
  terminal handling for a post-admission race.
- Component and API tests cover choice visibility, reset, disabled actions,
  query construction, and typed conflict classification.
- Desktop and mobile Playwright flows create a dirty task worktree, prove cancel
  and unconsented preservation, then consent and prove task/worktree removal.
- Internationalization and public-documentation checks run with the focused
  suites.

## Work orders

Wave 1:

- [x] [Task 01: Enforce the dirty-worktree deletion boundary](task-01-enforce-delete-boundary.md)

Wave 2:

- [x] [Task 02: Add explicit discard consent to task deletion](task-02-surface-discard-consent.md)

Execution is sequential in the primary conversation. Task 02 depends on the
backend contract from Task 01. The wave labels do not authorize subagents.

## Risks

- A checkout can become dirty between admission and cleanup. The worker must
  still fail closed when its snapshot has no consent.
- Cascade admission must be atomic. Finding one dirty descendant cannot leave a
  partially deleted subtree.
- Consent must never weaken path identity or Git redundancy checks.
- Bulk deletion uses separate requests. The UI must report partial success
  accurately if another process changes one checkout between requests.

## Validation commands

- `(cd apps/backend && go test ./internal/worktree ./internal/task/service ./internal/task/handlers -count=1)`
- `(cd apps/web && pnpm exec vitest run components/task/task-delete-confirm-dialog.test.tsx lib/api/domains/kanban-api.test.ts hooks/use-task-actions.test.ts)`
- `(cd apps/web && pnpm run i18n:check)`
- `(cd apps/web && pnpm e2e:run tests/task/sidebar-delete-confirm.spec.ts)`
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-confirmation-text-hierarchy.spec.ts)`
- `node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs`

## Results

- Added audited dirty-worktree inspection and typed service/HTTP conflicts. Direct and cascade
  deletion now preflight every target before task mutation; consent is persisted in cleanup
  snapshots and bypasses only the clean-checkout refusal. Legacy or raced unconsented refusals are
  terminal, while owner, path, registration, commit, branch, and shared-reference checks remain
  mandatory.
- Added the shared localized discard choice and propagated it through sidebar, mobile, Kanban,
  graph, list, bulk, and task-message delete actions. Delete stays disabled until required consent
  is selected, and typed conflicts keep client state intact while showing retry guidance.
- Added five-locale and pseudo-locale copy, public Git operations guidance, backend/component/API
  regressions, and desktop/mobile browser coverage.
- Verification passed: 2,685 focused backend tests; 29 focused frontend tests; web lint and
  typecheck; i18n checks; desktop sidebar E2E (1 passed); mobile confirmation E2E (4 passed); and
  public-doc validation (61 tests, 45 pages).
