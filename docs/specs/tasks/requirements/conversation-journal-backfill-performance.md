---
status: active
system: tasks
created: 2026-09-17
owners:
  - kandev
---

# Conversation Journal Backfill Performance Requirements

## Overview

On startup, Kandev runs a one-time backfill that pages through every existing
`task_session_turns` and `task_session_messages` row not yet present in the
conversation journal (`conversation_turn_versions` / `conversation_message_versions`)
and journals it, so pre-existing sessions gain the same durable conversation
history as sessions created after the journal shipped. An operator upgrading an
installation with a large amount of history depends on this backfill finishing
in bounded time; a backfill that degrades with corpus size turns a routine
upgrade into an extended startup outage with no visible progress.

## Terminology

- **Backfill batch:** One bounded page (100 rows) of turns or messages that the
  backfill selects, journals, and commits in a single transaction before
  requesting the next page.
- **Keyset comparison:** The `WHERE` clause term the backfill uses to select
  the next batch strictly after the last row processed, without re-scanning
  earlier rows.

## Requirements

### REQ-TASKS-JOURNAL-BACKFILL-001: Backfill batch selection scales with row count, not corpus size

**Intent:** Each backfill batch must cost roughly the same regardless of how
many rows the backfill has already processed, so total backfill time scales
with the size of the remaining corpus rather than growing per batch as the
corpus grows.

#### Acceptance criteria

- **AC-TASKS-JOURNAL-BACKFILL-001.1:** When the backfill selects the next batch
  of turns or messages, the system shall use a keyset comparison that lets the
  database seek the existing per-session index directly to the next batch,
  instead of collecting and sorting every remaining row in the table.
- **AC-TASKS-JOURNAL-BACKFILL-001.2:** When two or more turns, or two or more
  messages, within the same session share an identical timestamp and a batch
  boundary falls inside that group, the system shall still journal every one
  of those rows exactly once, in the same relative order as before, without
  omitting or duplicating any row.

## Out of scope

- Changing the conversation journal's schema, retention, or trigger-based
  write path for newly created turns and messages.
- The one-time plugin session-event mirror backfill
  (`syncAllCommittedSessionEvents`); its performance is not covered here.
- Making the two per-session indexes (`idx_turns_session_started`,
  `idx_messages_session_created`) fully cover the keyset comparison by adding
  `id` as a third column; that is index-tuning that follows this requirement,
  not part of satisfying it.
