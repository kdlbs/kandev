---
id: "02-reveal-restoration"
title: "Preserve command reveal during portal restoration"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001
acceptance_criteria:
  - AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.2
  - AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.3
  - AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.5
  - AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.6
system_design:
  - ../../specs/ui/system-design/command-panel-sidebar-task-reveal.md
---

# Task 02: Preserve command reveal during portal restoration

## Scope

Release pending portal restoration before explicit command navigation.
Wait for stable visible row geometry before the transient cue.
Allow one CSS pixel of browser scroll rounding at viewport boundaries.
Preserve smooth nearest scrolling, reduced-motion behavior, and cancellation.
If layout displaces a row after it entered view, permit another scroll within the same frame budget.
Do not restart a pending scroll on every frame.

## Evidence

PR #4007 CI and the first consolidated desktop run both failed the above-viewport cue check.
Browser instrumentation showed the row at 227.953125px against a viewport top of 228px.
The exact comparison prevented the cue after successful scrolling.
A new fractional-boundary unit case failed before the correction.
The corrected browser suite passed both overflow directions and the guarded-navigation case.

## Verification

Sidebar unit cases cover stable geometry, rounding, cancellation, and bounded failure.
Review regressions cover displacement after initial visibility and a pending scroll that never settles.
Portal unit cases cover active release, imminent restoration, and release expiry.
Desktop browser cases retain cue, row containment, active state, and document-scroll checks.
The phone command-navigation cases retain direct routing and hidden-sidebar behavior.
