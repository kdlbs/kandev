---
id: "01-add-confirmation"
title: "Add automation deletion confirmation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.9
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.10
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.11
system_design:
  - ../../specs/office/system-design/automations-settings-01.md
  - ../../specs/office/system-design/automations-settings-02.md
---

# Task 01: Add automation deletion confirmation

## Summary

Place a shared, localized alert-dialog confirmation between both Settings
automation deletion controls and the existing delete mutation. Preserve the
list-row removal and editor redirect outcomes, and prove the phone path with a
mobile Playwright flow.

## In scope

- Shared automation deletion confirmation component using the existing alert
  dialog primitives.
- List-page selected-automation state and editor confirmation trigger.
- Five-locale dialog title and warning copy.
- Desktop and mobile E2E coverage for cancellation and confirmation.

## Out of scope

- Backend/API changes.
- Automation run or trigger deletion behavior.
- New settings save-coordinator behavior.

## Acceptance

- Both list-row and editor deletion actions open the named confirmation before
  any delete mutation; cancel or dismissal leaves the automation in place.
- Confirming performs the existing deletion flow once, removes the list row or
  returns the editor to the automation list, and shows the same result on phone.
- The phone dialog remains viewport-contained and its Cancel/Delete controls
  have at least 44px active hitbox height.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/automations-settings.spec.ts -- --grep "delete automation")
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-automations-settings.spec.ts)
(python3 scripts/lint-spec-files.test.py)
(python3 scripts/lint-spec-files.py --all)
(git diff --check -- docs/specs docs/plans)
```

## Files likely touched

- `apps/web/components/automations/automation-delete-confirm-dialog.tsx`
- `apps/web/components/automations/automations-list-page.tsx`
- `apps/web/components/automations/automations-table.tsx`
- `apps/web/components/automations/automation-editor.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/automations.json`
- `apps/web/src/locales/pseudo/automations.json`
- `apps/web/e2e/pages/automations-page.ts`
- `apps/web/e2e/tests/automations-settings.spec.ts`
- `apps/web/e2e/tests/settings/mobile-automations-settings.spec.ts`

## Dependencies

None.

## Risks

- A controlled dialog must not leave stale selected automation state after a
  cancel, successful delete, or live list refresh.
- The mobile project only discovers files named `mobile-*.spec.ts`.

## Parallelism

`sequential`

## Inputs

- `docs/specs/office/requirements/automations-settings.md`, AC 001.9-001.11.
- `docs/specs/office/system-design/automations-settings-01.md`, Deletion confirmation.
- Existing `quick-chat-delete-dialog.tsx` and settings mobile deletion coverage.

## Results

- Added the shared AlertDialog confirmation and wired it to list-row and editor
  deletion controls without changing the existing delete API or redirect.
- Added localized title and permanent-deletion warning copy to all supported
  catalogs, including pseudo-locale coverage.
- Added desktop cancellation/confirmation coverage and mobile viewport,
  overflow, and touch-target coverage.
- Passed `pnpm --filter @kandev/web run typecheck`.
- Passed `pnpm --filter @kandev/web run lint`.
- Passed `pnpm --filter @kandev/web run i18n:check`.
- Passed focused Chromium E2E: 2 tests.
- Passed focused mobile Chrome E2E: 1 test.
- Added unit regression coverage for failed deletion keeping the confirmation
  open, and for the dialog not closing before the async confirmation settles.
- Passed `python3 scripts/lint-spec-files.test.py` and
  `python3 scripts/lint-spec-files.py --all`.
- Passed `git diff --check`.
