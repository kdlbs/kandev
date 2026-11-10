---
id: "01-compact-notification-preferences"
title: "Compact notification preferences"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001
acceptance_criteria:
  - AC-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001.1
  - AC-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001.2
  - AC-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001.3
  - AC-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001.4
  - AC-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001.5
  - AC-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001.6
system_design:
  - ../../specs/platform/system-design/notification-settings-layout.md
---

# Task 01: Compact notification preferences

## Summary

Deliver the accepted compact notification layout on desktop and phone.
Preserve existing domain state, permission feedback, search targets, and route Save behavior.

## In scope

- Page introduction, the two device preference rows, and sound placement before push.
- Always-visible sound selector and preview, disabled while sound is off.
- Local responsive presentation, translated copy, dirty feedback, and accessibility.
- Targeted component and browser tests listed in the plan, including existing affected suites.

## Out of scope

- Provider APIs, event delivery, playback policy, storage migrations, or global settings changes.
- Push, provider, webhook, or event-table redesigns.

## Acceptance

1. The rendered desktop and phone surfaces match UI-01 through UI-04 and retain translated copy and settings-search registration.
2. Sound controls keep a stable layout, preserve the chosen preset, and obey the existing Save and Reset behavior.
3. Permission actions and conditional warnings retain their behavior, and every targeted verification command passes.

## ASCII UI preview

Entry: Settings > Preferences > Notifications.
See the [complete preview and annotations](plan.md#ascii-ui-preview).

UI-01: Desktop, sound enabled, AC .1-.3 and .6.

```text
Notifications
Choose how this device alerts you to agent turns, questions, and Office events.
+---------------------------------------------------------------------+
| Desktop notifications                    [Enable] [Refresh] [Test]  |
+---------------------------------------------------------------------+
| Notification sound                [ON]  [Plim v]  [Play]             |
+---------------------------------------------------------------------+
Push notifications ...
```

UI-02: Desktop, sound disabled and permission denied, AC .3 and .6.

```text
| Desktop notifications                    [Enable] [Refresh] [Test]  |
| Existing localized permission warning, only when needed.             |
| Notification sound                [OFF] [Plim v]  [Play]             |
                                         disabled  disabled
```

UI-03: Phone, sound enabled, AC .3-.6.

```text
+--------------------------------------+
| Desktop notifications                |
| [Enable]  [Refresh]  [Test]           |
+--------------------------------------+
| Notification sound            [ON]   |
| [Plim                       v] [Play]|
+--------------------------------------+
```

UI-04: Phone, sound disabled and permission denied, AC .3-.6.

```text
| Desktop notifications                |
| [Enable]  [Refresh]  [Test]           |
| Existing localized warning wraps.    |
| Notification sound            [OFF]  |
| [Plim                       v] [Play]|
  selector and preview disabled
```

Grouping and control order are required; spacing and example words are illustrative.
Keep one page scroll owner, the existing Save surface, and phone targets of at least 44px.
Toggle changes must not change the number of control rows.

## Verification

Use TDD for changed sound logic and permission/action regressions.
First prove disabled-control behavior fails against the current implementation.
Then deliver the minimal layout and run the commands from the repository root.
If workspace dependencies are absent, first use `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run components/settings/notification-sound-section.test.tsx components/settings/notification-permission-section.test.tsx components/settings/notifications-settings.test.tsx components/settings/notification-save-drafts.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/settings/notification-sound-section.tsx components/settings/notification-permission-section.tsx components/settings/notifications-settings.tsx e2e/tests/settings/notifications-type-scale.spec.ts e2e/tests/settings/mobile-notifications-type-scale.spec.ts e2e/tests/settings/notifications-type-scale-helpers.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/notifications-type-scale.spec.ts tests/settings/notification-save-drafts.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-notifications-type-scale.spec.ts tests/settings/mobile-notification-save-drafts.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If the local row helper is created, also run:

```bash
(cd apps/web && pnpm exec eslint components/settings/notification-preference-row.tsx)
```

After translation edits, generate Traditional Chinese before running i18n checks:

```bash
(cd apps/web && pnpm run i18n:zh-hant)
```

Record final commands, results, screenshots, and any unresolved preview differences.

## Files likely touched

- `apps/web/components/settings/notifications-settings.tsx`
- `apps/web/components/settings/notification-permission-section.tsx`
- `apps/web/components/settings/notification-sound-section.tsx`
- `apps/web/components/settings/notification-preference-row.tsx` (new local presentation helper if needed)
- Their existing test files and `notification-save-drafts.test.tsx`.
- `apps/web/e2e/tests/settings/notifications-type-scale.spec.ts`
- `apps/web/e2e/tests/settings/mobile-notifications-type-scale.spec.ts`
- `apps/web/e2e/tests/settings/notifications-type-scale-helpers.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/settings.json`
- This plan, work order, and paired spec lifecycle fields after successful implementation.

## Dependencies

None. Existing save coordination, notification providers, and settings primitives are already shipped dependencies.

## Risks

Replacing wrappers can lose dirty feedback or discovery registration.
Existing E2E heading/body selectors need intentional replacement with equivalent behavior assertions.
Touch sizing must remain scoped to phones and coarse pointers.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/notification-settings-layout.md), AC .1 through .6.
- [System design](../../specs/platform/system-design/notification-settings-layout.md).
- [Combined plan](plan.md), including test and E2E mappings.
- Existing notification permission, sound, Save, and type-scale tests.
- `apps/web/AGENTS.md`, `/tdd`, `/e2e`, and `/mobile-parity`.

## Results

Implementation completed on 2026-11-03.

- Added `notification-preference-row.tsx` and composed both device preferences as compact bordered rows with discovery registration, dirty feedback, stable sound controls, and phone-first ordering.
- Moved Notification Sound before Push notifications, replaced repeated descriptions with one page introduction, and localized the new copy in every catalog.
- Added component coverage for the stable disabled sound controls, preset retention, Save and Reset, one introduction/order, and both settings-search targets.
- Extended both type-scale E2E specs with row geometry, the 767/768 boundary, 44px phone targets, Reset, draft retention across resizing, and Save/reload.
- Updated the permission feedback design to point layout authority at the new compact-row design.
- Review remediation on 2026-11-03: the phone sound selector now fills the row beside the preview, the mobile E2E asserts that row fill, and the unrelated Traditional Chinese automation-copy changes were reverted. The review pass itself did not run builds or tests.
- PR preparation reran the four targeted component suites (47 passed) and both notification type-scale E2E specs with `CAPTURE_PR_ASSETS=true` (4 desktop and 3 mobile tests passed), producing fresh PR screenshots.
- PR fixup kept desktop rows on one line, added the disabled-preview explanation to every catalog, restored measured normal-row bounds, and tightened the same-row overlap check. The same component suites and both type-scale E2E specs passed with fresh captures.
- CI fixup aligned the settings-composition E2E with four shared groups plus two compact preference rows; the focused spec passed (8 tests).

Verification:

```text
pnpm exec vitest run <four notification settings suites>   4 files, 47 tests passed
NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck  passed
pnpm exec eslint <changed source, tests, and E2E files>    passed, 0 warnings
pnpm run i18n:check                                        passed
pnpm run i18n:ratchet                                      passed
pnpm e2e:run --project chromium tests/settings/notifications-type-scale.spec.ts
                                                           4 passed
pnpm e2e:run --project mobile-chrome tests/settings/mobile-notifications-type-scale.spec.ts
                                                           3 passed
pnpm e2e:run --project chromium tests/settings/notification-save-drafts.spec.ts
                                                           1 passed
pnpm e2e:run --project mobile-chrome tests/settings/mobile-notification-save-drafts.spec.ts
                                                           1 passed
python3 scripts/list-docs.py validate                      passed
python3 scripts/lint-spec-files.py --all                   passed
git diff --check                                           clean
```

No unresolved preview differences remain. The default 4 GB Node heap OOMs during the rich-output catalog check on this host; the recorded typecheck used the 8 GB heap override.
