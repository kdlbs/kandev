---
id: "01-desktop-navigation"
title: "Distinguish desktop actions and navigation groups"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-NAV-HIERARCHY-001
acceptance_criteria:
  - AC-UI-NAV-HIERARCHY-001.1
  - AC-UI-NAV-HIERARCHY-001.2
  - AC-UI-NAV-HIERARCHY-001.3
  - AC-UI-NAV-HIERARCHY-001.4
  - AC-UI-NAV-HIERARCHY-001.5
  - AC-UI-NAV-HIERARCHY-001.6
system_design:
  - ../../specs/ui/system-design/navigation-hierarchy.md
---

# Task 01: Desktop actions and navigation groups

## Summary

Make New Task primary and built-in navigation groups recognizably expandable.
Preserve saved order, route/creation ownership, rail behavior, and all utilities.

## In scope

- Read `/tdd`, backend/web guidance, and the design before edits. Install missing
  workspace dependencies once. The current shell has no Node/pnpm on PATH; use
  the pinned Node 24 and pnpm 9.15.9 toolchain from `mise.toml`, following
  `scripts/bootstrap-dev-env` and `docs/remote-cloud-environment.md`.
  Add failing behavioral tests first.
- Change canonical unsaved/default node order and the fallback desktop render;
  keep saved nodes untouched. Verify required inbox insertion in saved layouts.
- Full-width primary New Task with a centered plus and label, no shortcut hint,
  and separate secondary actions. Keep the configured keyboard binding.
- Icon/label/chevron resource headers, indented links, integration setup when
  empty, semantic active Home/destinations, and labelled Settings/account footer.
- Localize any new visible or accessible labels across all six locales.

## Out of scope

Task-row redesign, phone composition, settings-schema changes, forced saved-layout
reset, custom shortcut-group redesign, new account pages, and provider APIs.

## Acceptance

1. Default and saved desktop layouts satisfy 001.1-001.4 and 001.6; links,
   disclosure buttons, and creation actions each perform their own operation.
2. Expanded and rail navigation retain every existing destination/action, actual
   authenticated identity, focus behavior, and guarded settings navigation.
3. Targeted component/Go tests and the new desktop E2E pass with rendered
   indentation, active state, 44px creation-control height, and coarse-
   pointer reachability. Capture dark/light desktop views for structural review.

## ASCII UI preview

UI-01 excerpt, expanded default sidebar. Full [preview](plan.md#ascii-ui-preview).

```text
Kandev / Workspace v [collapse]  fixed
[ + New Task          shortcut ] neutral surface
Quick Chat   Terminal           secondary
Home                          *   destination
Automations                   >   disclosure
Canvases                      >
Integrations                  v
  GitHub                          child destination
  Integration settings            also when empty
--------------------------------
TASKS [existing panel]
--------------------------------
[avatar] Settings [theme] [...]    one fixed footer row
```

Maps to all AC-UI-NAV-HIERARCHY-001 criteria. Saved ordering overrides the sample.
Phone keeps its current composition until Task 03; shared changes must retain its
44px hit areas. Do not apply a new hierarchy to custom shortcut groups or Office.

## Verification

From repository root, once dependencies exist:

```bash
(cd apps/backend && go test ./internal/user/models ./internal/user/service -run SidebarLayout -count=1)
(cd apps/web && pnpm exec vitest run components/app-sidebar/app-sidebar-new-task-item.test.tsx components/app-sidebar/app-sidebar-primary-nav.test.tsx components/app-sidebar/app-sidebar-section.test.tsx components/app-sidebar/sections/integrations-section.test.tsx components/app-sidebar/sections/automations-section.test.tsx components/app-sidebar/sections/canvases-section.test.tsx components/app-sidebar/app-sidebar-footer.test.tsx components/app-sidebar/app-sidebar.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/layout/navigation-hierarchy.spec.ts tests/settings/sidebar-customization.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/app-sidebar --max-warnings 0)
(cd apps/web && pnpm run i18n:check)
git diff --check
```

The new E2E includes 1280px desktop, 768px coarse-pointer control checks, collapsed
rail, empty integrations, saved layout hide/reorder, and Home current state.

## Files likely touched

- `apps/backend/internal/user/models/sidebar_layouts.go` and new `sidebar_layouts_test.go`.
- `apps/web/components/app-sidebar/app-sidebar-primary-nav.tsx`,
  `app-sidebar-new-task-item.tsx`, `app-sidebar-nav-item.tsx`,
  `app-sidebar-section.tsx`, `sidebar-layout-navigation.tsx`,
  `app-sidebar-footer.tsx`, `current-user-chip.tsx` and corresponding tests.
- `apps/web/components/app-sidebar/sections/{automations,canvases,integrations}-section.tsx`.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,pseudo}/sidebar.json` as needed.
- New `apps/web/e2e/tests/layout/navigation-hierarchy.spec.ts`.

## Dependencies

None. Task 03 owns phone reordering and any additional phone-specific labels.

## Risks

Do not replace the existing creation callbacks, drop quick-action availability,
relax provider gates, or lose plugin footer overflow and settings draft guards.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/navigation-hierarchy.md), REQ-UI-NAV-HIERARCHY-001.
- [Design](../../specs/ui/system-design/navigation-hierarchy.md), Existing authority/Desktop/Footer.
- Existing sidebar component tests, `settings/sidebar-customization.spec.ts`,
  and `models.DefaultSidebarLayout`.

## Results

Implemented the theme-primary New Task button and real shortcut hint; default order is New Task before Home without rewriting saved layouts. Built-in tool disclosures now have icons, chevrons, indented children, and an integration-settings destination. The initial preview used labelled Settings and authenticated-account footer rows; the requested refinement below supersedes that presentation.

Validation passed:

- `go test ./internal/user/models ./internal/user/service -run SidebarLayout -count=1`.
- The task-defined sidebar component unit tests, including existing creation and footer behavior.
- Desktop navigation (four scenarios) and saved-layout customization (three scenarios). Coverage includes empty integrations, keyboard disclosures, collapsed rail, both themes, 768px coarse-pointer controls, and 767px fine-pointer controls.
- Type checking, changed-file lint, localization completeness and new-code ratchet.

The shared filled action is `new-task-button.tsx`; it retains the existing creation host and shortcut overrides. No saved-layout migration was added.

## Requested refinement (2026-09-28)

After reviewing the seeded preview, the user explicitly requested a larger New
Task action, discoverable Quick Chat and Terminal, and one compact footer row.
The revised design uses a 44px primary action, equal-width labelled 28px secondary
actions, and Settings/theme/More Actions in one row. The authenticated account is
an avatar with identity in its menu. All secondary utilities and eligible footer
plugin destinations are labelled entries in the shared menu, preserving handlers
and the unseen-release indicator. Phone keeps its existing labelled utilities and
44px controls. The same isolated instance will be refreshed without reseeding.

Refinement completed. Three focused component suites passed (76 tests), covering
creation routes/shortcuts, utility navigation, release notes, account/logout,
settings guards, plugin registration order, and phone navigation. Fifteen distinct
desktop browser scenarios and twelve phone scenarios passed: 44px primary action,
equal labelled quick actions, one-row footer, plugin utilities, support dialogs,
terminal lifecycle, keyboard focus restoration, touch menus/theme control,
localized phone quick actions, issue access, and draft continuity. Type checking,
changed-file lint/formatting, six-language localization checks, documentation
validation, spec lint, and whitespace checks passed.

The same seeded instance was refreshed without resetting tasks, sessions, saved
views, or the grouping changed during user review. API read-back still confirms
five tasks, four sessions, two issues, two PRs, and one paused automation. The
browser checks passed in dark/light at 1280px and on 393/767px phones, plus
Portuguese, without page errors. `screenshots-iteration-1` preserves the previous
captures; `screenshots` contains the revision and its open utilities menu.
The original preview URL and stop command remain valid.


## Quiet action hierarchy (2026-09-28)

The user requested autonomous refinement of the saturated primary button and
oversized secondary actions. Visual comparison of neutral filled and outlined
variants selected a faint neutral surface and quiet border for New Task, retaining
its 44px target. Quick Chat and Terminal now use content-width muted ghost actions
at 28px on desktop. Phone retains native 44px targets and labelled ghost actions.
The existing one-row footer is unchanged. Comparison included neutral fill and
plain outline in both themes; the selected intermediate fill provides enough
creation affordance without a saturated block. Labels keep at least 4.74:1
contrast in light mode and 5.00:1 in dark mode, measured from rendered colors.
New Task retains 44px height; desktop utilities are 28px and phone utilities 44px.

Validation passed: 55 component tests, four desktop hierarchy/touch scenarios,
twelve phone navigation/translation/draft scenarios, and three Quick Chat focus
and terminal lifecycle scenarios. Type checking, changed-file lint/formatting,
localization completeness and ratchet, and documentation validation passed.
Actual final captures include dark/light desktop and phone, Portuguese desktop,
and keyboard focus. Live tasks and preferences were preserved. Evidence lives in
`/tmp/kandev-navigation-compare-z54_iuek/quiet-final` and `quiet-check.json`.
The seeded preview remains running at its existing URL.


## PR review remediation (2026-09-30)

Section disclosures reference their controlled content in ordinary and growing
sections. The utilities menu uses release-note availability rather than unseen
notification eligibility; reopening read notes shows the current release instead
of an empty dialog. Preferences still control the trigger indicator. Added
failing regressions before implementation, then verified section/footer
components and the release-note lifecycle. Browser coverage verifies actual
menu clickability and readable notes at the 768px desktop boundary. Final
validation and CI evidence are recorded in the plan and linked PR.
