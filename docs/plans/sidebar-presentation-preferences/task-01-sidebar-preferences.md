---
id: "01-sidebar-preferences"
title: "Persist and apply sidebar presentation preferences"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-NAV-HIERARCHY-001
  - REQ-UI-NAV-HIERARCHY-003
  - REQ-UI-NAV-HIERARCHY-004
  - REQ-UI-CONTROL-SIZING-001
acceptance_criteria:
  - AC-UI-NAV-HIERARCHY-001.1
  - AC-UI-NAV-HIERARCHY-001.3
  - AC-UI-NAV-HIERARCHY-001.4
  - AC-UI-NAV-HIERARCHY-001.5
  - AC-UI-NAV-HIERARCHY-001.6
  - AC-UI-NAV-HIERARCHY-001.7
  - AC-UI-NAV-HIERARCHY-003.5
  - AC-UI-NAV-HIERARCHY-003.6
  - AC-UI-NAV-HIERARCHY-004.1
  - AC-UI-NAV-HIERARCHY-004.2
  - AC-UI-NAV-HIERARCHY-004.3
  - AC-UI-NAV-HIERARCHY-004.4
  - AC-UI-NAV-HIERARCHY-004.5
  - AC-UI-NAV-HIERARCHY-004.6
  - AC-UI-NAV-HIERARCHY-004.7
  - AC-UI-CONTROL-SIZING-001.1
  - AC-UI-CONTROL-SIZING-001.2
  - AC-UI-CONTROL-SIZING-001.4
system_design:
  - ../../specs/ui/system-design/navigation-hierarchy.md
  - ../../specs/ui/system-design/control-sizing.md
---

# Task 01: Persist and apply sidebar presentation preferences

## Summary

Deliver both saved preferences, safe legacy backfill, explicit new-user defaults,
and Settings > Layout > Sidebar controls. Apply all four desktop states and
reduce the simple creation button and utility bar to 28px without altering phone flows.

## In scope

- Read `/tdd`, `/mobile-parity`, `/e2e`, and scoped backend/web guidance before implementation.
- Add `sidebar_fast_actions_enabled` and `sidebar_new_task_style` through existing
  user model, strict optional PATCH input, store, service, response, events,
  Go boot payload, settings catalog, generated contract snapshots, and client mapping.
- Implement transactional missing-key backfill and explicit insert defaults for
  both the initial default user and subsequently created accounts. Preserve raw
  unknown settings, explicit choices, layout nodes, and normal revision semantics.
- Add a user-scoped Sidebar appearance settings contributor. Shared Save/Discard,
  failure retention, event rebasing, and in-flight edits must work with the existing editor.
- Render simple/compact styles with either inline icons or labelled utility bar.
  Gate built-in canvas/provider shortcuts and retain labelled destinations/settings.
- Preserve route-aware launchers, dialog hosts, activities, keyboard shortcuts,
  saved layout positions, plugin actions, rail behavior, and direct Stats access.
- Add meaningful migration, contract, state, component, and browser tests; update
  seven language catalogs, generated Traditional Chinese/pseudo resources, scoped
  web guidance, and the public tutorial once implementation exists.

## Out of scope

New runtime flags, phone redesign, provider eligibility, layout-node migrations,
unrelated preferences, new backend endpoints, commits, pushes, and PR publication.

## Acceptance

1. Fresh/default/new accounts retain false/simple after database replay; legacy
   users receive true/compact only for missing fields. Partial explicit values,
   unrelated fields, unknown keys, and failed migrations behave as designed.
2. Settings persist both independent choices through shared Save/Discard, API,
   boot, and WS paths. Failed requests retain drafts; partial updates and later
   in-flight edits do not reset saved preferences or navigation layouts.
3. All four desktop states follow the plan's matrix and measured sizes. Optional
   shortcuts remain independent controls; labelled paths, phone touch sizing,
   creation-draft continuity, rail behavior, plugins, and Stats remain intact.

## ASCII UI preview

See the [full preview and state matrix](plan.md#ascii-ui-preview).
UI-01 through UI-03 implement AC-UI-NAV-HIERARCHY-004.1/4/5/6; UI-04 preserves 004.7.

UI-01: Expanded desktop, simple style, icons off. UI-02: Icons on.

```text
UI-01 (28px + 28px)            UI-02 (style height + 24px icons)
+-------------------------+   +-------------------+----+----+
|        + New Task       |   | + New Task        | >_ | QC |
+------------+------------+   +-------------------+----+----+
| Quick Chat |  Terminal  |     Canvases                [C] >
+------------+------------+     Integrations          GH LN >
  Canvases               >
  Integrations           >
```

UI-03: Sidebar appearance settings. UI-04: Phone drawer, all desktop settings.

```text
UI-03                             UI-04
Show fast action icons [off]       Menu / Workspace [v] (fixed)
New Task style [New design v]      [+ New Task]
[Existing layout editor]          Home
[Error only on save failure]      [Quick Chat] [Terminal]
[Shared Discard / Save]           [Tools, Tasks, utilities] (scroll)
```

Labels/icons are illustrative ASCII. Fine-pointer desktop simple buttons are
28px; old compact creation rows remain 36px; inline icons are 24px. Phone and
coarse-pointer controls keep 44px minimums. Settings rows may stack on phone.
Use the existing inset Drawer, safe-area clearance, and one nav scroller.

## Verification

Use Red-Green-Refactor for migration and changed logic. If dependencies are
missing, first run `(cd apps && pnpm install --frozen-lockfile)`.
The PostgreSQL migration test should share assertions with SQLite and use the
existing optional `testutil.PostgresDSNFromEnv` integration convention. Record
whether that test actually executed or skipped; do not claim PostgreSQL proof from a skip.

Run from the repository root; each command roots its own working directory.
The proposed settings component/test paths below are new work-order outputs.

```bash
(cd apps/backend && go test ./internal/user/models ./internal/user/store ./internal/user/dto ./internal/user/service ./internal/user/handlers)
(cd apps/web && pnpm exec vitest run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts lib/state/default-state.test.ts components/settings/sidebar-presentation-settings.test.tsx components/settings/sidebar-presentation-state.test.ts components/settings/layouts/layout-settings.test.tsx components/app-sidebar/app-sidebar-new-task-item.test.tsx components/app-sidebar/sections/canvases-section.test.tsx components/app-sidebar/sections/integrations-section.test.tsx components/app-sidebar/app-sidebar-footer.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/layout/navigation-hierarchy.spec.ts tests/settings/sidebar-customization.spec.ts tests/plugins/plugin-action-ux.spec.ts tests/chat/quick-chat.spec.ts tests/terminal/quick-terminal.spec.ts tests/integrations/hide-disabled-integrations-nav.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-navigation-hierarchy.spec.ts tests/layout/mobile-unified-navigation.spec.ts tests/layout/mobile-menu-hierarchy.spec.ts tests/settings/mobile-sidebar-customization.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/app-sidebar components/settings/sidebar-presentation-settings.tsx components/settings/sidebar-presentation-state.ts components/settings/layouts/layout-settings.tsx lib/ssr/user-settings.ts --max-warnings 0)
(cd apps/web && pnpm run i18n:check)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed E2E runner builds current assets. Do not overlap full browser suites
or bypass shard/worker guards. Record actual sizes, theme/translation checks,
each test command's results, and any PostgreSQL skip. Run the repository's
PR-documentation coverage preflight with the new work order before completion.

## Files likely touched

- `apps/backend/internal/user/models/models.go` and a focused style helper/test.
- `apps/backend/internal/user/store/sqlite.go`, a focused migration helper/test,
  and existing SQLite/PostgreSQL migration fixtures.
- `apps/backend/internal/user/dto/dto.go`, strict input handling, and tests.
- `apps/backend/internal/user/service/service.go`, settings validation/application helpers, and tests.
- `apps/backend/internal/user/handlers/` preference round-trip tests.
- `apps/web/lib/types/http-user-settings.ts`, `lib/state/slices/settings/types.ts`,
  `lib/state/default-state.ts`, `lib/ssr/user-settings.ts`, and their tests.
- `apps/web/lib/ws/handlers/users.test.ts` for partial/stale event preservation.
- New `apps/web/components/settings/sidebar-presentation-settings.tsx`,
  `sidebar-presentation-state.ts`, and focused tests.
- `apps/web/components/settings/layouts/layout-settings.tsx` and its test.
- `apps/web/components/app-sidebar/app-sidebar-new-task-item.tsx`, optional
  focused presentation helper, workspace actions, and component tests.
- `apps/web/components/app-sidebar/sections/{canvases,integrations}-section.tsx` and tests.
- Browser specs listed above and `apps/web/e2e/helpers/api-client.ts` if fixture
  settings payload types need the new fields.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/settings.json`.
- `apps/web/AGENTS.md` and `docs/public/use-kandev.md`.
- Paired specifications and plan/work-order status/results after verification.

## Dependencies

None. The branch already contains the completed placement implementation in PR #4239.
Re-resolve the PR and base heads before implementation if another session updates them.

## Risks

Preserve migration atomicity across both database engines. A missing-key migration
must not reclassify new users on restart. Do not remove canvas management access
or lose newer draft edits when a settings response arrives.

## Parallelism

`sequential`

## Inputs

- [Navigation requirements](../../specs/ui/requirements/navigation-hierarchy.md), especially REQ-UI-NAV-HIERARCHY-004.
- [Navigation design](../../specs/ui/system-design/navigation-hierarchy.md), Saved sidebar preferences and Desktop composition.
- [Control sizing](../../specs/ui/requirements/control-sizing.md) and its paired design.
- Existing user-settings DTO/store/service patterns, revision migrations, SSR/WS mapping,
  `appearance-settings-state.ts`, `useSettingsSaveContributor`, and sidebar layout editor.
- Prior placement implementation and [completed evidence](../sidebar-action-placement/plan.md).

## Results

Complete on 2026-10-06. Implemented and verified in the primary session.
See the [shared implementation and validation record](plan.md#implementation-results-2026-10-06)
for backend, SQLite/PostgreSQL, frontend, desktop/phone browser, build,
localization, lint/type, and documentation evidence. No unresolved blocker.
The user subsequently authorized delivery to existing PR #4239.
