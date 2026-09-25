---
id: "05-system-account"
title: "System, account, and plugin settings"
status: complete
wave: 5
depends_on: 
  - 04-workspace-integration
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

# Task 05: System, account, and plugin settings

## Summary

Complete the remaining first-party settings migration and reconcile the surface inventory. Keep system operations, account permissions, and plugin-owned content boundaries intact.

## In scope

- Migrate System, Account, users, organizations, units, plugin listing/detail, and host-owned plugin settings chrome.
- Preserve feature-toggle metadata, locks, restart actions, and existing runtime flag behavior. This is presentation only.
- Retain diagnostic and table layouts with shared surrounding chrome.
- Extend the rendered matrix with the system account tag. Record final family coverage and reconcile related typography-plan evidence.

## Out of scope

- Runtime flag registration or rollout, plugin SDK changes, and new system operations.

## Acceptance

- Every inventory row has migration or retained-layout evidence, including dynamic route forms.
- System and account actions retain their permissions, confirmation, and persistence behavior.
- The complete desktop/mobile matrix passes and the durable package records exact results without claiming historical unfinished tasks are done.

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
(cd apps/web && pnpm exec vitest run components/settings src/settings-routes.test.ts)
(cd apps/web && pnpm e2e:run e2e/tests/settings/settings-composition.spec.ts e2e/tests/settings/settings-typography.spec.ts e2e/tests/settings/settings-manual-save.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-settings-composition.spec.ts e2e/tests/settings/mobile-settings-typography.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec eslint components/settings app/settings lib/settings-discovery --max-warnings 0)
git diff --check
```

Also run every existing E2E file whose selectors or assertions this work order changes.
Record its exact command and outcome under Results. Do not weaken assertions or leave changed tests unrun.

## Files likely touched

- `apps/web/components/settings/system/`
- `apps/web/components/settings/account/`
- `apps/web/components/settings/units/`
- `apps/web/components/settings/plugins/`
- `apps/web/app/settings/plugins/`
- `apps/web/src/settings-routes.tsx (presentation wrappers only)`
- `apps/web/src/locales/`
- `docs/plans/settings-composition/ and related typography records`
- `apps/web/e2e/tests/settings/settings-composition.spec.ts` (new shared family matrix)
- `apps/web/e2e/tests/settings/mobile-settings-composition.spec.ts` (new shared family matrix)
- Existing component and E2E tests beside the migrated surfaces

## Dependencies

Complete 04-workspace-integration first.

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

Implemented and verified. System data, logs, About, users, organizations, units, account tokens, plugin listings, and plugin detail surfaces now use shared group chrome around their existing diagnostics, tables, forms, and plugin-owned bodies. Permissions, locks, restart actions, confirmation flows, and discovery targets remain owned by the existing domain components.

- `(cd apps/web && pnpm exec vitest run components/settings lib/settings-discovery/target.test.ts src/settings-routes.test.ts)`: passed, 207 files and 1,389 tests.
- `(cd apps/web && pnpm e2e:run e2e/tests/settings/settings-composition.spec.ts)`: passed, 6 desktop tests, including system/account coverage.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-settings-composition.spec.ts)`: passed, 5 mobile tests, including system/account coverage.
- `python3 scripts/list-docs.py validate`: passed (294 decisions, 1,067 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Typecheck, localization checks, ratchet, scoped ESLint, scoped Prettier, and `git diff --check`: passed.

Fixup remediation preserves plugin-owned integration settings as frameless
content through the explicit `SettingsSection` frame opt-out. The route test
verifies that the plugin surface is rendered without a native group card.
