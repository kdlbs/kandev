---
status: active
system: ui
created: 2026-10-07
owners:
  - kandev
---

# Transcript history freshness requirements

## Overview

History refresh must keep already observed message outcomes usable while still
recovering newer server results. UI owns this reusable client transcript
projection contract. Persisted message authority remains with Tasks, and
subscription readiness remains with Platform. This capability complements
[history visibility](task-prompt-transcript-visibility.md): row revision
selection is distinct from conversation boundaries and upward navigation.

## Terms

- **History refresh:** A bounded latest-message read used on entry, recovery,
  foreground return, running backfill, or turn settlement.
- **Comparable revision:** Both copies of a message supply valid update times
  that identify their relative freshness.
- **Concurrent change:** The cached message changed or arrived after the shared
  history request began, before its response was accepted.

## Requirements

### REQ-UI-TRANSCRIPT-HISTORY-FRESHNESS-001: Preserve observed message outcomes

**Intent:** A routine refresh cannot hide an observed tool result or restore its
older running status; newer server results must still reach the transcript.

#### Acceptance criteria

- **AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.1:** When history returns an older
  comparable revision of a message already present in the transcript, the
  transcript shall retain the newer cached content, result, and status together,
  including a completed live tool result observed while the read was pending.
- **AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.2:** When history returns a newer
  comparable revision, the transcript shall apply its content, result, and
  status together, whether or not a concurrent cached change occurred.
- **AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.3:** When revisions tie or cannot be
  compared and a concurrent change occurred, the transcript shall retain that
  changed cached message. Without a concurrent change, equal revisions or an
  unversioned cache shall permit history hydration; an unversioned history row
  shall not replace a cache with a valid revision.
- **AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.4:** Freshness selection shall not
  change which rows belong to the bounded contiguous history window. Overlap
  shall retain loaded older pages; disjoint windows shall discard stale prefixes
  while retaining eligible new live additions, without duplicate message IDs
  or a cursor that skips the intervening history.
- **AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.5:** Authoritative recovery shall still
  remove absent rows in its replacement interval and clear a successfully
  empty history, retaining pending local messages according to the existing
  recovery contract. Revision selection shall apply to matching IDs without
  preventing those removals.
- **AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.6:** Desktop and phone shall expose the
  same accepted message outcome. Session identity, subscription readiness,
  loading completion, request sharing, retry eligibility, and optimistic
  submission behavior shall retain their existing contracts.

## Exclusions

- Server mutation, persistence, delivery ordering, and event replay changes.
- New API fields, revision counters, global event journals, or reporters.
- Changing around-window or older-page duplicate policies.
- Changes to transcript layout, navigation, scrolling, touch, or localized copy.
- Repairing missing persisted records or guaranteeing timestamps order writes
  across clock regressions.

## Implementation plans

- [Preserve live tool results](../../../plans/preserve-live-tool-results/plan.md)
