---
id: "01-select-mobile-toolbar"
title: "Select one mobile toolbar per plugin"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-MENU-008
acceptance_criteria:
  - AC-UI-MOBILE-MENU-008.1
  - AC-UI-MOBILE-MENU-008.2
system_design:
  - ../../specs/ui/system-design/unified-mobile-navigation.md
---

# Task 01: Select one mobile toolbar per plugin

## Summary and scope

Choose task toolbar registrations in the phone menu for plugins contributing
both toolbars, retain all other controls, and remove redundant context headings.
Own the host render filtering, mobile composition, regressions, documentation,
and seeded before/after PR evidence. Plugin implementation, SDK signatures, and backend remain out of scope.

## Acceptance

- One toolbar per plugin on tasks; all task actions retain context.
- Listings, archived tasks, live unload, and distinct workspace plugins work.
- Actions fit 320/393/767px, retain 44px targets, and desktop is unchanged.

## ASCII UI preview

UI-01: Task hamburger menu. Existing inset drawer, fixed Menu heading, one
safe-area-aware vertical scroller, and 44px touch controls remain the exemplar.

```text
Before                    After
Plugins                   Plugins
  Workspace                 [CPU] [Companion] [Usage 42%]
  [CPU] [Usage 42%]          [Plugin destinations]
  Task
  [Companion] [Usage 42%]
```

Task controls keep task/session context. Listings use workspace controls.
No contributions means no section. Desktop retains its separate toolbar
locations. Wrapping and hierarchy are requirements; spacing is illustrative.

Full preview: [plan](plan.md#ascii-ui-preview).

## Files likely touched

- `apps/web/components/plugins/plugin-slot.tsx` and existing tests
- `apps/web/components/plugins/plugin-slot-presence.tsx`
- `apps/web/components/plugins/mobile-plugin-nav-section.tsx` and existing tests
- `apps/web/components/kanban/main-top-bar-plugin-actions.tsx`
- `apps/web/e2e/tests/plugins/mobile-plugin-topbar.spec.ts`
- Relevant UI requirements/design, frontend guide, and public plugin guide

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/plugins/plugin-slot.test.tsx components/plugins/mobile-plugin-nav-section.test.tsx components/kanban/main-top-bar-plugin-actions.test.tsx components/navigation/app-nav-sheet.test.tsx components/task/task-top-bar-plugin-actions.test.tsx)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/plugins/mobile-plugin-topbar.spec.ts)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run changed-file ESLint/Prettier and i18n ratchet; inspect and publish seeded
screenshots. Dependencies: none. Parallelism: sequential.

## Risks

Owner-based toolbar selection intentionally replaces a plugin's workspace
toolbar on task pages. All entries within its selected slot remain intact.

## Results

- RED: four focused unit failures reproduced owner duplication and subgroup headings.
- GREEN: 64 tests in five focused Vitest suites passed, including task slot
  context and error-boundary coverage. The task slot suite was included in the
  recorded command in addition to the four listed initially.
- Managed production build: all three selected mobile scenarios passed (two
  permanent regressions plus the disposable installed-plugin capture).
- Final mobile E2E: two scenarios passed after adding archived-task fallback
  and 768/1440px parity checks. Phone assertions cover 320/393/767px, control
  activation, current task/session props, one scroller, touch targets, and no
  horizontal overflow. This final run reused the same fresh production build.
- Web typecheck, changed-file ESLint, Prettier, and i18n ratchet passed.
- Public docs: 62 validator tests and 47 pages passed. Catalog validation:
  304 decisions and 1,136 specifications; specification and harness lint passed.
- Five inspected screenshots: before/after dark and light plus expanded usage.
  Installed the unmodified Provider Usage 0.9.3 release in isolated backends;
  seeded three Atlas Studio tasks, a session, and messages through the API.
  The provider's quota HTTP response is deterministic fixture data (42% session,
  28% weekly); no DOM replacement or live account data. Before assets used base
  d60274528129dc17d413357c104c9c427c4e7af9; after assets used the fixed build.
- Capture backend PIDs exited, ports closed, and temporary databases/repositories
  were removed by the fixture. The disposable capture spec was removed; only
  evidence remains in ignored PR assets and an external artifact directory.
- The phone Plugins drawer is absent from desktop; desktop parity is verified
  by the 768/1440px route checks rather than an unrelated screenshot.
- `git diff --check` passed.

## CI documentation follow-up

The coverage evaluator identified the new requirement missing from the design's
frontmatter. Added `REQ-UI-MOBILE-MENU-008` to the design dependency list. The
exact repository coverage evaluator failed before the metadata correction and
passes afterward (`status: covered`), along with catalog/spec lint and diff
checks. Product code and screenshot content are unchanged.

The authoring-guide review identified stale Workspace/Task subgroup guidance.
Updated both slot tables, the canonical guide, PLUGIN-API reference, and SDK/host
type comments to describe the same owner-based selection. Public docs validation
(62 tests, 47 pages), coverage evaluation, spec/catalog lint, and diff checks pass.

## Null-rendering task fallback

Review found registered task controls can return null before a session exists.
The new regression fails under registration-only selection. Observe mounted
content in layout-neutral owner wrappers, retaining workspace controls until
task content appears and restoring them if it disappears. An observer scoped
to the task slot disconnects on unmount and only reports changed owner sets.
Targeted unit tests, fresh production mobile E2E, and refreshed after screenshots
verify the correction: 64 focused unit tests and all three selected mobile
scenarios passed against a fresh production build.
