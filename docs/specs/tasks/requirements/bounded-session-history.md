---
status: active
system: tasks
created: 2026-09-14
owners:
  - kandev
---

# Bounded session history storage Requirements

## Overview

Task conversations accumulate large tool-call payloads, Git snapshots, and
plan revisions. Unbounded inline storage and full-row history reads make
long-lived installations slow: normal transcript hydration pays for data it
does not render, and duplicate snapshots grow the database without bound.
Conversations themselves must remain preserved by default.

## Requirements

### REQ-TASKS-BOUNDED-SESSION-HISTORY-001: Cursor-bounded history hydration

**Intent:** Normal session-history reads shall stay responsive by fetching
only the requested window and never eagerly materializing large tool
metadata.

#### Acceptance criteria

- **AC-TASKS-BOUNDED-SESSION-HISTORY-001.1:** Session history list responses
  shall accept and return a stable cursor so each hydration reads only the
  requested window.
- **AC-TASKS-BOUNDED-SESSION-HISTORY-001.2:** Normal history hydration shall
  return lightweight tool metadata without the full payload; the stored
  detail of a retained tool payload shall be returned only by a scoped
  explicit request.
- **AC-TASKS-BOUNDED-SESSION-HISTORY-001.3:** A reader on a separate pool
  shall observe committed writer updates, so paginated history stays fresh
  across readers and writers.

### REQ-TASKS-BOUNDED-SESSION-HISTORY-002: Digest-backed operational payload storage

**Intent:** Large operational payloads shall be stored once, verifiably, and
retainably without enlarging every transcript row.

#### Acceptance criteria

- **AC-TASKS-BOUNDED-SESSION-HISTORY-002.1:** A tool payload exceeding the
  configured inline threshold shall be compressed or externalized with a
  recorded digest, encoding, byte size, and internal reference; metadata
  keyed by payload identity shall be deduplicated between sessions.
- **AC-TASKS-BOUNDED-SESSION-HISTORY-002.2:** Explicit payload detail loading
  shall verify the recorded integrity digest before returning content.
- **AC-TASKS-BOUNDED-SESSION-HISTORY-002.3:** Human and agent conversational
  messages shall remain preserved by default; retention selection never
  implicitly deletes conversation history.
- **AC-TASKS-BOUNDED-SESSION-HISTORY-002.4:** Equivalent Git snapshots within
  the same session and content-digest group shall deduplicate; snapshots from
  different sessions remain distinct history.
- **AC-TASKS-BOUNDED-SESSION-HISTORY-002.5:** Retention selection shall
  identify superseded tool payloads, redundant live and status Git snapshots,
  and obsolete plan revisions only as reportable candidates; superseded
  selection respects configured retention and is non-destructive until
  maintenance executes.

## Out of scope

- The operator maintenance command and its backup gating, owned by the
  [system page](../system-page/requirements/database-maintenance-command.md).
- Projection contention, owned by the
  [platform system](../platform/requirements/task-summary-contention.md).
