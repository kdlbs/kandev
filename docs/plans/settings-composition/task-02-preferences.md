---
id: "02-preferences"
title: "Remaining preferences"
status: complete
wave: 2
depends_on:
  - 01-task-behavior
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
  - REQ-UI-SETTINGS-COMPOSITION-003
  - REQ-UI-SETTINGS-COMPOSITION-004
acceptance_criteria:
  - AC-UI-SETTINGS-COMPOSITION-001.1
  - AC-UI-SETTINGS-COMPOSITION-001.2
  - AC-UI-SETTINGS-COMPOSITION-001.3
  - AC-UI-SETTINGS-COMPOSITION-001.4
  - AC-UI-SETTINGS-COMPOSITION-001.5
  - AC-UI-SETTINGS-COMPOSITION-003.1
  - AC-UI-SETTINGS-COMPOSITION-003.2
  - AC-UI-SETTINGS-COMPOSITION-003.3
  - AC-UI-SETTINGS-COMPOSITION-003.4
  - AC-UI-SETTINGS-COMPOSITION-003.5
  - AC-UI-SETTINGS-COMPOSITION-004.1
  - AC-UI-SETTINGS-COMPOSITION-004.2
  - AC-UI-SETTINGS-COMPOSITION-004.3
  - AC-UI-SETTINGS-COMPOSITION-004.4
  - AC-UI-SETTINGS-COMPOSITION-004.5
system_design:
  - ../../specs/ui/system-design/settings-composition.md
---

# Task 02: Remaining preferences

## Summary

Apply the shared composition to Appearance, keyboard shortcuts, notifications, layouts, and terminal/editor settings. Retain their existing tabs, previews, and save behavior.

## In scope

- Migrate every preference-family row in the surface inventory.
- Group simple preferences and normalize compound forms without changing their options.
- Remove duplicate headings and descriptions. Preserve technical previews, credential fields, and immediate permission commands.
- Extend desktop and mobile matrix tests with the preferences tag.

## Out of scope

- Task behavior logic and non-preference routes.
- Rewriting layout editors or terminal content.

## Acceptance

- All preference inventory rows use the shared patterns or record a specialized-layout exception.
- One saved setting per affected page persists after reload, with Reset and existing tabs intact.
- Long descriptions and actions remain contained on desktop, phone, and boundary widths.

## ASCII UI preview

### UI-04 / UI-03: Shared group composition

```text
Page title                         [existing actions/tabs]
One description.
Group heading                      [group action]
+-------------------------------------------------------+
| Label / short description          [control]           |
| Related field or existing resource content             |
| Visible error or scope notice when applicable          |
+-------------------------------------------------------+
[Reset] [Save changes]             only while dirty
```

On phones, group actions and fields stack. Specialized bodies retain their existing mobile interaction.

See the [full labelled previews](plan.md#ascii-ui-preview). Applicable criteria are listed in frontmatter.
Group structure and state visibility are required. ASCII spacing is illustrative.

## Verification

Start from the repository root. Install dependencies once before the first package command.
Add or update tests before changing covered behavior. Compare rendered results with the assigned preview.
New E2E files and family tags are defined in the plan and must exist before running these commands.

```bash
(cd apps/web && pnpm exec vitest run components/settings)
(cd apps/web && pnpm e2e:run e2e/tests/settings/settings-composition.spec.ts -- --grep preferences)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-settings-composition.spec.ts -- --grep preferences)
(cd apps/web && pnpm e2e:run e2e/tests/settings/notifications-type-scale.spec.ts e2e/tests/settings/settings-typography.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-settings-typography.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec eslint components/settings app/settings lib/settings-discovery --max-warnings 0)
git diff --check
```

Also run every existing E2E file whose selectors or assertions this work order changes.
Record its exact command and outcome under Results. Do not weaken assertions or leave changed tests unrun.

## Files likely touched

- `apps/web/components/settings/general-settings.tsx`
- `apps/web/components/settings/notifications-settings*.tsx`
- `apps/web/components/settings/layouts/`
- `apps/web/components/settings/{terminal-editors-settings,terminal-settings,editors-settings,appearance-account-sections,startup-page-settings-card}.tsx`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/settings/settings-composition.spec.ts` (new shared family matrix)
- `apps/web/e2e/tests/settings/mobile-settings-composition.spec.ts` (new shared family matrix)
- Existing component and E2E tests beside the migrated surfaces

## Dependencies

Complete 01-task-behavior first.

## Risks

- Shared wrappers can lose dirty markers, accessible descriptions, discovery targets, or state identity.
- Long translations and coarse-pointer controls can expose clipping despite correct desktop appearance.
- Existing tests can depend on card nesting. Preserve behavioral checks when updating selectors.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/settings-composition.md).
- [System design](../../specs/ui/system-design/settings-composition.md).
- [Surface inventory](surface-inventory.md) and the assigned route imports.
- `apps/web/AGENTS.md`, `/tdd`, `/mobile-parity`, and `/e2e`.
- Existing typography, header tabs, control sizing, and route-save contracts.

## Results

Implemented and verified. Preference callers now use the shared group contract through direct migrations and the `SettingsSection` adapter. Notifications, layouts, and file-editor settings have group chrome; terminal and editor bodies retain their specialized previews and technical geometry. Existing tabs, save contributors, discovery targets, and immediate permission/provider actions remain in their owning components.

- `(cd apps/web && pnpm exec vitest run components/settings lib/settings-discovery/target.test.ts src/settings-routes.test.ts)`: passed, 207 files and 1,400 tests.
- `(cd apps/web && pnpm e2e:run e2e/tests/settings/settings-composition.spec.ts)`: passed, 7 desktop tests, including preferences coverage.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-settings-composition.spec.ts)`: passed, 6 mobile tests, including preferences coverage.
- `(cd apps/web && pnpm e2e:run e2e/tests/settings/notifications-type-scale.spec.ts e2e/tests/integrations/github-workspace-settings.spec.ts -- --grep 'renders the card body|repository scope is saved per workspace')`: passed, 2 focused desktop tests.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-notifications-type-scale.spec.ts e2e/tests/settings/mobile-settings-typography.spec.ts)`: passed as part of the 10-test mobile specialized suite.
- Typecheck, localization checks, ratchet, scoped ESLint, scoped Prettier, and `git diff --check`: passed.

Review remediation on 2026-09-21 removed the extra page card from Notifications
and Terminal and Editors, flattened simple Appearance preference cards into
rows, and updated structural/type-scale assertions to inspect the actual
frameless page composition. Specialized editor, table, and provider bodies
remain inside their owning group frames. The production Appearance Select also
forwards its generated row description to the trigger. The shared section
adapter now has an explicit frame opt-out for content whose owner remains
outside the native settings composition.
