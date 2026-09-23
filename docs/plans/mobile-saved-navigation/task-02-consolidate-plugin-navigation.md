---
id: "02-consolidate-plugin-navigation"
title: "Consolidate mobile plugin navigation"
status: in_progress
wave: 2
depends_on: ["01-restore-phone-navigation"]
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-MENU-007
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
acceptance_criteria:
  - AC-UI-MOBILE-MENU-007.2
  - AC-UI-MOBILE-MENU-007.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.5
system_design:
  - ../../specs/ui/system-design/unified-mobile-navigation.md
  - ../../specs/ui/system-design/sidebar-customization.md
---

# Task 02: Consolidate mobile plugin navigation

## Scope

Combine the plugin grouping and resource placement from PR #3889 with the saved
navigation fixes in PR #3890. The user requested one PR, adapted shared
components, and new screenshots populated with tasks and installed plugins.
The source handoff is `b48359592` plus review/CI follow-up `e27b2b466`.

## Acceptance and ASCII UI preview

```text
Menu                         x
Workspace / Home / quick actions
Tasks v                      +
  [populated task rows]
Saved optional tools and plugin destinations
Integrations v
  GitHub / GitLab / Integration settings
Plugins
  Workspace: Project brief / Release notes
  Task: Review checklist
System metrics
Utilities
```

This saved-layout excerpt covers the criteria above. Default layouts retain
their existing destination order. Plugin controls share one section; saved
destinations render once. Workspace/task context, the single scroller, 44px phone
targets, desktop composition, and metrics preferences remain intact.

## Ownership, dependencies, and risks

Work stays in the primary session. Task 01 and the committed peer handoff are
the inputs. Reconcile AppNavSheet, AppNavSections, workspace actions, scoped
guidance, and the unified navigation design. Plugin-owned controls must retain
their context, including the workspace label, without forced square sizing.
No plugin SDK, backend, release-flag default, or persistence changes.

## Verification

```bash
make build-web
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-sidebar-customization.spec.ts tests/layout/mobile-menu-hierarchy.spec.ts tests/integrations/mobile-integrations-nav.spec.ts tests/plugins/mobile-plugin-topbar.spec.ts tests/settings/mobile-resource-metrics-display.spec.ts tests/task/mobile-autopilot-mode.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --no-build --project chromium tests/settings/sidebar-customization.spec.ts -- --retries=0)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

Run the nine focused unit suites from Task 01 plus mobile-plugin-nav-section and
main-top-bar-plugin-actions, and focused ESLint/Prettier. Capture from a fresh
isolated E2E backend using this PR's production build. Seed four storefront
tasks, synthetic GitHub/GitLab identities, and a packaged demonstration plugin
with functioning project notes, release notes, and a task review checklist.
Enable Canvases only in the disposable capture runtime. Inspect both themes,
320px/393px/767px phones, and a 1280px desktop. Publish media separately from code.

## Results

- Reconciled both committed peer changes while preserving task-first saved
  layouts, native built-in disclosures, and saved plugin destination ownership.
- RED/GREEN: moved sidebar controls omitted the workspace label. The grouping
  test reproduces the omission and now passes with the full context forwarded.
- 22 combined mobile scenarios passed, including the first saved visibility
  edit with installed plugins, task controls, metrics preference branches,
  320px/767px containment, the 768px boundary, and the autopilot assertion fix.
- All three desktop sidebar regressions passed after integration.
- 77 focused unit cases across nine suites pass on the combined implementation.
  Typecheck, affected lint/formatting, catalog/specification checks, and public
  documentation validation passed.
- Two capture rehearsals exercised installed demo-plugin actions and verified
  four tasks in the phone and desktop navigation. Final clean-source capture,
  PR update, and superseded-PR closure remain delivery steps.
- The peer's intermittent history-recovery scenario passed three fresh diagnostic
  runs with retries disabled. Each observed the intended two dropped replies.
  No speculative fixture change is included; final CI remains the delivery gate.
