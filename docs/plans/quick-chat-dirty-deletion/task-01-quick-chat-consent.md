---
id: "01-quick-chat-consent"
title: "Enable explicit Quick Chat worktree discard"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-CLEANUP-CONFIRMATION-001
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-DELETE-RECOVERY-001
  - REQ-TASKS-DELETE-RECOVERY-002
acceptance_criteria:
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.12
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.13
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.14
  - AC-TASKS-RUNTIME-CLEANUP-001.9
  - AC-TASKS-DELETE-RECOVERY-001.6
  - AC-TASKS-DELETE-RECOVERY-001.7
  - AC-TASKS-DELETE-RECOVERY-002.7
system_design:
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
  - ../../specs/tasks/system-design/dirty-worktree-deletion.md
  - ../../specs/tasks/system-design/task-delete-recovery.md
---

# Task 01: Enable explicit Quick Chat worktree discard

## Summary

Wire the existing task deletion confirmation and its consent-bound preview through Quick Chat conversation and retained-creation deletion. Prove a user can discard a dirty owned worktree explicitly while refusal or failure leaves the chat intact.

## In scope

- Adapt both backed-chat entry points to preflight, discard, unknown-inspection and keep-files behavior, preserving target and selected options through the deletion call.
- Preserve local-only setup close, success navigation, error handling, stale-response and consent reset. Add focused tests and desktop/phone browser regressions using isolated repository-backed worktrees.
- Add or reuse localized copy in all supported catalogs if necessary, following the existing i18n checks and generation rules.

## Out of scope

- Changing backend deletion safeguards, changing shared helper semantics, expiration or filesystem-cleanup diagnosis for unrelated errors.

## Acceptance

- Dirty preflight requires a user-selected discard option; the matching ticket and exact options reach the deletion endpoint from both Quick Chat entry points. Clean, unknown, failed and stale inspection follow shared task deletion rules.
- Cancel, missing consent, stale or dirty conflict, and deletion failure preserve the chat and any unconsented local changes; successful deletion removes the relevant tab or pending creation. Local setup without a task closes without a backend request.
- Desktop and phone tests exercise real dirty repository worktrees, including cancellation and successful explicit discard; phone confirms reachable controls and containment.

## ASCII UI preview

UI-01/UI-02: Backed Quick Chat delete, dirty inspection. See the [complete preview](plan.md#ascii-ui-preview). AC-UI-TASK-CLEANUP-CONFIRMATION-001.12-.14.

```text
Delete Quick Chat
Conversation, task and worktree will be removed.
[ ] Discard tracked and untracked local changes
[Cancel]  [Delete: disabled until selected]
```

Phone uses the centered inset alert with a scrolling body and stacked reachable footer actions. Clean inspection hides the selection; unavailable and unknown inspection use the existing task recovery states.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/quick-chat/quick-chat-delete-dialog.test.tsx components/quick-chat/use-quick-chat-modal.test.ts components/quick-chat/use-quick-chat-creation.test.tsx components/task/task-delete-confirm-dialog.test.tsx hooks/use-task-delete-preflight.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=7168 pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/backend && go test -tags fts5 ./internal/task/service -run '^(TestDeleteTaskRejectsDirtyWorktreeBeforeMutation|TestDeleteTaskWithDiscardConsentPersistsAndCleansDirtyWorktree|TestTaskDeletePreflightUsesExactScopeAndReportsDirtyWorktrees)$' -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/quick-chat.spec.ts -- --grep 'deletes dirty repository quick chat with consent')
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-quick-chat-repository.spec.ts -- --grep 'deletes dirty repository quick chat with consent')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/quick-chat/quick-chat-delete-dialog.tsx` and its test
- `apps/web/components/quick-chat/quick-chat-modal.tsx`, `use-quick-chat-modal.ts`, `use-quick-chat-close-actions.ts` and related tests
- `apps/web/components/quick-chat/quick-chat-creation-status.tsx`, `use-quick-chat-creation.ts`, `use-quick-chat-agent-selection.ts` and related tests
- `apps/web/lib/api/task-delete-errors.ts` for preview conflict classification
- `apps/web/components/task/task-delete-confirm-dialog.tsx` if shared confirmation reuse needs an adapter
- `apps/web/e2e/tests/chat/quick-chat.spec.ts`, `mobile-quick-chat-repository.spec.ts`
- `apps/web/src/locales/` only if new copy is required

## Dependencies

None.

## Risks

A stale preflight or changed consent must not be sent with a mismatched ticket. Do not infer consent from the existing generic delete confirmation. Retain mode must never be selected implicitly.

## Parallelism

Sequential.

## Inputs

- [Confirmation design](../../specs/ui/system-design/confirmation-warning-hierarchy.md)
- [Dirty worktree deletion](../../specs/tasks/system-design/dirty-worktree-deletion.md)
- [Task deletion recovery](../../specs/tasks/system-design/task-delete-recovery.md)
- [Plan and preview](plan.md)

## Results

Completed. The focused Vitest suite passed (92 tests); the task deletion
service tests, TypeScript check with a 7 GiB Node heap, changed-file ESLint,
localization checks, public-doc validation, and specification checks passed.
The desktop and phone E2E scenarios passed against a fresh production build;
the phone geometry assertion waits for dialog animations to settle. The first
typecheck exceeded Node's default heap and the first phone geometry rerun
measured during animation; both passed after those issues were addressed.
The review correction exercised the actual setup-error reporter with stale
preview and dirty-worktree failures; the focused suite, desktop/phone E2E,
typecheck, localization ratchet, and changed-file lint passed again.
