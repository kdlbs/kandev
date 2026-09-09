---
status: draft
system: office
created: 2026-09-09
owners:
  - kandev
---

# Office Run History Retention Requirements

## Overview

Office writes two families of history rows that nothing removes on a schedule.
`office_routine_runs` records one row per routine firing. `run_events` records
the timeline of every Office run, append-only, alongside the `runs` queue row
and its per-run satellites. The only deletions today are manual and structural:
deleting a routine removes its runs, and deleting a workspace removes everything
belonging to it. An install that never deletes a routine or a workspace grows
forever.

The growth is driven by a clock, not by usage. A routine on a `*/5 * * * *`
schedule fires 105,120 times a year, and each firing writes a routine-run row
whether or not it does anything. On the reference install, one routine had
accumulated 323 consecutive `coalesced` rows over 28 days while producing no
work at all: that is the curve with the loop broken, and a working loop also
writes the run row, its `run_events` timeline, and its route-attempt rows.

This document bounds those tables. It defines what is history and may be
deleted, what is live state and must never be deleted on age, when the deletion
runs, and that it behaves identically on both database engines. What an operator
is told before the first row is removed, what they are warned about while the
tables grow, and what they can configure and read is the paired contract in
[run history retention operations](run-history-retention-operations.md).

Office owns this contract because the rows are defined by Office primitives: the
routine dispatch ledger, the run queue, and the run event timeline. The System
pages own the operator surface it reports through. The filesystem and container
cleanup owned by [storage
maintenance](../../system-page/requirements/storage-maintenance.md) is a
separate capability that never touches database rows.

## Terminology

- **Retention sweep** (or **sweep**): one pass that evaluates every in-scope
  table against the configured policy and deletes the eligible rows.
- **History row**: a row recording something that already happened, which no
  live decision reads. History rows are eligible for deletion.
- **Live-state row**: a row a live decision still reads, regardless of age.
  Never eligible for age-based deletion.
- **Run satellite row**: a row keyed by a `runs` row's identifier and owned by
  it: a `run_events`, `office_run_route_attempts`, or `office_run_skills` entry.
- **Retention window**: the age past which a history row becomes eligible,
  measured from the row's own completion time.
- **Completion time**: `COALESCE(completed_at, created_at)` for a routine-run
  row and `COALESCE(finished_at, requested_at)` for a `runs` row. Both fallback
  columns are `NOT NULL`, so completion time is never null.
- **Retention floor**: most-recent history rows kept per owner regardless of
  age, so a rarely-firing routine or rarely-woken agent keeps visible history.
- **Batch limit**: the maximum rows one sweep deletes from one table.
- **Preview**: a swept table's first evaluation on a database; it counts what it
  would delete from that table and deletes nothing. Tracked per swept table.
- **Retained count**: the number of rows a table currently holds — a property of
  the table, not of a sweep, defined whether or not a sweep has ever run and
  whether or not retention is enabled.
- **Swept tables**: the two tables retention selects rows from by policy,
  `office_routine_runs` and `runs`.
- **Reported tables**: the five tables a sweep can delete rows from and reports
  deleted counts for: the two swept tables plus `run_events`,
  `office_run_route_attempts`, and `office_run_skills`.
- **Thresholded tables**: the three tables carrying a warning threshold,
  `office_routine_runs`, `runs`, and `run_events`.

## Requirements

### REQ-OFFICE-RUN-HISTORY-RETENTION-001: History is bounded and live state is not

**Intent:** Bound `office_routine_runs`, `runs`, and the run satellite tables by
age and by an owner-scoped floor, while guaranteeing that no row another
decision still reads is removed because it is old.

As an operator running Office continuously, I want old run history removed
automatically, so a scheduled routine does not grow the database without limit.

#### Acceptance criteria

- **AC-OFFICE-RUN-HISTORY-RETENTION-001.1:** A routine-run row is a history row
  only when its status is one of `skipped`, `coalesced`, `failed`, `done`, or
  `cancelled`. When a routine-run row's status is `received` or `task_created`,
  the system shall treat it as a live-state row and shall not delete it on age,
  at any age.
- **AC-OFFICE-RUN-HISTORY-RETENTION-001.2:** A `runs` row is a history row when its
  status is `finished`, `failed`, or `cancelled`. `cancelled` is terminal: its
  only writer moves a row there from `queued` or `claimed` and stamps the
  completion timestamp in the same statement. When a `runs` row's status is
  `queued` or `claimed`, the system shall treat it as a live-state row and shall
  not delete it on age, at any age, including a run parked for a future routing
  retry.
- **AC-OFFICE-RUN-HISTORY-RETENTION-001.3:** When a history row's completion time is
  older than that table's retention window, the system shall make it eligible
  for deletion. Completion time is defined in Terminology. A row's
  classification as history shall depend on its status alone and shall not
  additionally require `completed_at` or `finished_at` to be set; a terminal row
  with an unset stamp is history, dated by its fallback column, and ages out
  rather than being retained forever.
- **AC-OFFICE-RUN-HISTORY-RETENTION-001.4:** The system shall retain the newest
  history rows of each owner up to that table's retention floor even when they
  are older than the retention window. The owner is the routine for
  `office_routine_runs` and the agent profile for `runs`. Newest is completion
  time descending, with the row identifier descending as the tiebreak.
  Completion time is never null, so this ordering is total on both engines and
  does not depend on either engine's default placement of nulls. The identifier
  gives a stable order for equal timestamps and is not claimed to be
  chronological.
- **AC-OFFICE-RUN-HISTORY-RETENTION-001.5:** When the system deletes a `runs`
  row, it shall delete that run's satellite rows in the same database
  transaction, and no satellite row shall remain that references a `runs` row
  the system has deleted.
- **AC-OFFICE-RUN-HISTORY-RETENTION-001.6:** The system shall not delete
  `run_events` rows for a run it is not deleting in the same transaction, at any
  age.
- **AC-OFFICE-RUN-HISTORY-RETENTION-001.7:** A routine's status shall not exempt
  its history from retention. When a routine is `paused`, its history rows are
  evaluated by the same policy as an `active` routine's.
- **AC-OFFICE-RUN-HISTORY-RETENTION-001.8:** The system shall not delete, alter,
  archive, or cancel a task, a session, a task checkout, or a workspace as part
  of a retention sweep. A routine-run row naming a task in `linked_task_id` may
  be deleted while that task continues to exist.
- **AC-OFFICE-RUN-HISTORY-RETENTION-001.9:** A retained `coalesced` routine-run
  row may name a `coalesced_into_run_id` whose row has already been deleted.
  `coalesced_into_run_id` records provenance and is not a referential
  constraint; the system shall not delete a coalesced row because its target was
  deleted, and shall not retain a target because a coalesced row names it.

- **AC-OFFICE-RUN-HISTORY-RETENTION-001.10:** When a row in a swept table holds a
  status belonging to neither that table's history set nor its live-state set,
  the system shall treat it as a live-state row, shall not delete it at any age,
  and shall emit an operator-visible warning naming the table and the
  unrecognized status, through the channel required by AC-OFFICE-RUN-HISTORY-RETENTION-003.8 in [run history
  retention operations](run-history-retention-operations.md). The warning shall be
  produced by the count evaluation required by
  AC-OFFICE-RUN-HISTORY-RETENTION-003.11 rather than by the deletion path, which
  cannot observe a status it does not select; it shall therefore be emitted on an
  install where retention is disabled and no sweep runs. When one table holds more
  than one unrecognized status, the system shall emit a single warning for that
  table naming every unrecognized status in ascending lexicographic order with its
  row count.

### REQ-OFFICE-RUN-HISTORY-RETENTION-002: The retention sweep

**Intent:** Run retention on its own schedule, off the run-claiming hot path, in
bounded batches, with one sweep at a time and no dependence on database cascade
behavior.

#### Acceptance criteria

- **AC-OFFICE-RUN-HISTORY-RETENTION-002.1:** The system shall run retention on a
  dedicated schedule whose interval is configurable in hours, and shall not
  perform retention work on the Office run-processing tick.
- **AC-OFFICE-RUN-HISTORY-RETENTION-002.2:** When a scheduled sweep is due while a
  previous sweep is still running, the system shall skip the due sweep rather
  than run two concurrently or queue it, and shall record that it was skipped.
  Recording a skip shall not replace the reported result of the last completed
  sweep.
- **AC-OFFICE-RUN-HISTORY-RETENTION-002.3:** A sweep shall delete at most the batch
  limit of rows from `office_routine_runs` and at most that many from `runs`. A
  preview evaluation shall never be reported as having remaining backlog: it
  deletes nothing by design, so "retention is behind" is not true of it however
  many rows were eligible.
  The limit counts those rows only; every satellite row of a deleted run is
  removed regardless of the limit. When more rows were eligible than the limit
  allowed, the system shall complete the sweep, report that table as having
  remaining backlog, and continue on the next scheduled sweep. Within a table
  the sweep shall select its batch in completion-time ascending order, with the
  row identifier ascending as the tiebreak, so the oldest eligible rows are
  removed first and both engines select the same batch.
- **AC-OFFICE-RUN-HISTORY-RETENTION-002.4:** The system shall re-assert every
  eligibility condition in the deletion itself, not only when selecting
  candidates. A run that returns to `queued` between selection and deletion,
  which a scheduled retry does by clearing `finished_at`, shall not be deleted
  by that sweep. The re-asserted conditions shall include the retention floor as
  well as status and age.
- **AC-OFFICE-RUN-HISTORY-RETENTION-002.5:** Each batch shall be atomic: after a
  sweep is interrupted by shutdown or error, every batch that was applied is
  complete, including its satellite rows, and no batch is partially applied.
- **AC-OFFICE-RUN-HISTORY-RETENTION-002.6:** Once a table holds no eligible row,
  a further sweep against unchanged settings shall delete nothing from that table
  and report zero deletions for it. This criterion is scoped to that drained
  state and does not contradict AC-OFFICE-RUN-HISTORY-RETENTION-002.3: while a
  table still reports remaining backlog, the next sweep is required to delete its
  next batch, so "deletes nothing further" is not claimed of a backlogged table.
- **AC-OFFICE-RUN-HISTORY-RETENTION-002.7:** When a table's sweep fails with a
  database error, the system shall record the failure for that table, continue
  with the remaining in-scope tables, and retry the failed table on the next
  scheduled sweep. A retention failure shall not fail backend startup, stop the
  Office scheduler, or abort the remainder of the sweep. A batch abandoned
  because its deletion could not be applied consistently shall be recorded as a
  failure for that table rather than as backlog. Every table whose rows that
  batch's transaction addressed shall report zero rows deleted for that sweep, so
  no table reports rows that the rollback restored.
- **AC-OFFICE-RUN-HISTORY-RETENTION-002.8:** When retention is disabled, the
  system shall run no sweep and delete no row, and shall still report retained
  counts and threshold warnings.
- **AC-OFFICE-RUN-HISTORY-RETENTION-002.9:** When every in-scope table is empty
  or holds no eligible row, the sweep shall complete reporting zero deletions,
  without error and without a warning.
- **AC-OFFICE-RUN-HISTORY-RETENTION-002.10:** The first sweep after the backend
  starts shall run after a short fixed delay rather than after a full sweep
  interval, so that an install restarted more often than the interval still runs
  retention. Later sweeps use the configured interval.
- **AC-OFFICE-RUN-HISTORY-RETENTION-002.11:** A sweep shall compute one cutoff
  instant at its start and evaluate every table against that instant, so two
  tables in one sweep cannot disagree about what "older than the window" means.

- **AC-OFFICE-RUN-HISTORY-RETENTION-002.12:** On PostgreSQL, where several backend
  processes can share one database, sweep exclusivity shall hold across
  processes and not only within one: a backend that cannot acquire the retention
  lock shall skip its due sweep exactly as it would for a sweep already running
  in its own process. On SQLite one backend process owns the database file, so
  the in-process guard is sufficient.
- **AC-OFFICE-RUN-HISTORY-RETENTION-002.13:** Sweeps shall be scheduled fixed-delay:
  the next sweep is armed when the previous one finishes, so a sweep running
  longer than the interval delays its successor rather than causing an immediate
  second one. A settings change re-arms the delay from the moment of the change.
  Enabling retention that was disabled arms the next sweep at the same short
  delay as AC-OFFICE-RUN-HISTORY-RETENTION-002.10 rather than at a full interval.

### REQ-OFFICE-RUN-HISTORY-RETENTION-005: Integrity and database engine parity

**Intent:** Retention leaves the database consistent and behaves identically on
both supported engines, including where the schema differs between them.

#### Acceptance criteria

- **AC-OFFICE-RUN-HISTORY-RETENTION-005.1:** The system shall produce the same
  observable retention outcome on SQLite and on PostgreSQL for the same settings
  and the same starting rows: the same rows deleted, the same rows retained, the
  same reported counts, and the same warnings. This shall hold on the backlog
  path as well, because batch selection order is fixed by named columns in
  AC-OFFICE-RUN-HISTORY-RETENTION-002.3 rather than left to the engine.
- **AC-OFFICE-RUN-HISTORY-RETENTION-005.2:** The system shall delete satellite
  rows explicitly and shall not depend on a foreign-key cascade to remove them.
  Neither engine declares a foreign key from a satellite table to `runs`.
- **AC-OFFICE-RUN-HISTORY-RETENTION-005.3:** After any sequence of sweeps, the
  concurrency gate that finds a routine's active run by dispatch fingerprint
  shall return exactly what it would have returned had no sweep run.
- **AC-OFFICE-RUN-HISTORY-RETENTION-005.4:** After any sequence of sweeps, the
  lookup that closes out a routine run when its linked task reaches a terminal
  step shall not resolve a deleted row to a different routine's run. A deleted
  row resolves to nothing.
- **AC-OFFICE-RUN-HISTORY-RETENTION-005.5:** Retention shall not change the
  behavior of deleting a routine or deleting a workspace. Those paths continue
  to remove every row they remove today, including rows retention has not yet
  reached.

## Out of scope

Each exclusion below is a decision, not an oversight.

- **Automation run history (`automation_runs` and its tables).** Owned by the
  automation system, and its published contract is that history is removed only
  by an explicit per-run or delete-all action. It has the same unbounded-growth
  gap and needs its own requirement; changing it here would silently break a
  documented promise.
- **`office_activity_log`, inbox dismissals, and approval rows.** The
  [inbox requirement](inbox.md) already states these accumulate indefinitely and
  excludes activity-log retention. Reopening it belongs to that contract.
- **`office_cost_events`.** Deleting cost rows changes reported spend and budget
  enforcement: a decision about financial records, not about storage.
- **Detecting a stuck routine.** The 323-row reference case came from a routine
  that fired correctly and did nothing useful for 28 days. Bounding its history
  does not detect it; a detector for that state is a scheduler or stall
  visibility concern.
- **Reclaiming file bytes after deletion.** Deleting rows does not shrink a
  SQLite file. `VACUUM` is already an operator action on the System pages and a
  sweep does not trigger it.
- **Archival or export before deletion.** Deleted history is gone. An install
  needing it kept sets a longer window or takes a backup.
- **Per-workspace or per-routine retention overrides.** Settings are
  instance-wide here. The retention floor is already per owner, which covers what
  a per-routine override would most often be used for.
- **A persisted history of retention sweeps.** The last sweep's result is held
  in memory and reported alongside the durable warnings. A growing table
  recording the work of the job that stops tables growing is the same defect in
  a new place.
- **Tables outside Office.** Nothing here changes task, session, workflow,
  plugin, or auth storage.

## Prior art

Receipts for both legs. The design document carries what each finding changed.

**Our own wiki: not consulted, tool unavailable.** The `@henry` pin resolved
`~/.obsidian-wiki/config` to `config.henry`, giving
`OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki` and
`QMD_WIKI_COLLECTION=wiki`. Neither retrieval path ran: `qmd` and
`obsidian-wiki` are absent from `PATH`, no QMD MCP tool is registered in this
session, and the vault directory returns `Operation not permitted` both
sandboxed and unsandboxed, a macOS file-access restriction on this process
rather than a missing vault. The grep fallback is blocked the same way, so this
leg is a tooling gap, not evidence the wiki is silent on retention.

**What other products shipped: consulted.** Queried the `saas-kb` server
(`search_fsm_docs`, `category: "ai_sdlc"`) three times: run-history retention and
database growth; session history retention and automatic deletion; and
scheduled-automation run-history limits. Relevance was low across all three,
itself a finding about corpus coverage. Two useful hits: **GitLab Duo** deletes
sessions 30 days after last activity, which anchors the default window here; and
**the Claude apps gateway** documents four tables with per-table windows
enforced by one hourly sweep, marking one table "until deleted via the API"
instead of giving it a window, which is the split this document draws between
history rows and live-state rows.

Neither previewed before a policy's first deletion, nor warned ahead of the
window. Those are where this capability goes further, because an upgrade that
silently deletes a year of history on its first sweep is the failure mode a
shipped default carries.
