---
id: "02-backup-row-action-guidance"
title: "Describe backup row actions"
status: done
wave: 2
depends_on:
  - "01-resolved-backup-directory"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-BACKUP-GUIDANCE-001
acceptance_criteria:
  - AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.3
  - AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.4
  - AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.5
system_design:
  - ../../specs/system-page/system-design/backup-location-actions.md
---

# Task 02: Describe Backup Row Actions

## Summary

Add localized operation-only hover and focus tooltips to each backup row action.
Keep the same actions visible and directly usable on coarse pointers.

## In scope

- Add Tooltip wrappers for Download, Restore, and Delete.
- Use localized operation labels as tooltip content and keep snapshot-specific accessible names.
- Keep the download control as an anchor.
- Add 44-pixel coarse-pointer targets.
- Add component, desktop E2E, and mobile E2E evidence.

## Out of scope

- Add labels inside the desktop table cells.
- Move actions into a mobile overflow menu.
- Change action handlers, dialogs, or authorization.

## Acceptance

- Hover or keyboard focus shows only the correct operation in the tooltip.
- Each button keeps an accessible name that identifies its snapshot and keeps its existing action.
- Coarse-pointer targets are at least 44 pixels and cause no horizontal page scroll.

## Verification

```bash
pnpm --filter @kandev/web exec vitest run components/settings/system/backups-table.test.tsx
pnpm --filter @kandev/web e2e:run tests/system/backups-page.spec.ts
pnpm --filter @kandev/web e2e:run --project mobile-chrome tests/auth/mobile-system-data-storage-member-gating.spec.ts
pnpm --filter @kandev/web run i18n:check
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the pnpm commands from `apps`.
Run the final two commands from the repository root.

## Files likely touched

- `apps/web/components/settings/system/backups-table.tsx`
- `apps/web/components/settings/system/backups-table.test.tsx`
- `apps/web/e2e/tests/system/backups-page.spec.ts`
- `apps/web/e2e/tests/auth/mobile-system-data-storage-member-gating.spec.ts`

## Dependencies

Task 01 establishes the final Backups page description and desktop E2E baseline.

## Risks

- Nested slots can change the download anchor semantics.
- A global tooltip locator can match a closed portal.
- Touch-sized controls can widen the table on small screens.

## Parallelism

`sequential`

## Inputs

- `REQ-SYSTEM-PAGE-BACKUP-GUIDANCE-001`
- The Row action guidance and Responsive behavior sections in the system design.
- `apps/web/components/task/chat/queued-ghost-row-actions.tsx`
- The tooltip selector rules in `.agents/skills/e2e/SKILL.md`.

## Results

Passed.

- `pnpm --filter @kandev/web exec vitest run components/settings/system/backups-table.test.tsx` passed (4 tests).
- `pnpm --filter @kandev/web e2e:run tests/system/backups-page.spec.ts` passed (3 Chromium tests, including all three hover tooltips).
- `pnpm --filter @kandev/web e2e:run --project mobile-chrome tests/auth/mobile-system-data-storage-member-gating.spec.ts` passed (2 mobile Chromium tests, including coarse-pointer target sizing and containment).
- `pnpm --filter @kandev/web run i18n:check` passed with the repository's existing advisory orphan-catalog warnings.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
- Tooltip refinement passed: visible tooltips now contain only the localized operation, while accessible names retain the snapshot name.
