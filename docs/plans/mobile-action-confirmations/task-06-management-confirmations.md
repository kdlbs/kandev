---
id: "06-management-confirmations"
title: "Phone management removal confirmations"
status: done
wave: 2
depends_on:
  - "01-shared-mobile-surfaces"
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-CONFIRMATION-001
  - REQ-UI-MOBILE-CONFIRMATION-002
  - REQ-UI-MOBILE-CONFIRMATION-003
acceptance_criteria:
  - AC-UI-MOBILE-CONFIRMATION-001.1
  - AC-UI-MOBILE-CONFIRMATION-001.2
  - AC-UI-MOBILE-CONFIRMATION-001.3
  - AC-UI-MOBILE-CONFIRMATION-001.4
  - AC-UI-MOBILE-CONFIRMATION-001.5
  - AC-UI-MOBILE-CONFIRMATION-001.7
  - AC-UI-MOBILE-CONFIRMATION-002.1
  - AC-UI-MOBILE-CONFIRMATION-002.2
  - AC-UI-MOBILE-CONFIRMATION-002.3
  - AC-UI-MOBILE-CONFIRMATION-002.6
  - AC-UI-MOBILE-CONFIRMATION-003.1
  - AC-UI-MOBILE-CONFIRMATION-003.2
  - AC-UI-MOBILE-CONFIRMATION-003.5
  - AC-UI-MOBILE-CONFIRMATION-003.6
system_design:
  - ../../specs/ui/system-design/mobile-action-confirmations.md
---

# Task 06: Phone Management Removal Confirmations

## Summary

Finish the current inline-consumer inventory across integration settings,
watches, secrets, users, plugin uninstall and workflow-sync removal. Preserve
the originating page or form and each owner's existing failure behavior.

## In scope

- Use standalone compact sheets from ordinary settings cards and stable menu
  owners, with named targets and current warning copy.
- Use a focused step in the existing workflow-sync form dialog; Cancel and a
  retryable failure preserve the form's values and current error handling.
- Extend component/mobile tests and account for every remaining phone use of
  `InlineConfirmActions` in the manifest. Retain explicit full-alert exceptions.

## Out of scope

Credential/provider health, watchers, user permission policies, plugin runtime
or uninstall behavior, workflow reconciliation, full system alerts, and APIs.

## Acceptance

1. Each listed removal reaches one named confirmation surface; opening and
   cancelling never mutate or disturb the origin's state.
2. Confirmation respects pending/eligibility rules, dispatches once, and
   preserves the existing success, error, rollback and retry behavior.
3. Mobile tests cover standalone and existing-dialog composition, plus actual
   removals; remaining inline usage is non-phone or an explicitly scoped
   existing full-alert path, documented in the work-order results.

## Verification

Use `/tdd` and `/e2e`. Extend the existing provider/management component tests
and add the new focused watcher/Jira tests where coverage is absent. Add
`mobile-management-confirmations.spec.ts` for user/plugin/workflow-sync outcomes.

```bash
cd apps/web
pnpm exec vitest run components/watches/watcher-delete-action.test.tsx components/jira/jira-action-bar.test.tsx components/linear/linear-settings.test.tsx components/azure-devops/azure-devops-settings.test.tsx components/sentry/sentry-instance-card.test.tsx components/settings/secrets-list-item-row.test.tsx components/settings/system/users-table.test.tsx components/settings/plugins/uninstall-plugin-dialog.test.tsx components/settings/workflow-sync-dialog.test.tsx
pnpm e2e:run --project mobile-chrome tests/integrations/mobile-integration-remove-confirmations.spec.ts tests/integrations/mobile-watcher-delete-confirmations.spec.ts tests/settings/mobile-secrets-delete.spec.ts tests/settings/mobile-management-confirmations.spec.ts
pnpm e2e:run --project chromium tests/integrations/integration-remove-confirmations.spec.ts tests/integrations/watcher-delete-confirmations.spec.ts tests/settings/secrets-delete.spec.ts
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

Run this inventory check from the repository root and record dispositions:

```bash
rg -n 'InlineConfirmActions|ActionConfirmPopover' apps/web/components --glob '!*.test.*'
```

Keep E2E seeding on disposable fixture data. Extend existing mobile tests with
visible successful removals and persisted-state assertions where applicable.
Inspect a standalone integration confirmation and the workflow form step.

## Files likely touched

- Both 06 inventory rows in `plan.md`, their stable parent surfaces, and component tests.
- Existing mobile integration/watch/secret specs; new watcher/Jira unit tests and `apps/web/e2e/tests/settings/mobile-management-confirmations.spec.ts`.
- Relevant locale catalogs; no provider/auth/backend or plugin SDK files.

## Dependencies

01 supplies standalone and existing-dialog hosting.

## Risks

Workflow-sync removal currently restores inline confirmation after a rejected
promise; a blanket close-on-submit policy would lose its retry behavior.
Settings sources vary between menus and persistent cards, so handoff must be
explicit. Secret values must never enter confirmation titles or test captures.

## Parallelism

`sequential`

## Inputs

Mobile design sections on routing, other consumers, and failure handling;
current provider/settings components and their cancellation/removal tests.

## Results

Implemented named phone sheets for all four watcher types, Jira/Linear/Azure
DevOps connection removal, Sentry instances, plugin uninstall, secret deletion,
and user role/status changes. Plugin row/detail and secret/user action controls
remain mounted. Existing permissions, last-administrator protection, secret
redaction, and full in-use conflict dialogs remain authoritative.

Workflow-sync removal uses a step in its actual form dialog. Its explicit
await-with-retry policy disables duplicate submission, retains the form on
failure, and leaves feedback with the existing controller. Target changes,
closure and unmount invalidate stale completion callbacks. A behavioral RED
test exposed an unmounted completion dismissing a replacement dialog; the
generation cleanup fixes that case.

Verification:

- Behavioral RED tests preceded the new adapters and retry policy. All 94
  focused component cases passed across 13 files, including the shared host,
  plugin row, secret conflict and stale-completion regressions.
- Ten mobile browser scenarios passed: four integration removals, watcher
  deletion, plugin row/detail uninstall, workflow-sync Cancel/failure/retry,
  simple secret deletion, secret conflict, and user self-action guards.
- User coverage extends `auth/mobile-users-self-actions.spec.ts` rather than
  duplicating its authenticated setup in the new management test file.
- Seven desktop provider/watch/secret cases passed in the combined 14-case
  compatibility run. Existing popovers and full conflict flows remain usable.
- Inspected the standalone Sentry sheet and hosted workflow removal capture.
  Browser assertions cover reachable 48px actions, one modal, Cancel focus,
  preserved draft, successful mutation and persistence where supported.
- Changed production ESLint, typecheck, build, i18n checks/ratchet, public-doc
  validation, and specification lint passed. No new translation keys or API,
  backend, credential, permission or plugin-runtime changes were needed.

Inventory disposition:

- Every `InlineConfirmActions` consumer in the manifest now sits behind the
  phone adapter or an explicit non-phone branch. This includes archive,
  session/terminal pickers, content, saved artifacts, and management consumers.
- `CloseTerminalConfirmPopover` remains in Dockview/terminal reopen controls
  and the tablet right panel; phones use the migrated terminal picker.
- `DeleteSessionPopover` remains in Dockview and the non-phone Kanban preview;
  phones use the migrated session picker. Kanban skips preview below 768px.
- `changes-panel-dialogs.tsx` retains its existing single-file Git-discard
  popover and bulk discard alert. This separate loss-of-work consent flow is
  not an inline consumer and is outside the adopted inventory.
- Existing full task-delete/discard/type-to-confirm, full detach and plan
  preview alerts, profile/secret conflict alerts, and system maintenance
  dialogs are intentionally unchanged. Shared legacy primitives are unchanged.
