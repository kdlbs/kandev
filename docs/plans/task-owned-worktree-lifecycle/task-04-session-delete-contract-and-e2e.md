---
id: "04-session-delete-contract-and-e2e"
title: "Verify session deletion preserves workspaces"
status: done
wave: 4
depends_on: ["03-task-owned-durable-cleanup"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/session-delete-resource-cleanup.md"
decision: "../../decisions/2026-08-08-task-owned-worktree-lifetime.md"
---

# Task 04: Verify Session Deletion Preserves Workspaces

## Acceptance

- Desktop and mobile confirmation dialogs say that conversation history is
  deleted while the task workspace and files are retained.
- Neither dialog fetches or warns about uncommitted/unpushed state; destructive
  workspace warnings remain on task archive/delete.
- Deleting the only session leaves the task with zero sessions, its directory,
  Git worktree registration, and uncommitted file intact; a new session reuses
  the retained workspace.
- Deleting one of two sessions sharing a workspace leaves the other session
  operational in that workspace.
- The existing mobile session-actions sheet and viewport-safe confirmation dialog
  remain the native entry point with unchanged action semantics and touch targets.
- Public docs distinguish ordinary session deletion from task archive/delete and
  keep Quick Chat's task-backed cleanup semantics separate.
- Operations and Kubernetes upgrade docs require a verified PostgreSQL backup,
  stopped mixed-version backends, one successful schema initializer, and backup
  restore before binary downgrade; they explain SQLite's automatic pre-upgrade
  snapshot.
- All new copy is localized in English, pseudo, Portuguese, and Simplified
  Chinese catalogs.

## TDD Sequence

1. RED: extend desktop and mobile rendered-component tests for the retained-
   workspace copy and absence of uncommitted/unpushed warnings.
2. RED: add a managed desktop E2E that writes an uncommitted marker, deletes the
   only session through visible UI, verifies zero sessions plus directory/Git
   registration/marker preservation, and creates a replacement session that
   observes the marker.
3. RED: add the two-session shared-workspace case and verify the remaining
   session continues to operate.
4. RED: extend the mobile E2E to assert the same confirmation contract from the
   native session-actions sheet.
5. GREEN: update localized dialog copy without adding new warning-fetch state.
6. GREEN: update public session documentation.
7. GREEN: update the existing operations/Kubernetes upgrade and rollback guides
   with the one-time ownership-schema cutover requirements.
8. REFACTOR: share only translation keys or test helpers that reduce duplication;
   retain the existing desktop and mobile compositions.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- components/task/session-tab-menu.test.tsx components/task/mobile/mobile-sessions-section.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm run i18n:ratchet && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/session/multi-session-ux.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-deletion.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/components/task/session-tab-menu.tsx`
- `apps/web/components/task/session-tab-menu.test.tsx`
- `apps/web/components/task/mobile/mobile-sessions-section.tsx`
- `apps/web/components/task/mobile/mobile-sessions-section.test.tsx`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pseudo/task.json`
- `apps/web/src/locales/pt-pt/task.json`
- `apps/web/src/locales/zh-cn/task.json`
- `apps/web/e2e/tests/session/multi-session-ux.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-deletion.spec.ts`
- `docs/public/sessions-and-review.md`
- `docs/public/operations.md`
- `docs/public/k8s.md`

## Dependencies

- Task 03 completes the backend ownership and durable task-cleanup contract.

## Inputs

- Current desktop `DeleteSessionDialog` and mobile
  `DeleteSessionConfirmDialog` compositions.
- Existing task archive/delete cleanup summaries and warnings.
- Existing session Playwright fixtures and Git helpers.

## Output contract

Report desktop/mobile rendered test results, the exact user-visible contract,
zero-session and shared-session E2E evidence, Git registration and uncommitted
marker results, replacement-session reuse evidence, the documented
SQLite/PostgreSQL upgrade and downgrade procedure, i18n/typecheck/docs results,
and screenshots or traces for any failed mobile state.
