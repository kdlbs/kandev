---
status: active
system: ui
created: 2026-07-30
owners:
  - cfl
---
# Transcript Auto-scroll Stability Requirements

## Overview

Users who turn off transcript auto-scroll expect the visible conversation to stay fixed while new content arrives. Chrome can still adjust the view through its native overflow anchoring when the toggle is disabled from the bottom. Separately, the clarification-recovery regression test must reliably observe the asynchronous hand-off it is designed to protect.

## Requirements

### REQ-UI-TRANSCRIPT-AUTO-SCROLL-001: Transcript Auto-scroll Stability

**Intent:** Users who turn off transcript auto-scroll expect the visible conversation to stay fixed while new content arrives. Chrome can still adjust the view through its native overflow anchoring when the toggle is disabled from the bottom. Separately, the clarification-recovery regression test must reliably observe the asynchronous hand-off it is designed to protect.

#### Acceptance criteria

- **AC-UI-TRANSCRIPT-AUTO-SCROLL-001.1:** A disabled native transcript auto-scroll preference prevents browser scroll anchoring from moving the transcript when appended content arrives, including when the user disabled it while already at the bottom.
- **AC-UI-TRANSCRIPT-AUTO-SCROLL-001.2:** Re-enabling auto-scroll retains the existing catch-up behavior.
- **AC-UI-TRANSCRIPT-AUTO-SCROLL-001.3:** The clarification-recovery concurrency regression waits for its asynchronous completion signal within a bounded interval instead of treating scheduler timing as a product failure.
- **AC-UI-TRANSCRIPT-AUTO-SCROLL-001.4:** **GIVEN** an overflowing native transcript at its bottom with auto-scroll disabled, **WHEN** a new message is appended, **THEN** the transcript's `scrollTop` remains at the pre-append position.
- **AC-UI-TRANSCRIPT-AUTO-SCROLL-001.5:** **GIVEN** an overflowing native transcript with auto-scroll enabled, **WHEN** new content arrives, **THEN** the transcript remains pinned to the bottom.
- **AC-UI-TRANSCRIPT-AUTO-SCROLL-001.6:** **GIVEN** clarification recovery has accepted its retry prompt, **WHEN** its asynchronous dispatch is scheduled, **THEN** the recovery call completes before the intentionally blocked prompt is released.

## Migrated source detail

## Why

Users who turn off transcript auto-scroll expect the visible conversation to
stay fixed while new content arrives. Chrome can still adjust the view through
its native overflow anchoring when the toggle is disabled from the bottom.
Separately, the clarification-recovery regression test must reliably observe
the asynchronous hand-off it is designed to protect.

## What

- A disabled native transcript auto-scroll preference prevents browser scroll
  anchoring from moving the transcript when appended content arrives, including
  when the user disabled it while already at the bottom.
- Re-enabling auto-scroll retains the existing catch-up behavior.
- The clarification-recovery concurrency regression waits for its asynchronous
  completion signal within a bounded interval instead of treating scheduler
  timing as a product failure.

## Scenarios

- **GIVEN** an overflowing native transcript at its bottom with auto-scroll
  disabled, **WHEN** a new message is appended, **THEN** the transcript's
  `scrollTop` remains at the pre-append position.
- **GIVEN** an overflowing native transcript with auto-scroll enabled,
  **WHEN** new content arrives, **THEN** the transcript remains pinned to the
  bottom.
- **GIVEN** clarification recovery has accepted its retry prompt,
  **WHEN** its asynchronous dispatch is scheduled, **THEN** the recovery call
  completes before the intentionally blocked prompt is released.

## Out of scope

- Changing the transcript toggle's location, labels, or persisted preference.
- Changing clarification recovery production behavior or ownership rules.
