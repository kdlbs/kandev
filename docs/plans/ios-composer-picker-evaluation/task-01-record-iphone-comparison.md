---
id: "01-record-iphone-comparison"
title: "Record iPhone comparison"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMPOSER-OVERLAY-001
acceptance_criteria:
  - AC-UI-COMPOSER-OVERLAY-001.1
  - AC-UI-COMPOSER-OVERLAY-001.2
  - AC-UI-COMPOSER-OVERLAY-001.4
system_design:
  - ../../specs/ui/system-design/composer-suggestion-overlays.md
---

# Task 01: Record iPhone comparison

## Summary

Use the seeded current/candidate comparison on the reporting iPhone to identify
whether menus are active but misplaced, absent despite active suggestions, or
not activated until keyboard dismissal. Keep production unchanged.

## In scope

Current and candidate `@mobile` and `#MOB` flows, before-Done geometry, touch
selection and focus. The test proxy records the necessary measurements.

## Out of scope

Permanent UI changes, real providers, main-instance interaction, broad suites.

## Acceptance

1. Capture both modes on the actual iOS keyboard, with the full iOS version.
2. Classify the failure using suggestion state plus menu/viewport rectangles.
3. Record whether each candidate picker is selectable before Done; do not
   infer success from Chromium or a menu appearing after keyboard dismissal.

## Verification

Open `http://100.105.155.17:48762/__picker_eval/` on the reporting iPhone and
follow the three displayed steps in candidate mode, then current mode.
Inspect the recorded geometry without printing unrelated chat content:

```bash
jq -c '{at,browserId,mode,event,viewport,composing,suggestions,menus}' \
  /tmp/kandev-picker-eval-I97UqT/phone-events.jsonl
```

## Files likely touched

- Disposable `/tmp/kandev-picker-eval-I97UqT` artifacts and logs.
- This work order and `plan.md`, to record results.

## Dependencies

User access to the reporting iPhone and Tailscale. No desktop inspector needed.

## Risks

Only candidate CSS/geometry differs between modes. A missing DOM menu cannot
be repaired by this prototype and must route back to activation diagnosis.

## Parallelism

sequential

## Inputs

The [plan](plan.md), current overlay requirement, and isolated seed manifest.

## Results

The user confirmed candidate positioning works on 2026-09-11 and authorized
implementation and cleanup. The captured iPhone Safari UA reports
`Version/26.6.1`; its OS token is `18_7`, so do not use that frozen token to
infer the installed iOS release.

With the keyboard open, both modes reported visual viewport offsetTop=321 and
height=336, with the editor DOMRect at y=157. Current mode activated the mention
plugin and loaded a result, but rendered height=0 at CSS top=329 (DOM y=8).
Candidate rendered the populated menu at DOM y=60 with height=89 and CSS
top=381; composer focus remained true and composition was false. The `#` menu
also activated and rendered its empty-query prompt before keyboard dismissal.

This confirms the coordinate-space mismatch, not delayed search or input
composition. Candidate functional success is user-confirmed; selection of a
specific external item is covered by the isolated browser tests, not asserted
from this short phone trace.

The authorized shutdown closed ports 48761, 48762 and 50761. Main port 9998
retained PID 3960526. Evidence values are retained here before temporary
artifacts are cleaned up.

Cleanup completed: the exact evaluation directory and two task-owned diagnostic
ZIPs were deleted. Raw artifacts are no longer available; the measurements
above are the retained evidence. No main-instance data was removed.
