---
status: active
system: platform
created: 2026-08-23
updated: 2026-08-27
owners:
  - kandev
---

# Browser Console Retention Requirements

## Overview

Kandev keeps bounded browser diagnostics for explicit support bundles. Logging
must remain off the application-critical path. Retention work must not consume
large amounts of CPU when the store is already within its limits.

## Terminology

- **Committed batch:** One group of staged entries that IndexedDB accepts in a
  successful write transaction.
- **Retained history:** All browser log entries across identity partitions in
  one browser profile.

## Requirements

### REQ-PLATFORM-BROWSER-CONSOLE-RETENTION-001: Bounded incremental retention

**Intent:** Preserve exact diagnostic limits without scanning the complete
retained history after every log batch.

#### Acceptance criteria

- **AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.1:** After each committed batch,
  retained history shall contain no entry older than three days, no more than
  10,000 entries, and no more than 20 MiB of serialized entry data.
- **AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.2:** When entries exceed a limit,
  retention shall remove expired entries first and shall then remove the oldest
  entries until all limits hold.
- **AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.3:** When multiple tabs write to
  the same browser profile, every committed transaction shall preserve the
  shared count and byte limits.
- **AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.4:** After retention totals are
  initialized, a batch that is within all limits shall not scan unexpired
  retained entries.
- **AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.5:** When log batches arrive while
  persistence is active in one tab, that tab shall run no more than one browser
  log write at a time.
- **AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.6:** A console call shall not wait
  for IndexedDB work. If persistence fails, the bounded memory fallback and
  diagnostic loss metadata shall remain available.
- **AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.7:** Clearing browser diagnostic
  history shall also reset its retained count and byte totals.
- **AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.8:** Entries emitted during one
  250 ms collection window shall use the fewest committed batches allowed by
  the 50-entry and 256 KiB batch limits. Browser idle time shall not shorten
  that collection window.
- **AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.9:** An explicit diagnostic
  snapshot shall bypass an outstanding collection window, flush all staged
  entries, and wait for the existing serialized drain.

## Compatibility and persistence

- The existing database name and retained entries survive the schema upgrade.
- Entries remain partitioned by the authenticated Kandev identity.
- Each entry remains limited to 64 KiB.
- A diagnostic snapshot returns only the requested identity partition.

## Out of scope

- Changing bundle upload limits or permissions.
- Uploading browser logs continuously.
- Adding user-configurable retention limits.
- Changing the 500-entry memory fallback or the staging byte limit.
