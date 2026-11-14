---
id: "05-import-progress-ui"
title: "Show import progress in settings"
status: done
wave: 4
depends_on:
  - "04-import-lifecycle"
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-IMPORT-003
acceptance_criteria:
  - AC-COSTS-USAGE-IMPORT-003.1
  - AC-COSTS-USAGE-IMPORT-003.2
  - AC-COSTS-USAGE-IMPORT-003.3
  - AC-COSTS-USAGE-IMPORT-003.4
  - AC-COSTS-USAGE-IMPORT-003.5
  - AC-COSTS-USAGE-IMPORT-003.6
  - AC-COSTS-USAGE-IMPORT-003.7
system_design:
  - ../../specs/costs/system-design/usage-history-import.md
---

# Task 05: Show import progress in settings

## Summary

Extend `historicalImportView` and the settings card so users see the step, day progress, session counts, an estimate, and the right action for each state. Use `/mobile-parity` for the phone view.

## In scope

- The status contract from the design, including `eta_seconds`.
- `makeHistoricalImportSettings`: determinate progress, counts, states, actions, and the live region.
- `IMPORT_TRANSLATIONS` in `en` and `pt-pt`.

## Out of scope

- Workspace display names. The card keeps the identifiers `host.context` provides.
- Installed-Host browser coverage, which Task 06 owns.

## Acceptance

- Rendering each status produces the state and action shown in UI-02, and a queued status names no other workspace.
- The progress bar exposes value and maximum during the daily step, and no estimate appears before 3 completed days.
- The card renders the same content in a phone viewport, with 44 px actions.

## ASCII UI preview

UI-01 and UI-02 from the [plan](plan.md#ascii-ui-preview). AC-003.1 to .7. Structure and actions are requirements. Copy and numbers are illustrative.

```text
UI-01: daily step
+----------------------------------------------------------+
| Import historical usage                                  |
| Workspace  [ Acme                                  v ]   |
| Step 2 of 2: Adding daily history                        |
| [==================>                  ] 142 of 238 days  |
| Working on 2026-03-12 - about 40 min left                |
| Sessions found 312 | Imported 287                        |
| No local transcript 21 | Skipped (not safe) 4            |
| All-time totals are ready. Charts fill in as days finish.|
| [ Cancel import ]                                        |
+----------------------------------------------------------+

UI-02: other states
Queued     Waiting for another import to finish.   [ Cancel import ]
Failed     Import stopped on 2026-03-12.           [ Resume ]
Completed  Imported 287 of 312 sessions.           [ Import again ]
```

## Verification

```bash
(cd "$PLUGIN_ROOT" && go test ./server/ -run 'TestImportStatus' -count=1)
(cd "$PLUGIN_ROOT" && node --test test/bundle.test.mjs)
(cd "$PLUGIN_ROOT" && node --check ui/bundle.js)
```

## Files likely touched

- `server/import.go`
- `server/import_status_test.go` (new)
- `ui/bundle.js`
- `test/bundle.test.mjs` (import-card cases)

## Dependencies

Task 04.

## Risks

- `@kandev/ui/progress` treats `value` as a percentage, not a completed count. The card passes a rounded percent with `max: 100` and keeps the day text in `aria-valuetext`.
- Status polling must not replace the current status with a spinner or the live region repeats. The card keeps the last status visible while a poll is in flight.

## Parallelism

`sequential`

## Inputs

- [Plan preview](plan.md#ascii-ui-preview).
- [System design](../../specs/costs/system-design/usage-history-import.md), Status contract and Settings card.

## Results

Extended `historicalImportView` to the closed status contract and added the settings-card states, actions, session counts, determinate daily progress, current day, and after-three-days estimate.
Queued status names no other workspace; failed and disabled states offer Resume; completed and cancelled states offer Import again.
Status text is a polite live region, polling keeps the last status visible, and the card uses full-width 44 px actions on phones. The daily progress value is a rounded percent because `@kandev/ui/progress` treats `value` as a percentage; `aria-valuetext` keeps the completed-of-total day text. Completed cards do not show the in-progress chart note, and `undated` is set only when an imported target has no date range. A disabled card keeps Resume enabled so collection can be re-enabled.
English and Portuguese import translations cover the new copy.

Validation: `go test ./server/ -run TestImportStatus -count=1`, `node --check ui/bundle.js`, and `node --test test/bundle.test.mjs` passed. The bundle cases cover every action state, queued privacy, determinate percent with 142/238 days, counts, estimate, and the daily-only completed copy.
