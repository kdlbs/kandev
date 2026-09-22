---
status: active
system: cross-workspace-task-handoff
created: 2026-09-02
updated: 2026-09-15
owners:
  - nova28
---

# Reverse-link integrity Requirements

## Overview

The source task's reverse-link list is written by a key-scoped compare-and-set
append, never a read-modify-write of the whole task, and is repaired — not
recreated — on a replay. Corrupt or conflicting stored data is never silently
discarded, coerced, or overwritten: this list is a durable index the caller
depends on, and a caller-visible failure that leaves the data intact and
retryable is preferable to one that quietly loses provenance.

### REQ-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001: Repair-on-replay, ownership, and readability

#### Acceptance criteria

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001.1:** On either
  found-task outcome, the action shall still ensure the source task's
  reverse-link entry for that delivery task exists: adding it when absent,
  leaving it untouched when present. This is a source-side write, in the
  source workspace, to a task the shared idempotency mechanism knows nothing
  about, and it turns an identical replay into a repair for a call that
  created the task and then failed before recording the reverse link.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001.2:** A repaired entry's
  handoff timestamp shall be read from the found task's own stored forward
  record, not from the repairing call's clock — the forward record is
  write-once and was never re-stamped, so the value already stored there is
  the one true timestamp for that handoff. A repair therefore carries an
  older timestamp than entries appended after it, and the stored list's
  sort order (criterion .6) is what keeps it correctly placed rather than
  assumed to belong at the end.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001.3:** Before performing a
  repair, the action shall compare the found task's stored source-task id
  against the calling source task's own id. When they differ, or when the
  found task carries no forward-provenance record at all, the action shall
  refuse with HTTP 400, shall write no reverse-link entry, and shall state
  that the external id is already held by a task this source did not hand
  off. Per-workspace uniqueness of `external_id` means two different source
  tasks can otherwise choose the same key; without this check, the second
  source would acquire a reverse link to a delivery task whose forward
  record names someone else.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001.4:** Passing the
  ownership check in criterion .3 does not make the found task's stored
  forward record usable, so the action shall additionally verify that its
  handoff timestamp is present, is a string, and parses as an RFC 3339
  timestamp, before writing a repair. When it is not:
  - and the reverse-link entry for that delivery task is absent, the action
    shall make no write, shall report `reverse_link_recorded: false` with a
    `reverse_link_error` naming the unreadable timestamp, and the response's
    handoff timestamp shall be empty;
  - and the reverse-link entry for that delivery task is already present,
    the action shall report `reverse_link_recorded: true` with no
    `reverse_link_error` — nothing failed on this call — while the response's
    handoff timestamp is still empty, because the instant itself remains
    unreadable.

  In neither case shall the action substitute a fresh timestamp, write an
  entry omitting the field, repair the found task's forward record, or
  retry. Writing an entry with a missing or unparseable timestamp anyway
  would create exactly the corruption criterion .5 refuses to read, leaving
  the source task unable to hand off again until a human repairs it — the
  outcome this criterion exists to prevent the action from causing itself.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001.5:** The append shall be
  a key-scoped compare-and-set restricted to the reverse-link key alone:
  read the current value, compute the appended list, and write only that key
  conditionally on the read value still being current, retrying from a fresh
  read on conflict, bounded at 5 attempts. The write shall touch no other
  metadata key, and a concurrent write to a different key shall never cause
  a spurious conflict. Two concurrent handoffs from one source task shall
  both be durably recorded; this guarantee does not extend to a
  read-modify-write of the whole task's metadata performed by an unrelated
  writer racing this compare-and-set — that residual is accepted, and the
  repair-on-replay behaviour in criterion .1 is its mitigation. An absent
  reverse-link key shall compare equal to an empty list, so a source task's
  first handoff is an ordinary append and two concurrent first handoffs
  still cannot lose one.

  The stored list is corrupt, and shall never be overwritten, when it is
  present but not an array, or when it is an array containing an entry that
  is not an object, has a missing or non-string-non-empty delivery-task id,
  or has a `handed_off_at` that is missing or does not parse as RFC 3339.
  That list of corruption conditions is exhaustive: an entry carrying
  additional unknown fields is well-formed, is preserved unchanged, and is
  never refused on that basis alone, and an empty array is well-formed. On
  either corrupt shape the action shall make no write, shall surface a
  partial-failure result (`reverse_link_recorded: false` with a
  `reverse_link_error` naming the corruption), and shall not retry, drop,
  coerce, normalize, or re-sort the malformed data.

  The write's possible outcomes are ranked in one strict order, so no call
  ever has to decide between two applicable outcomes: the source task no
  longer exists outranks corrupt data, which outranks a stale compare value,
  which outranks any other write failure. A missing source task and corrupt
  data are both non-retryable partial failures; a stale compare value is the
  only retryable case, bounded at 5 attempts, after which it too becomes a
  partial failure rather than a silent drop.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001.6:** The reverse-link
  list shall be idempotent by delivery-task id — an entry for a given task
  id shall appear at most once regardless of replay count — and shall be kept
  sorted by handoff timestamp ascending, with delivery-task id ascending as
  the tiebreak for equal timestamps, re-established on every append so the
  stored array is always sorted and a reader needs no sort of its own.
  Insertion order is not relied on for either property, and a repair
  (criterion .2) can legitimately sort ahead of entries appended earlier.
  Both properties are evaluated only after the corruption check in criterion
  .5 has passed; a corrupt list is refused before either is computed.
