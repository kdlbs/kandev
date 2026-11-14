---
created: 2026-10-26
status: implemented
requirements:
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-007
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
legacy_specs: []
---

# Implementation Plan: Bounded terminal turn-change processing

## Overview

Terminal capture and summary processing must stop retrying an impossible
historical interval without taking ownership of another live interval.
The implementation is published in [PR #4785](https://github.com/kdlbs/kandev/pull/4785),
at implementation commit `256022c3799e6866c21d084e8efa0a17d45ec37f`.
`implemented` records that source result; it does not assert merge or deployment.

This delivery record follows the existing
[turn-change design](../../specs/tasks/system-design/turn-changed-files.md).
The [original feature package](../turn-changed-files/plan.md) owns broader capture
qualification. The [failed-start repair](../turn-change-start-failure/plan.md)
already permits guarded unavailable finalization without an accepted start.
This repair preserves that behavior rather than repeating its repository change.

## Evidence and root cause

Both lifecycle retry loops already used exponential backoff from 250 ms to
5 seconds, but neither had an attempt limit. The coordinator accepts a missing
terminal change set as finished. The runtime adapter then asked the actual
summary publisher to load the same absent row, returning
`ErrTurnChangeSetNotFound` and restarting the unbounded summary loop.

A separate terminal fallback adopted the stored change-set owner before
finalization. A delayed terminal could therefore finalize a newer live interval.
An operator's aggregate warning rate and associated session stalls were reported
observations; this repair did not reproduce those production measurements.

## Scope

One sequential work order covers the lifecycle retry budget, runtime adapter,
coordinator terminal ownership and the existing ownership-test expectation.
Preserve accepted endpoints, retained content, valid summaries and normal
versioned completion/failure handling. Capture state, its generation fence and
durable session-turn completion remain separate authorities.

No UI, configuration, schema, provider policy, completion gate, retention policy
or operating-data change is included. No new permanent test framework is added.

## Technical approach

- Capture and summary each allow three logical attempts total, including any
  inline attempt. Background continuations retain that budget and use
  shutdown-cancellable 250 ms and 500 ms waits.
- Typed missing-set and ownership-loss errors stop retry work immediately.
  A joined unrelated storage error retains bounded retries. A missing historical
  summary has nothing to publish; unrelated publication errors remain errors.
- On exhaustion, abandon only the exact still-owned pending interval, guarded
  by revision and terminal claim. Preserve accepted refs/content and completed
  summaries. Missing or superseded intervals are not recreated or cleaned.
- Release the original capture fence only after accepted capture or successful
  abandonment. Background release leases the captured startup attempt and
  respects the prompt-generation guard. Failed persistence remains fail-closed.
- Existing versioned completion/failure owners settle session turns and managed
  inputs. The correction does not synthesize success or bypass route/queue guards.

## Work orders

- [x] [Task 01: Bound terminal processing](task-01-bound-terminal-processing.md)

## Verification results

The work order records the original observed RED/GREEN evidence and exact
existing focused commands. Real SQLite/publisher, originating-turn settlement,
route/queue delivery, startup/generation protection and shutdown checks passed.
The implementation's normal commit hooks and independent review passed.
These unchanged runtime checks are reused for this documentation-only follow-up.

## Documentation CI follow-up

The first published head contained the six implementation files without a
changed work order. Run `40235457684`, job `121295939635`, failed with
`No changed work order covers the triggering paths.` This package supplies the
missing implementation scope and its real requirement/design/evidence links.
It does not change the coverage checker, its criteria or exemptions.

Local documentation catalog validation, specification lint and whitespace checks
passed. The existing coverage module evaluated the actual six implementation
paths plus these two documents and returned `covered`, `ok: true`, no errors,
and the declared requirement/design references. No checker or exemption changed.

The new public head's actual coverage result remains pending until these
documents are applied to the same PR. This record does not claim that an old
failed run has passed.

## Risks

Never release a successor across an unproven durable boundary to suppress an
error. Never reinterpret a stale terminal as another live owner. Documentation
reconciliation must preserve these constraints and the failed-start repair.
