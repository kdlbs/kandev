---
status: draft
system: office
created: 2026-09-09
owners:
  - kandev
---

# Office Run History Retention Operations Requirements

## Overview

[Run history retention](run-history-retention.md) defines which Office run
history rows may be deleted and how the sweep that deletes them behaves. This
document defines the other half of that contract: what an operator is told
before the first row is removed, what they are told while the tables grow, what
they can configure, and what they can read about the last sweep.

The split follows the ownership boundary the retention contract already draws.
Office owns the rows and the deletion policy because they are Office primitives.
The System pages own the operator surface those decisions are reported through,
and that surface is a separate contract with a separate consumer: an operator
reading a settings page and a health card, rather than a scheduler deleting
rows. Splitting here keeps each document inside its size limit without cutting
either contract.

The requirement IDs continue the retention capability's sequence rather than
starting a new one, because these are the same capability's requirements viewed
from the operator's side.

## Terminology

Terms are defined once, in
[run history retention](run-history-retention.md#terminology), and used here
with the same meaning. The ones this document leans on most:

- **Retention sweep**, **preview**, and **retained count**.
- **Swept tables** (`office_routine_runs`, `runs`), **reported tables** (those
  two plus the three run satellites), and **thresholded tables**
  (`office_routine_runs`, `runs`, `run_events`). "Per table" always names one of
  these three sets, never an unqualified "table".

## Requirements

### REQ-OFFICE-RUN-HISTORY-RETENTION-003: Warn before deleting, and warn while growing

**Intent:** An operator learns what retention is about to remove before it
removes anything, and learns that history is growing past a threshold while
there is still time to widen the window or fix the routine.

As an operator upgrading an install with a year of run history, I want to be
told what the first sweep would delete before it deletes it, so that I can widen
the retention window first if that history matters to me.

#### Acceptance criteria

- **AC-OFFICE-RUN-HISTORY-RETENTION-003.1:** The first evaluation of each swept table
  on a database shall be a preview for that table: it evaluates the policy,
  reports the number of rows it would delete from that table, and deletes
  nothing from it. A table that has not completed a preview never deletes,
  regardless of what any other table has done.
- **AC-OFFICE-RUN-HISTORY-RETENTION-003.2:** When a preview sweep would delete
  at least one row, the system shall emit an operator-visible warning naming
  each table, its would-delete count, and the configured retention window, and
  stating that deletion begins at the next scheduled sweep.
- **AC-OFFICE-RUN-HISTORY-RETENTION-003.3:** When a preview sweep would delete
  no rows, the system shall not emit a warning.
- **AC-OFFICE-RUN-HISTORY-RETENTION-003.4:** The system shall run a preview at most
  once per swept table per database. A table's preview shall be recorded only
  when that table's preview evaluation completed successfully; a table that
  failed during a sweep in which it was being previewed shall be previewed again
  on the next sweep rather than deleting. Disabling and re-enabling retention,
  restarting the backend, or changing any retention setting shall not produce a
  second preview for a table that has already completed one.
- **AC-OFFICE-RUN-HISTORY-RETENTION-003.5:** When a thresholded table's retained
  count exceeds that table's configured warning threshold, the system shall emit
  an operator-visible warning naming the table, its retained count, and the
  threshold. For `office_routine_runs`, the warning shall also name the routine
  holding the largest share of retained rows and that share, where the share is
  that routine's retained rows as a proportion of the table's retained count.
  When two routines hold an equal largest share, the lower routine identifier
  shall be named.
- **AC-OFFICE-RUN-HISTORY-RETENTION-003.6:** When a table reports remaining
  backlog after a sweep, the system shall emit an operator-visible warning
  naming the table, stating that retention is behind, and reporting the number
  of rows deleted in that sweep. A table that was previewed in that sweep shall
  not produce this warning however many rows were eligible, because a preview
  deletes nothing by design and retention is therefore not behind; the preview
  warning of AC-OFFICE-RUN-HISTORY-RETENTION-003.2 reports its eligible count
  instead.
- **AC-OFFICE-RUN-HISTORY-RETENTION-003.7:** When retention is disabled and a
  table's retained count exceeds its warning threshold, the system shall emit
  the same threshold warning and shall additionally state that retention is
  disabled, so a silent unbounded table is distinguishable from one being
  actively managed.
- **AC-OFFICE-RUN-HISTORY-RETENTION-003.8:** Every warning in this requirement
  shall be emitted through a channel available in a production build, and shall
  not be observable only through the debug metrics endpoint.
- **AC-OFFICE-RUN-HISTORY-RETENTION-003.9:** A preview sweep's would-delete
  counts shall be the full eligible count per table, not capped by the batch
  limit, so an operator is told the real size of what is about to be removed.
- **AC-OFFICE-RUN-HISTORY-RETENTION-003.10:** When a swept table's recorded preview
  state cannot be read or parsed, the system shall treat that table as not yet
  previewed and shall emit an operator-visible warning naming the table.
- **AC-OFFICE-RUN-HISTORY-RETENTION-003.11:** Retained counts for the thresholded
  tables shall be produced by a count evaluation that does not require a sweep
  to have run, so the counts and the threshold warnings required by AC-OFFICE-RUN-HISTORY-RETENTION-003.7 and
  AC-OFFICE-RUN-HISTORY-RETENTION-004.8 are available on a fresh install and while retention is disabled. The
  counts shall be refreshed on the sweep interval and reused between refreshes
  rather than recomputed for each operator page load or health poll. The first
  count evaluation shall run at startup, before the delay of
  AC-OFFICE-RUN-HISTORY-RETENTION-002.10 arms the first sweep, so an operator is
  not shown countless tables for a sweep interval after every restart. When a
  count evaluation fails, the system shall keep the counts from the last
  successful evaluation, report them as stale together with the time they were
  produced, and emit an operator-visible warning. A failed count shall not
  suppress the threshold warnings derived from the last successful counts, and
  shall not fail the sweep. Until the first evaluation has completed, and when it
  fails with no earlier successful evaluation to fall back on, the system shall
  report the counts as not yet computed and shall not render them as zero, on the
  same grounds as AC-OFFICE-RUN-HISTORY-RETENTION-004.7: a count nobody has taken
  must not read as a table that is empty. Success and failure shall be tracked
  per thresholded table, so one table's failed count neither discards nor marks
  stale another table's successful one.

### REQ-OFFICE-RUN-HISTORY-RETENTION-004: Operator configuration and sweep visibility

**Intent:** Retention is configurable, its values are validated, and the result
of the most recent sweep is readable by an operator without reading logs.

#### Acceptance criteria

- **AC-OFFICE-RUN-HISTORY-RETENTION-004.1:** An operator can read and change
  whether retention is enabled, the retention window per in-scope table, the
  sweep interval, the retention floor per in-scope table, the batch limit, and
  the warning threshold per in-scope table.
- **AC-OFFICE-RUN-HISTORY-RETENTION-004.2:** Retention shall be enabled by
  default. Default values shall be: retention window 30 days for both
  `office_routine_runs` and `runs`; sweep interval 6 hours; retention floor 50
  rows per owner for both tables; batch limit 5,000 rows per table per sweep;
  warning threshold 25,000 rows for `office_routine_runs`, 25,000 for `runs`,
  and 250,000 for `run_events`.
- **AC-OFFICE-RUN-HISTORY-RETENTION-004.3:** The system shall reject a setting
  outside its permitted range with an error naming the field, and shall leave
  the stored settings unchanged. Permitted ranges are: retention window 1 to
  3,650 days; sweep interval 1 to 168 hours; retention floor 0 to 10,000; batch
  limit 100 to 100,000; warning threshold 0 or greater, where 0 disables that
  table's threshold warning.
- **AC-OFFICE-RUN-HISTORY-RETENTION-004.4:** When stored retention settings
  cannot be read or parsed, the system shall use the documented defaults, emit
  an operator-visible warning, and continue. Unreadable settings shall not
  disable retention silently and shall not fail startup.
- **AC-OFFICE-RUN-HISTORY-RETENTION-004.5:** A settings change shall take effect
  without a backend restart, and shall apply from the next scheduled sweep
  rather than interrupting a sweep in progress. Two concurrent writes resolve
  last-writer-wins; repeating an identical write changes nothing and returns the
  same normalized document. A sweep shall read the stored settings at its start
  rather than relying on a value cached when this process last observed a change,
  so that where several backend processes share one database a process that did
  not serve the write still sweeps under the new settings rather than the
  replaced ones. When that read fails or the stored settings cannot be parsed,
  the sweep shall be skipped and recorded as skipped rather than run against the
  documented defaults, because a default window is shorter than a window an
  operator has widened and sweeping under it would delete the history they
  configured the system to keep. This does not change
  AC-OFFICE-RUN-HISTORY-RETENTION-004.4, which governs reading settings for
  reporting and startup, where using the defaults deletes nothing.
- **AC-OFFICE-RUN-HISTORY-RETENTION-004.6:** An operator can read, on the System
  pages, the outcome of the most recent completed sweep: when it ran, which
  swept tables it previewed, the rows deleted per reported table, the rows it
  would have deleted per previewed table, the retained count per thresholded
  table, whether any table has remaining backlog, and any table that failed.
- **AC-OFFICE-RUN-HISTORY-RETENTION-004.7:** When no sweep has run since the
  backend started, the surface in AC-OFFICE-RUN-HISTORY-RETENTION-004.6 shall
  say so explicitly rather than render an empty or zeroed result that reads as a
  sweep that deleted nothing.
- **AC-OFFICE-RUN-HISTORY-RETENTION-004.8:** The sweep-result surface shall be
  readable while retention is disabled, reporting retained counts and the
  disabled state.
- **AC-OFFICE-RUN-HISTORY-RETENTION-004.9:** A settings write shall replace the whole
  settings document. A field the caller omits shall take its documented default
  rather than its previously stored value, so the same request always produces
  the same stored document. An unrecognized field, a numeric field whose
  value is not a whole number, a field present with a JSON `null`, and a field
  whose value is not of its documented type, shall each be rejected with an error
  naming the field, leaving the stored settings unchanged. `null` shall be
  rejected rather than treated as omission: omission means the documented
  default, so reading `null` the same way would let a client silently replace a
  configured retention window with a shorter default and destroy history the
  operator meant to keep.

## Out of scope

- **A persisted history of retention sweeps.** The last sweep's result is held
  in memory and reported alongside the durable warnings. A growing table
  recording the work of the job that stops tables growing is the same defect in
  a new place.
- **Per-workspace or per-routine retention overrides.** Settings are
  instance-wide. The retention floor is already per owner, which covers what a
  per-routine override would most often be used for.
- **Archival or export before deletion.** Deleted history is gone. An install
  needing it kept sets a longer window or takes a backup.
- **Anything the deletion policy owns.** Eligibility, ordering, batching,
  atomicity, and engine parity are specified in
  [run history retention](run-history-retention.md) and are not restated here.

## Prior art

The receipts for both prior-art legs are recorded once, in
[run history retention](run-history-retention.md#prior-art). The finding that
bears on this document specifically: neither surveyed product previews before a
policy's first deletion, nor warns ahead of the window. Those two behaviors are
where this capability goes further, and they are the reason this document
exists rather than being a settings page bolted onto a sweep.
