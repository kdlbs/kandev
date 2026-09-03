---
status: active
system: ui
created: 2026-07-30
updated: 2026-09-02
owners:
  - cfl
---

# Transcript Auto-scroll Stability Requirements

## Overview

Users expect an enabled transcript to follow the newest message, including when
they activate a long-running session tab that was mounted outside the visible
Dockview layout. Users who turn off transcript auto-scroll expect the visible
conversation to stay fixed while new content arrives. Chrome can still adjust
the view through its native overflow anchoring when the toggle is disabled from
the bottom. Separately, the clarification-recovery regression test must reliably
observe the asynchronous hand-off it is designed to protect.

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
- **AC-UI-TRANSCRIPT-AUTO-SCROLL-001.7:** **GIVEN** an enabled transcript that is
  pinned to the bottom, **WHEN** streamed content commits, **THEN** the
  transcript remains pinned without a synchronous content-size read in the
  message-commit path.
- **AC-UI-TRANSCRIPT-AUTO-SCROLL-001.8:** **GIVEN** an enabled, overflowing
  transcript that was mounted while its desktop session tab was inactive,
  **WHEN** the user activates that tab after initial load or page refresh,
  **THEN** the visible transcript settles at its newest message.
- **AC-UI-TRANSCRIPT-AUTO-SCROLL-001.9:** **GIVEN** an enabled transcript that
  was following the newest message before its desktop session tab became
  inactive, **WHEN** content arrives while the tab is inactive and the user
  activates it again, **THEN** the transcript catches up to the newest message.
- **AC-UI-TRANSCRIPT-AUTO-SCROLL-001.10:** **GIVEN** a transcript whose
  auto-scroll preference is disabled or whose reader moved away from the newest
  message, **WHEN** its desktop session tab becomes inactive and visible again,
  **THEN** the transcript preserves the reader-owned position instead of
  forcing the newest message into view.

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
- Activating an enabled desktop session transcript after initial load, refresh,
  or hidden message delivery places a bottom-following reader at the newest
  message after the panel becomes measurable.
- Activating a reader-owned transcript position preserves that position.
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
- **GIVEN** a task with two overflowing session transcripts, **WHEN** the page
  refreshes with one session inactive and the user activates that session,
  **THEN** the session opens at the newest message when auto-scroll is enabled.
- **GIVEN** a bottom-following session receives content while its desktop tab is
  inactive, **WHEN** the user activates the session, **THEN** its transcript
  catches up to the newest message.
- **GIVEN** the reader disabled auto-scroll or scrolled away from the newest
  message before switching desktop session tabs, **WHEN** the reader returns,
  **THEN** the prior reading position remains visible.

## Out of scope

- Changing the transcript toggle's location, labels, or persisted preference.
- Changing clarification recovery production behavior or ownership rules.
