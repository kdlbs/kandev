---
status: active
system: platform
created: 2026-09-14
owners:
  - kandev
---

# Task status summary projection contention Requirements

## Overview

The task status summary is a rebuildable read model derived by a per-task
projector. Duplicate or equivalent pull request signals make several writers
persist near-simultaneously, and external unsynchronized writers boot
reconciliation and HTTP rebuild passes race the live projector on the same
compare-and-set row. Repeatedly losing writers need bounded, paced retries and
equivalent work needs coalescing so sustained load converges without handler
errors or lost summaries.

## Requirements

### REQ-PLATFORM-TASK-SUMMARY-CONTENTION-001: Coalesce and rebase task summary projection under contention

**Intent:** Equivalent projection work for one task shall converge to one
effective persistence and publication, and a writer that genuinely loses a
compare-and-set race shall rebase and retry with bounded pacing instead of
failing the handler.

#### Acceptance criteria

- **AC-PLATFORM-TASK-SUMMARY-CONTENTION-001.1:** Concurrent equivalent pending
  refreshes for one task shall be single-flight coalesced and result in at most
  one effective summary persistence and publication.
- **AC-PLATFORM-TASK-SUMMARY-CONTENTION-001.2:** A writer whose
  compare-and-set attempt is rejected shall reload the authoritative summary
  state and rebase its derivation on it before retrying, rather than deriving
  from stale state.
- **AC-PLATFORM-TASK-SUMMARY-CONTENTION-001.3:** Retry pacing between rejected
  attempts shall use a bounded exponential backoff with jitter and respect
  context cancellation; the attempt bound shall remain unchanged and pacing
  shall be injectable so tests can run instantaneously.
- **AC-PLATFORM-TASK-SUMMARY-CONTENTION-001.4:** Under sustained per-task
  contention where an external writer forces repeated compare-and-set losses,
  concurrent events for many tasks shall preserve each task's correct final
  summary and report zero exhausted-retry handler errors.
- **AC-PLATFORM-TASK-SUMMARY-CONTENTION-001.5:** Per-task in-process
  serialization and the semantic no-op short-circuit for equivalent derived
  summaries shall keep a semantically-equal refresh from touching the store or
  publishing a new revision.

## Out of scope

- Watch identity and event idempotency semantics, owned by
  [canonical pull request watch identity](../integrations/requirements/pr-watch-identity.md).
- Storage retention and maintenance, owned by the
  [system page](../system-page/README.md).
