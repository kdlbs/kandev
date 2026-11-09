---
created: 2026-10-28
status: done
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-004
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
  - ../../specs/ui/system-design/session-refresh-efficiency.md
legacy_specs: []
---

# Workbench interaction correctness

Extract contribution 4 from the reviewed dense-workbench-performance package.
The source is frozen at `a659c453d3bcb67ca790a588f500f35b068dfd09`.
Implementation starts from `d17d84a8084b15ddf1df10ff465a51d27c780e64`.
The original evidence remains in the research branch. This package records
only the extracted correctness changes and their current verification.

## Scope

Restore saved split arrangements even when panel inventories match. Preserve
upward reading through queued native scroll events. Resume following when a
recent wheel reaches its measured bottom despite intervening content growth.
Cancel obsolete scroll saves. Register plan Link and Underline once.

Keep current-main content-reflow safeguards. Exclude shared scroll batching,
transcript edge batching, historical-action memoization, subscription cleanup,
and performance claims. Full-density Firefox End during growth remains open.

## Mobile design contract

Desktop tasks restore independent split trees and drafts. Phone tasks retain
the existing focused Chat and Plan tabs in `task-layout.tsx`. Chat owns transcript
scrolling. Plan owns document scrolling and formatting. The existing bottom
navigation opens each surface. Native touch and keyboard controls keep their
current presentation, focus behavior, safe areas, and primary actions.
Phone navigation never writes desktop split geometry. Existing mobile
navigation and auto-scroll tests verify this shared behavior.

## UI preview

The rendered controls and styles stay the same. Desktop task A restores Chat
beside Files/Changes above Terminal. Task B restores Files beside Chat and
Changes/Terminal. Phone users open one Chat or Plan surface at a time through
the existing bottom navigation. The layout test captures both desktop states;
plan-link tests capture desktop and phone editing.

## Work orders

- [Task 01: Retain interaction fixes](task-01-retain-interaction-fixes.md): done.

## Evidence limits

This contribution proves bounded correctness. Historical Linux measurements do
not establish a whole-view speed gain, M2 behavior, Firefox performance, or a
heap leak repair. Input thresholds and native-event deadlines stay unchanged.
