---
id: "02-settings-surface"
title: "Expose session capacity in Settings"
status: completed
wave: 2
depends_on:
  - "01-capacity-settings"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
  - REQ-AGENTS-SESSION-CEILING-002
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-001.1
  - AC-AGENTS-SESSION-CEILING-001.2
  - AC-AGENTS-SESSION-CEILING-001.8
  - AC-AGENTS-SESSION-CEILING-001.9
  - AC-AGENTS-SESSION-CEILING-002.1
  - AC-AGENTS-SESSION-CEILING-002.2
  - AC-AGENTS-SESSION-CEILING-002.3
  - AC-AGENTS-SESSION-CEILING-002.4
  - AC-AGENTS-SESSION-CEILING-002.5
  - AC-AGENTS-SESSION-CEILING-002.6
  - AC-AGENTS-SESSION-CEILING-002.7
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 02: Expose session capacity in Settings

## Summary

Let administrators enable, change, and disable the ceiling in Task Behavior.
Prove the saved form changes automatic admission on desktop and remains fully
usable on a phone. Update public docs with the delivered behavior.

## In scope

- Typed settings API/DTOs, section, discovery target, and shared save contributor.
- Off/on, remembered maximum, validation, load/save errors, member and env locks.
- All five locale catalogs and generated Traditional Chinese translations.
- Component/API tests and desktop/phone E2E with real backend saves and reloads.
- Public task/workflow docs and this work order's results. Leave final
  specification/package status reconciliation to Task 03 after all checks pass.

## Out of scope

- New settings navigation hierarchy or an independent mobile drawer.
- Admission logic beyond Task 01, workflow WIP changes, or live-instance edits.

## Acceptance

1. The discoverable form saves enabled state and maximum atomically, preserves
   in-flight edits, and accurately reports effective, invalid, error, and locked states.
2. Desktop and phone flows satisfy UI-01/UI-02 and focused tests prove enabling
   queues automatic work and disabling releases the exact eligible session.
3. Localized copy and public docs describe the actual disabled default,
   instance scope, manual bypass, environment precedence, and live saves.

## ASCII UI preview

UI-01 excerpt, Settings > Task Behavior; see the
[full preview](plan.md#ascii-ui-preview) for UI-02 errors and managed states.

```text
Desktop, default                 Phone, enabled draft
+----------------------------+  +--------------------------------+
| Session capacity           |  | < Settings    Task Behavior     |
| All workspaces             |  | Session capacity               |
| Limit automatic sessions   |  | All workspaces                 |
|                      [OFF] |  | Limit automatic sessions [ON]  |
| Current: No session limit  |  | Maximum automatic sessions     |
+----------------------------+  | [ 5                          ] |
                                | Current: No session limit      |
                                | (changes not saved)            |
                                +--------------------------------+
                                  [ Reset ] [ Save changes ]
```

Switch then numeric field when enabled; scope and effective state remain visible.
The existing Settings scroll container and save bar own scrolling and safe areas.
Use 44px phone/coarse-pointer hitboxes and 28px desktop controls. The card has
no nested scrolling. Exact spacing is illustrative. Maps to AC-002.1, .2, .6,
and .7; error/read-only states also cover AC-002.4 and .5.

## Verification

Run from the repository root; install dependencies once for this worktree.
Use TDD for draft/API behavior. Use the managed E2E runner to build current
assets, then compare the rendered phone composition with the preview.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/settings/system/session-capacity-settings.test.tsx lib/api/domains/settings-api.test.ts lib/settings-discovery/catalog.test.ts lib/settings-discovery/target.test.ts lib/settings-discovery/coverage-inventory.test.ts components/settings/settings-target-surfaces.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/settings/system/session-capacity-settings.tsx components/settings/system/use-session-capacity-settings.ts components/settings/task-behavior-settings.tsx lib/api/domains/settings-api.ts lib/types/system.ts lib/settings-discovery/catalog/preferences.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/system/session-capacity-settings.spec.ts tests/workflow/queued-session-ownership.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-session-capacity-settings.spec.ts tests/workflow/mobile-queued-session-ownership.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the locale generation command before i18n validation during implementation:
`(cd apps/web && pnpm run i18n:zh-hant)`. Inspect its diff and retain only required
catalog changes. Existing queued-session E2E must retain an explicit test limit;
do not reintroduce a production profile default for tests. Capture and restore
the settings baseline in every E2E mutation scenario. Report exact test counts
and results, including blocked commands, without treating planned checks as run.

## Files likely touched

- New `apps/web/components/settings/system/session-capacity-settings.tsx`,
  `use-session-capacity-settings.ts`, and `session-capacity-settings.test.tsx`.
- `apps/web/components/settings/task-behavior-settings.tsx`.
- `apps/web/lib/api/domains/settings-api.ts`, `settings-api.test.ts`.
- `apps/web/lib/types/system.ts`, `lib/settings-discovery/catalog/preferences.ts`.
- Settings discovery/target tests named in Verification.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/system.json` and settings keys as needed.
- New `apps/web/e2e/tests/system/session-capacity-settings.spec.ts`,
  `mobile-session-capacity-settings.spec.ts`, and shared helper.
- `docs/public/tasks-and-workflows.md` and this package's status/results.
- Paired system-design status when implementation and all checks are complete.

## Dependencies

Task 01's persisted settings API and live runtime application.

## Risks

The selected page also contains personal settings: the new section must clearly
state instance scope and enforce admin permissions. Do not show an editable
saved value as effective under an environment lock. Preserve draft edits during
save and keep the mobile save bar from covering the final control.

## Parallelism

`sequential`

## Inputs

- Paired requirement/design and plan UI-01/UI-02 and E2E matrix.
- `apps/web/AGENTS.md`, `/mobile-parity`, `/e2e`, and `/docs-maintainer`.
- Existing Message Queue Settings card, save contributor, and mobile E2E.

## Results

Implemented the Task Behavior Settings section, typed API, shared save
contributor, discovery target, environment/member read-only states, localized
copy, and desktop/phone layouts.

- Focused frontend validation passed with 68 tests across 9 files. Web
  typecheck, full ESLint, i18n validation, and the production Vite build passed.
- Managed Chromium passed the session-capacity Settings persistence and
  environment-lock scenarios. Managed mobile Chromium passed the touch-target,
  single-scroll-owner, save/reload, and disable scenarios.
- Public task/workflow documentation was updated and validated.
