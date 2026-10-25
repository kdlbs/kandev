---
created: 2026-10-12
status: done
requirements:
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
system_design:
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
legacy_specs: []
---

# Implementation plan: Launch queue refresh sources

## Overview

The task status summary projector re-reads a task's launch queue for every
event it handles. Task 01 limits that read to the events that can
change the queue. Task 02 preserves bounded recovery when mutation publication
is lost, using the existing ceiling sweep.

## Evidence and confirmed scope

Baseline is upstream `main` at `add376a2af17753ec349d9e7b042306c0f91100b`.

- `Projector.handleEvent` calls `loadLaunchQueue` for every subscribed event,
  including `message.added`, `message.updated`, turn, and activity events.
- The production loader reads the task row and samples the install-wide
  session ceiling (`CurrentSessionCeilingObservation`).
- A ceiling deferral is written on the task row and publishes `task.updated`.
  The ceiling sweep's still-deferred path publishes `task.updated` so the
  displayed capacity sample stays current. The queue and its capacity sample
  therefore already arrive through task events.
- State creation restores the queue once, and `handleEvent` then reads it
  again for the same event.

No open upstream pull request changes the production files in scope.

## Contract change

- `AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.15` (new): a closed set of
  launch-queue refresh sources; other events keep the last observation.

## Scope

### In scope

- `task/statussummary` refresh gating and its unit tests.
- Sweep publication recovery and SQLite/Postgres persisted-summary selection.
- The requirement criterion, design rule, this plan, and the work order.

### Out of scope

- The loader, ceiling observation, sweep cadence, deferral record semantics,
  summary schema, and frontend.

## Work orders

- [x] [Task 01: Refresh the launch queue only from its sources](task-01-launch-queue-refresh-sources.md) (wave 1, no dependencies)

- [x] [Task 02: Recover lost launch queue publications](task-02-launch-queue-publication-recovery.md) (wave 2, depends on Task 01)

## Verification strategy

The order adds source-adjacent unit tests that fail on the baseline and pass
after the change, and keeps the existing projector tests green. Commands run
only named tests with `-run`. `go build`, `go vet`, and scoped lint run once
on the changed package. Documentation checks:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## E2E, docs, and mobile assessment

Backend-internal. No API, payload, copy, setting, or UI change, so no ASCII
preview, public docs, or browser/E2E evidence is needed.

## Risks

A future subscribed event that can change the queue must join the closed set,
which sits next to the other refresh classifiers to keep that visible.

## Review remediation

A lost clear publication removes the authoritative record from replay selection
while leaving a positive summary. Recovery must index that persisted summary
independently of replay; a lost enqueue must refresh before replay can return
early. The original Task 01 results below remain historical. Task 02 results
record contained proofs and hosted verification separately.
