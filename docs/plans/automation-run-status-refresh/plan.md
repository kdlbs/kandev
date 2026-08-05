---
spec: docs/specs/office/automation-runs.md
created: 2026-08-05
status: draft
---

# Implementation Plan: Keep automation run status live

## Overview

Keep the automation detail rail and mobile drawer current after a run starts
and finishes. The existing server-reported open-run count remains authoritative
for runs outside the loaded history window, while a visible Running row also
keeps the shared detail refresh loop alive when the count temporarily reports
zero.

## Frontend

### Automation detail refresh gate

Update `apps/web/components/runs/automation-detail-page.tsx` to treat either a
positive `openRuns` count or a visible run whose status is open as a reason to
keep `useLiveRefresh` active. Preserve the existing bounded `settling` window
after Run now so a newly-created row is discovered even before either signal is
present. This is shared by the desktop `RunsRail` and mobile `RunsDrawer`.

## Tests

### Visible open run with an empty summary count

- **What:** A detail page with a visible `task_created` row and
  `open_runs: 0` refreshes and moves that row to Completed when the next API
  response is terminal.
- **File:** `apps/web/components/runs/automation-detail-page.test.tsx`
- **How:** Render with controlled sequential `listAutomationRuns` responses,
  use fake timers to advance `LIVE_REFRESH_INTERVAL_MS`, and assert the row
  changes groups without a manual refresh.

### Existing refresh contracts

- **What:** Server-reported open runs outside the capped history window still
  keep polling, and idle pages make no repeat requests.
- **Files:** `apps/web/components/runs/automation-detail-page.test.tsx`,
  `apps/web/components/runs/use-live-refresh.test.ts`,
  `apps/web/components/runs/use-automation-activity.test.ts`
- **How:** Run the focused frontend suite; existing tests protect both sides of
  the refresh gate.

## E2E Tests

- **Scenario:** The shared detail surface keeps the run switcher available on
  mobile while status data refreshes through the same page hook.
- **File:** `apps/web/e2e/tests/mobile-automation-detail.spec.ts`
- **What to verify:** Keep the existing mobile drawer and Run now coverage as
  the mobile-path evidence. No new mobile scenario is required because this
  repair changes state freshness only; it does not change composition, touch
  targets, scrolling, or navigation.

## Verification Results

Pending implementation.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [ ] [task-01-live-run-status](task-01-live-run-status.md)

## Open Questions

None.
