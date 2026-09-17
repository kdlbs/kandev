---
status: draft
system: typed-workflow-state
created: 2026-09-01
owners:
  - kandev
---

# Concurrency and idempotency requirements

This document defines the concurrency and idempotency contract for the typed
workflow review state system. Its acceptance criteria state **accepted**
behaviour: they are satisfied by adding no lock, no retry and no reconciliation,
and are verifiable by inspection rather than by a race test. System-wide
terminology, non-functional constraints and exclusions are in
[../README.md](../README.md).

`AC-TWS-005.3` through `AC-TWS-005.5` covered the withdrawn review-finding read and
resolve tools and were removed with them (README Out of scope 7). The numbering gap
is deliberate and the identifiers are not reused.

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
