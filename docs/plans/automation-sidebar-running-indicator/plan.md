---
created: 2026-08-26
status: done
requirements:
  - REQ-OFFICE-AUTOMATION-RUNS-001
system_design:
  - ../../specs/office/system-design/automation-runs.md
legacy_specs: []
---

# Implementation Plan: Automation Sidebar Running Indicator

## Overview

Restore live automation health in the visible desktop sidebar and make an open
run visually unmistakable. The change remains frontend-only: the sidebar will
reuse the guarded automation-summary request and established live-refresh
cadence, then render a spinner for the existing `running` state. Focused unit
and Playwright coverage will prove the idle-to-running transition without a
reload.

## Scope

### In scope

- Refresh automation health summaries while the expanded Automations section
  is visible.
- Stop health refreshes while the section or desktop rail is hidden.
- Replace the running health dot with an animated loader while preserving the
  localized screen-reader state.
- Prove the reported recurring-run case through component and desktop browser
  coverage.

### Out of scope

- New backend events or WebSocket actions.
- Scheduler timing, automation admission, run persistence, or concurrency-cap
  changes.
- Changes to the `/automations` agenda or detail views.
- Adding the desktop-only Automations rail section to phone navigation.

## Technical approach

### Visible summary refresh

- In
  `apps/web/components/app-sidebar/sections/automations-section.tsx`, retain the
  full `useAutomationSummaries` result and pass its stable `refresh` function to
  `useLiveRefresh` with `showing` as the activity gate.
- Keep the existing `summaryScope` gate. Initial reads and repeat reads occur
  only while the section is expanded in the non-collapsed desktop rail; the
  hook cleanup stops the timer when that condition changes.
- Reuse `useAutomationSummaries` request ordering rather than adding local
  response arbitration or another polling abstraction.

### Running indicator

- In `AutomationRowLink`, render a compact Tabler loader with `animate-spin`
  and a stable automation-scoped test ID when `row.state === "running"`.
- Keep `STATE_DOT_CLASS` for `idle` and `paused`. Preserve the current
  screen-reader-only `STATE_LABEL_KEY` text, so the running animation is
  decorative and the semantic state remains localized.
- Keep the row height, navigation target, title truncation, trailing last-run
  age, and pointer behavior unchanged.

### Mobile parity

`AppSidebar` is explicitly desktop-only (`hidden md:block`); phone navigation
does not render `AutomationsSection`. This is a compact status-content change,
not a navigation or touch change. The closest mobile visual precedent is the
shared task-switcher running indicator, which also pairs a compact animated
state with semantic text. No phone composition, action, scroll owner, safe-area
rule, or touch target changes, so mobile Playwright coverage is not added for
this desktop-only surface.

## Tests

- `apps/web/components/app-sidebar/sections/automations-section.test.tsx`
  covers `AC-OFFICE-AUTOMATION-RUNS-001.9`: an initially idle row re-reads its
  summary on the live-refresh cadence, changes to an animated running indicator,
  and returns to a non-running dot after the terminal summary arrives.
- The same suite asserts that a running row exposes the localized `Running.`
  text, that only the loader animates, and that folded/collapsed states do not
  issue health-summary requests.

## E2E tests

- Extend `apps/web/e2e/tests/automations-sidebar.spec.ts` for
  `AC-OFFICE-AUTOMATION-RUNS-001.9`: mount and open the sidebar while an
  automation is idle, seed an open run through the existing E2E API, wait for
  the causally matched `automation.summaries` WebSocket response, and assert
  that the same row shows the animated indicator without navigation or reload.
- Run the spec in the `chromium` project. The component is not mounted by the
  `mobile-chrome` layout, and the change does not alter any shared mobile
  component.

## Work orders

- [x] [Task 01: Restore the live sidebar indicator](task-01-live-sidebar-indicator.md)

## Verification results

- PASS `pnpm --filter @kandev/web exec vitest run components/app-sidebar/sections/automations-section.test.tsx components/runs/use-live-refresh.test.ts` (22 tests).
- PASS fixup regression suite covering the responsive visibility and refresh-error cases with the same targets plus `components/runs/use-automation-summaries.test.ts` (30 tests).
- PASS `pnpm --filter @kandev/web run typecheck`.
- PASS targeted ESLint for the changed sidebar component, component test, summary hook, and summary-hook test.
- PASS `pnpm e2e:run --project chromium tests/automations-sidebar.spec.ts` (4 tests).
- PASS `python3 scripts/lint-spec-files.py --all`.

## Risks

- The E2E stimulus must occur after the initial summary response so the next
  causally matched response represents the live refresh, not first load.
- A visible idle section will now perform the same 10-second summary read that
  running automation pages already use. The query is workspace-bounded and the
  timer is disabled whenever the section is not visible.
- The visual branch must not remove the localized state text or change the row
  geometry.
