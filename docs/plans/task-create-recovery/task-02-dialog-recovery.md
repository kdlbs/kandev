---
id: "02-dialog-recovery"
title: "Integrate recovery into the native dialog"
status: complete
wave: 2
depends_on:
  - "01-native-recovery-contract"
plan: "plan.md"
requirements:
  - REQ-TASKS-CREATE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-CREATE-RECOVERY-001.1
  - AC-TASKS-CREATE-RECOVERY-001.2
  - AC-TASKS-CREATE-RECOVERY-001.3
  - AC-TASKS-CREATE-RECOVERY-001.4
  - AC-TASKS-CREATE-RECOVERY-001.5
  - AC-TASKS-CREATE-RECOVERY-001.7
  - AC-TASKS-CREATE-RECOVERY-001.8
  - AC-TASKS-CREATE-RECOVERY-001.9
  - AC-TASKS-CREATE-RECOVERY-001.10
  - AC-TASKS-CREATE-RECOVERY-001.11
  - AC-TASKS-CREATE-RECOVERY-001.12
  - AC-TASKS-CREATE-RECOVERY-001.13
  - AC-TASKS-CREATE-RECOVERY-001.14
system_design:
  - ../../specs/tasks/system-design/task-create-recovery.md
---

# Task 02: Integrate recovery into the native dialog

## Summary

Route native task creation through the receipt owner and expose explicit recovery.
Preserve start intent, draft ownership, and the task identity before any presentation effects.
Keep existing custom transports and non-create modes on their current paths.

## In scope

- Both native creation branches in `useTaskSubmitHandlers`, including Start task, plan mode, and create-only.
- One guard across button, keyboard, and composer actions before asynchronous validation or consent.
- Same-key fresh-branch re-consent and existing stale-policy refresh behavior.
- Scoped receipt restoration before enabling create, including prefilled native dialogs.
- Callback-attempt persistence before `onSuccess` and task identity before settings/navigation effects.
- Incomplete and unavailable task states, missing-session hydration, and guarded Open task navigation.
- One inline status region and responsive recovery footer in the existing native dialog.
- Close preserves unresolved receipts. Successful cleanup preserves newer text and attachment edits.
- Seven-language copy, semantic status announcements, and the existing Escape/focus contract.

## Out of scope

- Plugin transport identity negotiation or plugin host SDK changes.
- Quick Chat, Edit Task, New Agent/additional-session behavior changes.
- Session preparation, launch recovery, duplicate message submission, or automatic create retries.
- A new dialog layout, nested phone sheet, or global control-size change.

## Acceptance

1. The native lost-response regression proves one task and captured start intent across repeated actions, then permits a distinct independent submission.
2. Accepted task identity survives a throwing callback, missing session hydration, dialog closure, workspace changes, and newer edits without a fresh create.
3. Rendered recovery matches UI-01/UI-02 on desktop and phones, with localized accessible actions and preserved existing dialog modes.

## ASCII UI preview

Use [UI-01 and UI-02 in the plan](plan.md#ascii-ui-preview).
The closest phone exemplar is the existing full-height Create Task dialog.
Keep its draft body scroll owner, header, and safe-area footer.

```text
UI-01 desktop
[Draft and selectors]
Task creation is unconfirmed.
Retry uses the earlier submission. Changes stay in this draft.
[Close] [Retry submission] [Check status]

UI-01 phone, fixed footer beneath the scrolling body
[Check status    ]
[Retry submission]
[Close           ]

UI-02, known incomplete task
This task exists. Creation is still in progress.
[Close] [Check status] [Open task]
```

Phone UI-02 stacks Open task, Check status, and Close.
Recovery controls need at least 44px phone hit areas. Ordinary desktop buttons remain 28px.
The English text is illustrative and must use translated keys.
These structural views map to criteria .5, .8, .9, .11, .12, and .13.

## TDD sequence

1. Add `retains one native task when creation succeeds but the browser response fails` before dialog wiring.
2. Exercise the default native transport, not the optional custom `createTask` override.
3. The fake transport records its allocated task before rejecting the first browser response.
4. Retry through the actual submit handler and record the two-task or missing-key RED result.
5. Add cases for same-tick actions, callback failure, newer edits, scope changes, queued tasks, and native creation modes.
6. Wire the receipt and recovery surface, then run the exact checks below.

The hook tests from task 01 do not replace this integration RED gate.
An assertion that fails only because a new test ID is missing is insufficient.

## Verification

Run from the repository root. Workspace dependencies must already be installed.

```bash
(cd apps/web && pnpm exec vitest run components/task-create-dialog-creation-retry.test.tsx components/task-create-dialog-recovery.test.tsx components/task-create-dialog-submit.test.tsx components/task-create-dialog-fresh-branch-consent.test.tsx components/task-create-dialog-composition.test.tsx components/task-create-dialog-reset-effects.test.ts components/task-create-dialog.test.tsx lib/task-creation-receipt.test.ts)
(cd apps/web && pnpm exec eslint components/task-create-dialog.tsx components/task-create-dialog-submit.tsx components/task-create-dialog-state.ts components/task-create-dialog-setup.ts components/task-create-dialog-types.ts components/task-create-dialog-prop-builders.ts components/task-create-dialog-footer.tsx components/task-create-dialog-recovery.ts components/task-create-dialog-recovery-panel.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

Run `pnpm run i18n:zh-hant` from `apps/web` when deriving the Traditional Chinese catalogs.
Task 03 owns the focused rendered browser and phone geometry evidence.

## Files likely touched

- `apps/web/components/task-create-dialog-submit.tsx`
- `apps/web/components/task-create-dialog-fresh-branch-consent.ts`
- `apps/web/components/task-create-dialog-fresh-branch-consent.test.tsx`
- `apps/web/components/task-create-dialog-state.ts`
- `apps/web/components/task-create-dialog-setup.ts`
- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-prop-builders.ts`
- `apps/web/components/task-create-dialog.tsx`
- `apps/web/components/task-create-dialog-footer.tsx`
- `apps/web/components/task-create-dialog-recovery.ts`
- `apps/web/components/task-create-dialog-recovery-panel.tsx` (new)
- `apps/web/components/task-create-dialog-creation-retry.test.tsx` (new)
- Existing dialog tests listed in Verification, when their native success fixtures require metadata.
- `apps/web/lib/local-storage.ts`, only for compatible draft identity/pointer support.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/task.json`

Do not add business logic separately to mobile wrappers.
Keep the recovery hook and inline status component small instead of expanding the submit module's complexity.

## Dependencies

[Task 01](task-01-native-recovery-contract.md). Its native API, receipt, and rejection evidence must pass first.

## Risks

- `onSuccess` currently precedes draft clearing and can throw after successful server creation.
- Native and custom transports share submit dependencies. An override must not silently receive native lookup semantics.
- A found response can lack the session needed for plan-mode navigation.
- The existing Cancel handler resets form state. Recovery Close must preserve its receipt and newer draft.
- Changing a breakpoint must not reset the receipt or mark an unresolved submission complete.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/task-create-recovery.md).
- [System design](../../specs/tasks/system-design/task-create-recovery.md), acceptance and recovery surface sections.
- [Plan previews](plan.md#ascii-ui-preview).
- Root and web `AGENTS.md`, `/tdd`, `/mobile-parity`, and existing dialog tests.
- `task-create-dialog-task-created.ts`: existing completion callback registration.

## Results

Completed on 2026-10-28.

- Both native creation branches and the create-only branch use the shared recovery guard and captured receipt.
- Start, plan-mode, and create-only outcomes record the task before callbacks, preserve newer edits, and skip automatic acceptance for incomplete tasks.
- The dialog renders one inline status region and responsive recovery actions above the footer. Phone actions stack with 44px targets; desktop keeps the normal 28px controls.
- Close preserves an unresolved receipt, and a same-tab reload restores it without creating anything.
- Added copy for `en`, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, `ja`, and `ko`.
- `components/task-create-dialog-creation-retry.test.tsx` proves lost-response retry, same-tick admission, independent identities, captured values, throwing callbacks, sticky uncertainty, rejection evidence, incomplete settlement, Open task, and missing-session hydration.
- Focused Vitest suites: 11 files and 192 tests passed, including the task-page creation-pending helper suite.
- `(cd apps/web && pnpm run typecheck)`: passed.
- `(cd apps/web && pnpm exec eslint ...)`: passed.
- `(cd apps/web && pnpm run i18n:check)` and `i18n:ratchet`: passed.
- Remediation: every async continuation captures an owner/open-cycle generation before its first await and rechecks it after lookup, consent, hydration, and response awaits. Closing or changing scope lets a late result update only its own retained receipt. In-memory receipts survive close and reopen when storage fails. Accepted handoff compares the live auto-title, description, and attachment descriptors and leaves newer drafts open with Open task. Incomplete or unknown tasks open behind the `creation=pending` read-only gate. Stale branch-policy failures refresh policy choices while preserving sticky uncertainty. Lookup misses preserve an already observed task state.
- Fixup review split receipt storage, owner fencing, resolution, handoff, and native recovery into smaller modules; the new functions meet the frontend limits without new complexity exemptions.
