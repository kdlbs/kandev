---
status: draft
system: typed-workflow-state
created: 2026-09-01
owners:
  - kandev
---

# Concurrency and idempotency requirements

This document defines the concurrency and idempotency contract for both parts of
the typed workflow review state system. Its acceptance criteria state **accepted**
behaviour: they are satisfied by adding no lock, no retry and no reconciliation,
and are verifiable by inspection rather than by a race test. System-wide
terminology, non-functional constraints and exclusions are in
[../README.md](../README.md).

## Requirements

### REQ-TWS-005: Concurrency and idempotency of the new paths

- **AC-TWS-005.1:** Building the prompt more than once **for the same step entry**
  — the replacement-session path in `launchAfterOnEnterDispatch` does exactly this
  — shall yield the **same** entry number, because re-prompting does not change
  `tasks.workflow_step_id` and `recordStepTransition` writes a row only when
  `from != to`. A later genuine **return** to the step, after the task has left it,
  IS a new entry: it writes a row and shall raise the number by one, as
  AC-TWS-001.2 requires.
- **AC-TWS-005.2:** The count query shall be a read outside any write
  transaction. The ledger row for the current entry is committed before prompt
  building at every call site, so no lock, no retry, and no read-your-write
  coordination is required. The value is a snapshot taken at read time and shall not
  be refreshed within a substitution: a transition committed by another writer
  between the current entry's row and this read would raise the number by one, which
  is accepted rather than defended against. Where one prompt build renders two
  token-bearing templates, NFR-1 permits two independent reads, which may therefore
  disagree if a transition commits between them. Neither is authoritative over the
  other and no reconciliation shall be added: a transition mid-build means the task
  has left the step and the prompt being assembled is already stale.
- **AC-TWS-005.3:** Two callers resolving the same finding to **different**
  statuses shall both succeed. The stored status is that of the write that
  committed last, and each caller's response reflects the row as read back after
  its own write, which may therefore differ from the status it submitted. No
  locking or conflict error is introduced.
- **AC-TWS-005.4:** A finding published concurrently with a list call shall
  either appear in full or not appear. A partially populated finding shall never
  be returned; publication is already all-or-nothing per batch. `total_matched`
  and the returned page need not come from one snapshot: a publish committing
  between them may leave the total inconsistent with the page. The total is
  advisory and this is accepted, not defended against.
- **AC-TWS-005.5:** AC-TWS-004.6's comparison is a plain read followed by a
  conditional write, with no lock and no enclosing transaction. If another writer
  changes the status between that read and the decision, the caller may skip its
  own write and return a row whose status differs from what it submitted. This is
  accepted, on the same grounds as AC-TWS-005.2 and .3: a lock or conflict error for
  a two-agent race on one advisory finding costs more than the race does. No retry
  shall be added.

