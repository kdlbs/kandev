---
created: 2026-10-28
status: implemented
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
legacy_specs: []
---

# Implementation Plan: Settle Office runs across ceiling replay

## Overview

An Office run whose launch the session ceiling refuses is parked as
`blocked_provider_action_required` with no retry deadline. The ceiling replay
sweep is the only launcher that resumes it. Before this plan, nothing settled
the parked Office run when the replay admitted or dropped the deferral, so the
run stayed queued and blocked after its session had already started.

This plan adds one Office-side settlement path driven by the ceiling's own
replay result. It keeps the ceiling as the sole launcher and adds no Office
retry, park status, timed lift, or backfill.

## Problem

- The ceiling start-deferral payload did not carry the Office run identity, so
  the replayed launch lost its Office attribution.
- A successful replay left the parked Office run queued and blocked because
  the Office scheduler deliberately never retries a ceiling-deferred launch.
- A terminal ceiling drop failed the automation run but not the Office run.
- The replay-success settlement write is best-effort. Its `AgentReady`
  recovery and the launch's `AgentStopped`/completion event run in separate
  goroutines; completion can arrive first, find no claimed run, and be
  discarded. A later ready recovery then claims a run whose execution has
  already stopped, with no later event to finish it. The old completion
  fallback could also guess from task and agent when the event had no launch
  identity, and a pre-ready launch failure could be lost before recovery.

## Scope

### In scope

- Persist and restore the Office `causing_run_id` in the ceiling `start`
  deferral payload.
- Settle the exact parked Office run when the ceiling replay admits or drops
  the deferral, using a compare-and-set on the run id, `queued` status,
  `blocked_provider_action_required`, payload task, and Office agent.
- Recover a failed replay-success settlement from the replayed launch's
  `AgentReady` event, gated by the durable ceiling-deferral marker.
- Preserve the exact run identity for terminal lifecycle events after
  `causing_run_id` is consumed by the first ready event. A new prompt clears
  the terminal identity.
- Finish the parked run directly when the launch's completion event arrives
  before the ready recovery. Completion and ready settlement are mutually
  exclusive exact-row transitions; completion retries the claimed-run lookup
  once if ready won the race.
- Route a pre-ready failure through the normal Office failure policy. A stop
  event with an error cannot finish the run successfully.
- Keep the ceiling deferral until the caller accepts a terminal-drop
  settlement, then clear it before writing the one-shot drop note.
- Check the exact run's queued/action-required state before reading its event
  history for the ceiling marker.
- Document the ceiling as the recovery owner and warn against manually
  clearing the routing block.

### Out of scope

- Automatic Office-side retry of a ceiling-deferred run, a session-capacity
  park status, timed lift, or boot backfill. These remain a separate
  policy/contract proposal.
- The taskless/execution-grade capacity work in `#4715` and the explicit
  discard contract in `#4508`.
- Unrelated style suggestions on the same PR.

## Technical approach

Follow the [session-ceiling system design](../../specs/agents/system-design/session-concurrency-ceiling.md)
and the Office routing design's deferred-launch section
([routing-01](../../specs/office/system-design/routing-01.md)). The Office
service observes the ceiling's replay outcome and performs an exact-row run
transition; it never dispatches a new launch. The completion-before-ready
ordering is a single parked-to-finished transition, so no run is left claimed
after its execution has stopped. Terminal events carry a retained launch
identity so a delayed stop cannot finish a successor run. A pre-ready failure
claims the exact parked run before normal Office failure handling. Drop
settlement must succeed before the ceiling record is cleared.

## Work orders

- [Task 01: Settle Office runs across ceiling replay](task-01-settle-office-runs-on-ceiling-replay.md)

## Validation

- Focused tests passed for lifecycle identity, Office ready/stop/failure
  settlement, repository error guards, watcher event decoding, and retained
  ceiling records after failed drop settlement.
- Documentation validation passed: `python3 scripts/list-docs.py validate`
  and `python3 scripts/lint-spec-files.py --all`.
- The broader five-package test run reached its 10-minute timeout in the
  unrelated `TestStartupReconciliationPreservesOnlyExactPendingRemoteOwner`
  test while it waited in SQLite. No assertion failure was reported for the
  focused tests above.
