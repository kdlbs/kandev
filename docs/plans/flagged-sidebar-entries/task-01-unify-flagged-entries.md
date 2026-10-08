---
id: "01-unify-flagged-entries"
title: "Unify flagged sidebar entries"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-001
  - REQ-UI-SIDEBAR-CUSTOMIZATION-004
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
  - REQ-UI-SIDEBAR-CUSTOMIZATION-006
  - REQ-COORDINATOR-NEEDS-YOU-006
acceptance_criteria:
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.5
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.6
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.7
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.8
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.9
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.5
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.6
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.7
  - AC-COORDINATOR-NEEDS-YOU-006.1
  - AC-COORDINATOR-NEEDS-YOU-006.2
  - AC-COORDINATOR-NEEDS-YOU-006.3
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
  - ../../specs/coordinator/system-design/needs-you.md
---

# Task 01: Unify flagged sidebar entries

## Summary

Give Coordinators its own portable sidebar node and current navigation presentation.
Use shared builtin eligibility for desktop, phone, and Settings.
Keep coordinator content and existing save recovery intact.

## In scope

- Backend defaults, projection compatibility, reset, and targeted tests.
- Frontend builtin metadata, API conversion, projection, editor, preview, and direct writes.
- Independent coordinator rendering and fast-action placement on desktop and phone.
- Unit/component and focused browser coverage.
- Update the customization how-to in `docs/public/use-kandev.md` after behavior exists.

## Out of scope

Flag lifecycle changes, fixed Office sections, Tasks layout, new persistence, and coordinator domain changes.

## Acceptance

1. Both configuration paths offer every eligible builtin entry, including Coordinators, without resource-list dependence.
   Disabled saved nodes survive edits and flag changes; old layouts gain Coordinators without disturbing existing choices.
2. Coordinators uses the current navigation disclosure, respects fast actions, and renders once at its saved position.
   Hiding Automations never controls Coordinator visibility. Hidden Coordinators never reappears through a fallback.
3. Desktop and phone flows pass the named checks with reload persistence, accessible controls, and scoped save behavior.
   The public guide explains eligibility and independent configuration.

## ASCII UI preview

Excerpts from [the combined previews](plan.md#ascii-ui-preview).
Read their state annotations before implementation.

UI-01: desktop, Coordinator enabled, fast actions off, empty list.

```text
[coord] Coordinators       v
  Open coordinators
  Set up a coordinator
[bolt] Automations         >

Sidebar settings
[x] Coordinators
[x] Automations
[x] Canvases (if enabled)
...existing choices...
Sidebar layout settings
```

UI-02: Settings layout list, enabled and unavailable saved states.

```text
Coordinators   [on] [up] [down]
Automations    [on] [up] [down]
Coordinators (unavailable) [saved choice]  (flag off)
...draft preview... [shared Save changes]
```

UI-03: phone navigation and temporary customization drawer.

```text
Navigation                 Close
[New Task]
Home / quick actions
[coord] Coordinators           v
  Open coordinators
  Planner                    [2]
Automations                    >
[Customize sidebar]
Tasks / local navigation

Customize sidebar           Done
[x] Coordinators
    [Move up] [Move down]
[x] Automations
Sidebar layout settings
```

Previews map to the acceptance IDs in the plan.
Use existing primitives, one drawer scroller, safe-area clearance, and 44px phone targets.
Keep fixed header and saved tool ordering. Spacing and example names are illustrative.

## Verification

For a fresh worktree without dependencies, first run `(cd apps && pnpm install --frozen-lockfile)`.
Read the relevant TDD and E2E references before implementation.
Run these commands sequentially from the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/user/models ./internal/user/service)
(cd apps/web && pnpm exec vitest run lib/sidebar/layout-projection.test.ts lib/sidebar/sidebar-customization.test.ts lib/sidebar/builtin-layout-nodes.test.ts hooks/domains/sidebar/use-sidebar-shortcut-catalog.test.ts hooks/domains/sidebar/use-sidebar-layout-navigation.test.ts hooks/domains/sidebar/use-sidebar-customization.test.ts components/settings/sidebar-layout-editor.test.tsx components/app-sidebar/sections/coordinators-section.test.tsx components/navigation/mobile-coordinators-section.test.tsx components/navigation/app-nav-sheet.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/sidebar/builtin-layout-nodes.ts lib/sidebar/layout-types.ts lib/sidebar/layout-projection.ts hooks/domains/sidebar/use-sidebar-layout-navigation.ts hooks/domains/sidebar/use-sidebar-customization.ts components/app-sidebar/sidebar-layout-navigation.tsx components/app-sidebar/sections/coordinators-section.tsx components/navigation/mobile-sidebar-layout-navigation.tsx components/navigation/mobile-coordinators-section.tsx components/settings/sidebar-layout-editor.tsx components/settings/sidebar-layout-editor-state.ts components/settings/sidebar-layout-editor-node-item.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/sidebar-flagged-entries.spec.ts tests/settings/sidebar-direct-customization.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-sidebar-flagged-entries.spec.ts tests/settings/mobile-sidebar-direct-customization.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 -- --project=chromium --workers=1 --retries=0 --repeat-each=3 tests/task/sidebar-scroll-preservation.spec.ts --grep "reveals a command-selected task")
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 -- --project=mobile-chrome --workers=1 --retries=0 tests/settings/mobile-config-chat-restart.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The new builtin-table, layout-hook, and flagged-entry browser test files are deliverables.
Add any further changed test suite to this block before completion.
Record behavioral RED evidence, final command results, and rendered comparisons to UI-01 through UI-03.
Run the documentation coverage preflight for this package before handoff and after changes to its references.

## Files likely touched

- `apps/backend/internal/user/models/sidebar_layouts.go` and `sidebar_layouts_test.go`.
- `apps/backend/internal/user/service/sidebar_layouts.go` and `sidebar_layout_test.go`.
- `apps/web/lib/sidebar/builtin-layout-nodes.ts` and its new test.
- `apps/web/lib/sidebar/layout-types.ts`, `layout-projection.ts`, and corresponding existing tests.
- `apps/web/hooks/domains/sidebar/use-sidebar-layout-navigation.ts` and its new test.
- `apps/web/hooks/domains/sidebar/use-sidebar-customization.ts` and its test.
- `apps/web/hooks/domains/sidebar/use-sidebar-shortcut-catalog.ts` and its test, if picker eligibility needs adaptation.
- `apps/web/components/app-sidebar/sidebar-layout-navigation.tsx`.
- `apps/web/components/app-sidebar/sections/coordinators-section.tsx` and its test.
- `apps/web/components/navigation/mobile-sidebar-layout-navigation.tsx`.
- `apps/web/components/navigation/mobile-coordinators-section.tsx` and its test.
- `apps/web/components/navigation/app-nav-sheet.test.tsx`.
- `apps/web/components/settings/sidebar-layout-editor.tsx`, its state, node list, node item, and test.
- `apps/web/e2e/tests/settings/sidebar-flagged-entries.spec.ts` and `mobile-sidebar-flagged-entries.spec.ts`.
- `docs/public/use-kandev.md` and this package's paired specifications.

## Dependencies

None. Reuse the shipped sidebar and coordinator implementations.

## Risks

- Full draft filtering can erase unavailable saved entries.
- Legacy renderer fallbacks can resurrect a hidden node.
- Default materialization can dirty a clean draft or move saved nodes.
- Shared metadata must not change Office or plugin eligibility.
- Delayed catalog responses must not overwrite another workspace or an unsaved draft.

## Parallelism

`sequential`

## Inputs

- [Sidebar requirements](../../specs/ui/requirements/sidebar-customization.md), especially 001.6 through 001.9.
- [Sidebar design](../../specs/ui/system-design/sidebar-customization.md#feature-gated-layout-entries).
- [Coordinator sidebar requirement](../../specs/coordinator/requirements/needs-you.md#req-coordinator-needs-you-006-entry-routes-and-sidebar).
- [Coordinator design](../../specs/coordinator/system-design/needs-you.md#routes-and-sidebar).
- Existing Canvases navigation, coordinator component tests, direct customization E2E, and coordinator feature fixture.
- [Portable settings decision](../../decisions/0041-backend-owned-portable-user-settings.md).
- [Navigation manifest decision](../../decisions/2026-08-04-navigation-manifest-boundaries.md).

## Results

The tests first failed on the missing default and compatibility Coordinator nodes,
feature eligibility projection, navigation presentation, and fast-action-off
destination. After implementation, verification passed:

- `(cd apps/backend && go test -trimpath ./internal/user/models ./internal/user/service)`
- The focused Vitest command in Verification: 10 files, 80 tests passed.
- `make -C apps/backend build` and `pnpm --filter @kandev/web build:vite` passed.
- `pnpm run typecheck` and changed-file ESLint passed.
- `pnpm run i18n:check` passed with 9,489 referenced keys and seven complete locales.
- Managed desktop E2E passed 7/7 tests; managed phone E2E passed 2/2 tests. The runner built the backend and Vite E2E assets for both projects.
- Public docs tests passed (62/62), and public docs validation covered 47 pages.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` passed.
- Actual-path PR docs coverage reported covered with no reference errors. `git diff --check` passed.

UI-01 rendered the Coordinator at its saved position with independent visibility,
empty-list setup, and the body destination when fast actions are off. UI-02 kept
feature-disabled entries unavailable while retaining their saved state, then
restored the hidden and reordered choices after re-enabling the feature. UI-03
followed saved phone order and passed customization, route, 44px target,
viewport-boundary, and narrow fine-pointer checks. Vite builds emitted existing
chunk-size and dynamic-import warnings but completed successfully.

PR fixup verification corrected flex shrinking in the desktop customization
wrapper so Coordinator content cannot overlap later Office sections. The phone
target assertions round measured CSS-pixel heights before checking the 44px
minimum, avoiding subpixel precision failures. The focused Office navigation
E2E checks passed 2/2 tests; the focused phone sidebar and plugin metadata E2E
checks passed 2/2 tests. Typecheck, changed-file ESLint, and Prettier passed.
These fixes preserve the documented navigation behavior, so no requirement,
design, or public-guide change was needed.

The full E2E retry summary also exposed a timing-sensitive sidebar cue assertion
and a subpixel touch-target measurement. The sidebar test now observes the cue
while task selection is in progress, and the phone measurements round CSS
pixels before checking the 44px target. Retries-disabled repetitions passed
9/9 sidebar tests and 3/3 repeated mobile refresh tests; the full Configuration
Chat restart spec passed 2/2 tests.
