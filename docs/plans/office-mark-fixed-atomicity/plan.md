---
created: 2026-09-17
status: done
requirements:
  - REQ-OFFICE-RUNTIME-001
system_design:
  - ../../specs/office/system-design/runtime-01.md
  - ../../specs/office/system-design/runtime-02.md
legacy_specs: []
---

# Implementation Plan: Office Mark fixed atomicity

## Overview

**Mark fixed** on an `agent_paused_after_failures` inbox entry clears the
agent's `pause_reason`, resets `consecutive_failures`, and re-queues the
affected tasks. When a second, unrelated auto-pause lands on the same agent
between the moment Mark fixed reads the agent and the moment it writes the
clear, the write used to succeed unconditionally: it clobbered the newer
pause's `pause_reason`, then went on to reset the counter and recover tasks
from the *first* pause's stale snapshot, silently discarding the second
pause's cause.

This plan closes that window with a compare-and-set write plus a
retry-abort guard, and dismisses the inbox entry only once the clear it
guards has actually succeeded. It does not touch the rest of the recovery
flow (dismissal semantics for `agent_run_failed`, reassignment, or the
counter-reset UX) and does not redesign it.

## Scope

### In scope

- A compare-and-set write for the unpause transition (`paused` -> `idle`)
  gated on both `status` and the `pause_reason` the caller observed, closing
  the read-then-write window.
- A retry-abort guard in the service-layer clear loop: when the CAS write is
  refused and a reload shows the agent still paused with a *different*
  reason than the caller started with, the call aborts instead of retrying
  against the newer reason.
- Reordering `MarkAgentPausedFixed` so the inbox entry is dismissed only
  after its clear succeeds, leaving an aborted call's entry retryable.
- Regression coverage for the compare-and-set write and the abort guard.

### Out of scope

- Any change to `agent_run_failed`'s dismiss-then-retry flow, reassignment,
  or the counter-reset semantics AC-OFFICE-RUNTIME-001.5 already covers.
- An event or activity entry for a refused Mark fixed call.
- The unrelated `office_agent_recovery` status-transition control (a
  different action on a different endpoint; see
  [`office-agent-recovery`](../office-agent-recovery/plan.md)).

## Technical approach

### Compare-and-set unpause write

`UnpauseAgentIfCurrent` (`internal/office/repository/sqlite/agents.go`)
updates `agent_instances` with `WHERE status = 'paused' AND pause_reason = ?`
instead of `WHERE status = 'paused'` alone, so a write built from a stale
`pause_reason` affects zero rows instead of clobbering a newer one.

### Retry-abort on a newer, unobserved reason

`clearAutoPause` (`internal/office/service/failure.go`) captures
`originalReason` before its CAS attempt. When the attempt reports no row
changed, it reloads the agent: if the reload is no longer auto-paused, the
clear is treated as already done; if it is still paused with the *same*
reason, the loop retries (handles a same-reason status flip between
`paused` and `stopped`); if it is still paused with a *different* reason,
the call aborts with an error rather than adopting it. The newer pause stays
untouched and reachable by its own Mark fixed.

### Dismiss-after-clear ordering

`MarkAgentPausedFixed` previously inserted the `inbox_dismissals` row before
clearing the pause. It now clears first and dismisses only on success, so an
aborted clear leaves the entry visible and the call retryable instead of
being silently swallowed by an already-recorded dismissal.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-OFFICE-RUNTIME-001.9` | `TestUnpauseAgentIfCurrent_RefusesWhenPauseReasonChanged` (repository CAS refusal) and `TestClearAutoPause_AbortsWhenNewerAutoPauseLands` (service-layer abort, `consecutive_failures` untouched) |
| `AC-OFFICE-RUNTIME-001.10` | `TestClearAutoPause_AbortsWhenNewerAutoPauseLands` asserts the newer `pause_reason` survives the aborted call; `TestClearAutoPause_SucceedsWhenPauseReasonStillMatches` is the matching-reason control proving the abort is reason-specific, not a general regression |

## Verification results

Delivered on PR [#3734](https://github.com/kdlbs/kandev/pull/3734), commits
`9d1f1975e` (CAS unpause write) and `4bcdbef71` (retry-abort guard and
dismiss-after-clear ordering). `go test -tags fts5 ./internal/office/service/...
./internal/office/repository/sqlite/...` passes except for the pre-existing,
unrelated `TestMigrate_PriorityIdempotent` FTS backfill failure (reproduces
identically on the branch base commit, before this plan's changes).

## Risks

- `clearAutoPause`'s loop caps at two attempts. A third auto-pause landing
  between the second attempt's read and write is not retried further; the
  call fails closed (an error), which is the same outcome as the guarded
  abort case, not a silent clobber.
- The compare-and-set predicate is `status = 'paused' AND pause_reason = ?`.
  A caller that races the *stopped* clear path
  (`ClearAgentPauseReasonIfCurrent`, gated on `status` alone) is unaffected
  by this plan - that path never held the unpause clobber this plan closes,
  because a `stopped` agent's pause_reason is not the multi-writer contended
  field the auto-pause scheduler mutates.

## Work orders

- [x] [Task 01: CAS unpause write and retry-abort guard](task-01-cas-unpause-retry-abort.md)
