---
status: active
system: tasks
created: 2026-10-03
owners:
  - kandev
---

# Partial task plan reads Requirements

## Overview

Agents need to read and change a small part of a large plan without exchanging
the whole document. Fragment writes already exist through exact edit and append;
this extension adds bounded reads and guidance that connects the two operations.
The tasks system owns the contract because it owns plan content and edit versions.

## Terminology

- **Character:** one Unicode code point, not a byte or UTF-16 code unit.
- **Range:** a zero-based character offset and a maximum character count.
- **Edit version:** the existing opaque identity of one committed plan state.

## Requirements

### REQ-TASKS-PLAN-READ-001: Read a bounded exact range

#### Acceptance criteria

- **AC-TASKS-PLAN-READ-001.1:** An authorized agent shall request a range through
  `get_task_plan_kandev` using optional integer `offset` and `limit` arguments.
  Supplying either argument shall enable partial reading; supplying neither
  shall preserve the existing full-content read.
- **AC-TASKS-PLAN-READ-001.2:** In partial mode, an omitted offset shall mean zero
  and an omitted limit shall mean 4,096 characters. Offset shall be between zero
  and 9,007,199,254,740,991 inclusive; limit shall be between 1 and 8,192 inclusive.
  Invalid types, fractional values,
  and out-of-bound values shall be rejected without a full-read fallback.
- **AC-TASKS-PLAN-READ-001.3:** A partial response shall contain only the requested
  exact content range, preserving whitespace, line endings, and Unicode text.
  It shall not echo the full plan in metadata or another response block.
- **AC-TASKS-PLAN-READ-001.4:** A partial response shall identify the plan and
  version, total characters and bytes, requested offset, returned characters
  and bytes, whether more text remains, and the next offset when applicable.
  These values shall describe the same committed content snapshot.
- **AC-TASKS-PLAN-READ-001.5:** Offset equal to the total character count shall
  return an empty final range. Offset beyond that count shall be rejected with
  a correction to an in-range offset. Without an expected version, an absent
  plan shall retain the existing no-plan response; with an expected version,
  absence shall be a conflict. A storage failure shall remain distinct from absence.
- **AC-TASKS-PLAN-READ-001.6:** Partial reads shall work on historical oversized
  plan heads without changing the stored-content limit or silently truncating
  the stored document. Each response shall obey the range limit independently
  of total plan size, including plans containing one very long line.

### REQ-TASKS-PLAN-READ-002: Read and edit one coherent version

#### Acceptance criteria

- **AC-TASKS-PLAN-READ-002.1:** `get_task_plan_kandev` shall accept an optional
  nonempty `expected_version` for full or partial reads. A mismatch shall return
  a conflict without returning content or changing state.
- **AC-TASKS-PLAN-READ-002.2:** An agent shall be able to follow the returned next
  offset while requiring the first page's version. Intervening content or title
  edits, including deletion and recreation, shall prevent combining pages from
  different versions. Conflicts shall direct reconciliation rather than an
  automatic retry using a refreshed version.
- **AC-TASKS-PLAN-READ-002.3:** Text and version from a partial response shall be
  usable with the existing exact-edit tool. A uniquely matching fragment shall
  change only that fragment; stale or ambiguous edits shall retain the existing
  safe-edit rejection behavior.
- **AC-TASKS-PLAN-READ-002.4:** Full and partial reads shall retain task reach,
  owner authorization, and existing tool exposure. Unauthorized callers shall
  receive no content, version, length, or pagination metadata. Reads shall never
  change history, comments, implementation state, title, or version, or publish
  mutation events.

### REQ-TASKS-PLAN-READ-003: Guide agents toward small plan exchanges

#### Acceptance criteria

- **AC-TASKS-PLAN-READ-003.1:** Tool discovery and agent-facing plan guidance shall
  explain bounded reads, character units, continuation, and version checks.
  They shall distinguish partial responses from whole documents.
- **AC-TASKS-PLAN-READ-003.2:** Task, Office, planning, and active-plan guidance
  shall recommend exact edits for local changes and append for new sections.
  They shall permit reuse of a current version from a read or successful write
  without requiring an unnecessary full-plan reread.
- **AC-TASKS-PLAN-READ-003.3:** Guidance shall retain full reads when the whole
  plan is needed and shall prohibit submitting a partial read as a replacement
  document. Existing user-edit preservation, question barriers, task/session
  identity, title ownership, completion gates, autopilot, delegation, and final
  action rules shall remain in force.

## Related contracts

- [Safe agent plan edits](plan-safe-edits.md) owns conditional writes, compact
  acknowledgements, exact matching, and recovery.
- [Append mode](plan-write-append-mode.md) owns append composition and retries.
- [Plan content size limit](plan-content-size-limit.md) owns storage ceilings.

## Out of scope

- New write modes, batched edits, fuzzy matching, section parsers, and search.
- Revision-body pagination, browser document pagination, or rendered UI changes.
- Automatic summarization, append deduplication, and agent continuation changes.
- Persistence migrations, cross-process writer coordination, and token metering.

## Implementation Plans

- [Partial plan reads and fragment guidance](../../../plans/plan-partial-reads/plan.md)
