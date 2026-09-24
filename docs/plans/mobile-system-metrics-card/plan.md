---
created: 2026-09-23
status: complete
requirements:
  - REQ-UI-APP-STATUS-BAR-001
system_design:
  - ../../specs/ui/system-design/app-status-bar.md
legacy_specs: []
---

# Implementation Plan: Mobile System Metrics Card

## Overview

Group built-in phone metrics into one compact card. The previous composition gave Host its own full-height row and spread three default readings over two rows. The user authorized implementation and PR publication in this session without a planning handoff.

## Scope and approach

Update `StatusSurfaceMetrics` only: move the host badge beside the heading, render three equal metric columns, and give detailed readings visible translated labels and meters below their values. Retain all enabled metrics, existing formatting, thresholds, tooltips, subscription gates, and simplified presentation. Plugin contributions and backend collection are out of scope.

## Mobile design contract

The existing Menu fallback and Status drawer remain the entry points. The nearest exemplars are `AppNavSheet` and `AppStatusDrawer`: inset drawers, fixed headings, safe-area clearance, and one internal scroller. This passive summary belongs inline in those temporary surfaces. It introduces no new action or navigation layer. Metric cells retain at least 44 px height and wrap labels within their columns; the shared store and subscription remain authoritative.

## ASCII UI preview

UI-01: Phone Menu or Status, detailed metrics loaded (AC-UI-APP-STATUS-BAR-001.2, .6, .7):

```text
+-------------------------------------+
| SYSTEM METRICS                 Host  |
| CPU          Memory         Disk    |
| 3%           52%            72%     |
| [meter]      [meter]        [meter]  |
+-------------------------------------+
```

Three equal columns, a shared heading line, and labels above values are structural. Values and spacing are illustrative. Additional metrics continue on another row; simplified mode omits the host, labels, and meters. The parent drawer owns scrolling.

UI-02: Desktop remains inline:

```text
| Host  [CPU meter] 3%  [Memory meter] 52%  [Disk meter] 72% |
```

## Tests and work orders

- Existing `components/system-metrics` unit tests retain subscription, loading, source filtering, and simplified-mode coverage.
- `mobile-resource-metrics-display.spec.ts` proves the five-metric grid, default readings in Menu at 320/393/767 px, the 768 px desktop fallback, containment, scrolling, and simplified settings persistence.
- `resource-metrics-display.spec.ts` proves compact desktop alignment and simplified settings persistence.
- [x] [Task 01: Group mobile system metrics](task-01-group-mobile-system-metrics.md).

## Verification results

RED: the production-build phone regression failed on expected two rows versus the existing three rows for five readings. GREEN: 11 system-metrics unit tests, 3 mobile Playwright tests, and 2 desktop Playwright tests passed against the rebuilt SPA. The final mobile run also verifies that every enabled label can be scrolled into the viewport. Targeted ESLint, Prettier, typecheck, i18n ratchet, specification validation, and public-doc validation passed. Fresh isolated phone and desktop screenshots were captured and inspected.

## Risks

Long translated labels must wrap within the narrow columns. Desktop density limits, unavailable values, and simplified metrics must retain their existing behavior.

## PR validation follow-up

PR #3880 review identified two documentation corrections: state explicitly that the phone Menu fallback applies when Show status bar is off, and link this plan from the status-bar design. Both are addressed.

CI also exposed a terminal context-reset race in `terminal-agent.spec.ts`. The same assertion reproduced without retries using the CI runtime image and backend artifacts. A fresh PTY's startup-ready callback could race the next running-state publication and suppress that turn's completion. The lifecycle reset now waits for first idle outside the passthrough lifecycle lock before returning. This is an internal ordering correction to the existing workflow reset contract; it does not change metrics collection or plugin behavior.

Two focused lifecycle regressions failed before the fix. All five reset tests passed with `-race -count=20`; the complete lifecycle package passed with `-race`, and changed-code Go lint reported zero issues. Terminal browser validation and the subsequent CI check are recorded in the task results.
