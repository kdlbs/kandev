---
id: "01-task-behavior"
title: "Shared composition and Task behavior"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
  - REQ-UI-SETTINGS-COMPOSITION-002
  - REQ-UI-SETTINGS-COMPOSITION-003
  - REQ-UI-SETTINGS-COMPOSITION-004
acceptance_criteria:
  - AC-UI-SETTINGS-COMPOSITION-001.1
  - AC-UI-SETTINGS-COMPOSITION-001.2
  - AC-UI-SETTINGS-COMPOSITION-001.3
  - AC-UI-SETTINGS-COMPOSITION-001.4
  - AC-UI-SETTINGS-COMPOSITION-001.5
  - AC-UI-SETTINGS-COMPOSITION-002.1
  - AC-UI-SETTINGS-COMPOSITION-002.2
  - AC-UI-SETTINGS-COMPOSITION-002.3
  - AC-UI-SETTINGS-COMPOSITION-002.4
  - AC-UI-SETTINGS-COMPOSITION-002.5
  - AC-UI-SETTINGS-COMPOSITION-002.6
  - AC-UI-SETTINGS-COMPOSITION-003.1
  - AC-UI-SETTINGS-COMPOSITION-003.2
  - AC-UI-SETTINGS-COMPOSITION-003.3
  - AC-UI-SETTINGS-COMPOSITION-003.4
  - AC-UI-SETTINGS-COMPOSITION-003.5
  - AC-UI-SETTINGS-COMPOSITION-003.6
  - AC-UI-SETTINGS-COMPOSITION-004.1
  - AC-UI-SETTINGS-COMPOSITION-004.2
  - AC-UI-SETTINGS-COMPOSITION-004.3
  - AC-UI-SETTINGS-COMPOSITION-004.4
  - AC-UI-SETTINGS-COMPOSITION-004.5
system_design:
  - ../../specs/ui/system-design/settings-composition.md
---

# Task 01: Shared composition and Task behavior

## Summary

Deliver the shared settings compositions through the complete Task behavior page. Preserve existing values, permissions, saves, and discovery targets.

## In scope

- Add shared groups and rows with aggregate dirty state and the existing typography and control sizes.
- Recompose every Task behavior control into UI-01 and UI-02. Keep runtime mounted while closed and derive its effective summary from existing state owners.
- Reveal native disclosures before discovery focus. Add error revelation and preserve repeated target requests.
- Update labels and all locale catalogs. Update relevant sections in `docs/public/configuration.md`, `operations.md`, `tasks-and-workflows.md`, `automation-and-mcp.md`, `coordination.md`, and `sessions-and-review.md` only where wording changes.
- Add the shipped composition convention to `apps/web/AGENTS.md`.

## Out of scope

- Other settings route migrations.
- Switch-polarity, backend, or plugin API changes.

## Acceptance

- The four groups and runtime states match UI-01 through UI-03, including scope and loading/error summaries.
- Collapsed drafts save/reset correctly, failures reveal controls, and direct/repeated discovery targets focus visible content.
- Desktop and phone tests prove geometry, localization, permissions, and saved behavior.

## ASCII UI preview

### UI-01 / UI-02 / UI-03: Task behavior

```text
Task behavior
Creating and opening tasks [grouped rows]
Conversation and panels    [grouped rows]
Archiving                  [grouped rows]
> Runtime and limits       [effective summary] [scope]
  Expand -> limits, merging, host sleep, visible errors
[Reset] [Save changes]     only while dirty
```

On phones, selectors stack below wrapping descriptions and actions retain 44px targets.

See the [full labelled previews](plan.md#ascii-ui-preview). Applicable criteria are listed in frontmatter.
Group structure and state visibility are required. ASCII spacing is illustrative.

## Verification

Start from the repository root. Install dependencies once before the first package command.
Add or update tests before changing covered behavior. Compare rendered results with the assigned preview.
New E2E files and family tags are defined in the plan and must exist before running these commands.

```bash
(cd apps/web && pnpm exec vitest run components/settings lib/settings-discovery/target.test.ts src/settings-routes.test.ts)
(cd apps/web && pnpm e2e:run e2e/tests/settings/settings-composition.spec.ts -- --grep 'task behavior')
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-settings-composition.spec.ts -- --grep 'task behavior')
(cd apps/web && pnpm e2e:run e2e/tests/system/message-queue-settings.spec.ts e2e/tests/system/session-capacity-settings.spec.ts e2e/tests/settings/settings-manual-save.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/system/mobile-message-queue-settings.spec.ts e2e/tests/settings/mobile-general-settings.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec eslint components/settings app/settings lib/settings-discovery --max-warnings 0)
git diff --check
```

Also run every existing E2E file whose selectors or assertions this work order changes.
Record its exact command and outcome under Results. Do not weaken assertions or leave changed tests unrun.

## Files likely touched

- `apps/web/components/settings/settings-{group,row}.tsx (new)`
- `apps/web/components/settings/settings-{card,card-header,section,page-template,typography,save-provider}.tsx`
- `apps/web/components/settings/{task-behavior-settings,general-settings}.tsx and all Task behavior controls`
- `apps/web/components/settings/system/{message-queue-settings,session-capacity-settings}.tsx`
- `apps/web/components/settings/system/use-session-capacity-settings.ts`
- `apps/web/lib/settings-discovery/{target.ts,target.test.ts,catalog/preferences.ts}`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/`
- `apps/web/AGENTS.md and affected public documentation`
- `apps/web/e2e/tests/settings/settings-composition.spec.ts` (new shared family matrix)
- `apps/web/e2e/tests/settings/mobile-settings-composition.spec.ts` (new shared family matrix)
- Existing component and E2E tests beside the migrated surfaces

## Dependencies

None.

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

Implemented and verified. Added the shared `SettingsGroup` and `SettingsRow` compositions, moved Task behavior into the four ordered groups, kept Runtime and limits mounted behind a closed native disclosure with effective summaries, preserved dirty/save ownership, and made discovery reveal closed targets before focus. Updated locales, public Task behavior descriptions, the engineering guide, and affected E2E selectors.

- `(cd apps/web && pnpm exec vitest run components/settings lib/settings-discovery/target.test.ts src/settings-routes.test.ts)`: passed, 207 files and 1,400 tests.
- `(cd apps/web && pnpm e2e:run e2e/tests/settings/settings-composition.spec.ts)`: passed, 7 desktop tests, including the Task behavior family.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-settings-composition.spec.ts)`: passed, 6 mobile tests, including the Task behavior family.
- `(cd apps/web && pnpm e2e:run e2e/tests/system/message-queue-settings.spec.ts e2e/tests/system/session-capacity-settings.spec.ts e2e/tests/settings/settings-manual-save.spec.ts)`: passed, 14 desktop tests.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/system/mobile-message-queue-settings.spec.ts e2e/tests/settings/mobile-general-settings.spec.ts e2e/tests/system/mobile-session-capacity-settings.spec.ts)`: passed, 9 mobile tests.
- Typecheck, localization checks, ratchet, scoped ESLint, and `git diff --check`: passed.

Review remediation on 2026-09-21 added transition-aware runtime revelation for
queue, session, and sleep load/validation/save attention, passed queue/session
dirty state into the Runtime group, and exposed the existing localized green
unsaved status. The focused Task behavior tests cover initial failure, invalid
draft, save failure after collapse, queue dirty state, and sleep attention while
the existing system save/reset suites cover successful persistence.

Fixup remediation added owner-specific attention transition keys so a second
runtime owner reopens the group while the first owner remains failed. It also
made discovery disclosure opening synchronize through an explicit event and
added the Sleep attention callback regression. The focused fixup suite passed
4 files and 28 tests.

The follow-up E2E selector migration uses the shipped row surface for the
auto-focus preference instead of the former standalone card. The exact desktop
and mobile flows pass with retries disabled.

- `(cd apps/web && pnpm e2e:raw --project chromium --retries=0 e2e/tests/task/creation-auto-focus.spec.ts)`: passed, 1 test.
- `(cd apps/web && pnpm e2e:raw --project mobile-chrome --retries=0 e2e/tests/task/mobile-creation-auto-focus.spec.ts)`: passed, 1 test.
- `(cd apps/web && pnpm e2e:raw --project mobile-chrome --retries=0 e2e/tests/task/mobile-mcp-task-agent-profile-default.spec.ts)`: passed, 1 test after aligning the flow with the group-owned surface.
