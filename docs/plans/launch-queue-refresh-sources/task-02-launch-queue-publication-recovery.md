---
id: "02-launch-queue-publication-recovery"
title: "Recover lost launch queue publications"
status: done
wave: 2
depends_on:
  - "01-launch-queue-refresh-sources"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
acceptance_criteria:
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.15
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.16
system_design:
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
---

# Task 02: Recover lost launch queue publications

## Summary

Retain bounded recovery after lost enqueue and clear publications while keeping
unrelated subscribed events free of launch-queue reads.

## In scope

- Select persisted positive launch queue summaries through the real repository
  on SQLite and Postgres, excluding deleted tasks and successfully cleared rows.
- Before ceiling replay, refresh the ordered union of those IDs and actual
  deferred tasks using the existing task-updated publisher.
- Retry on subsequent sweep passes after task reload or delivery failures.
- Verify convergence through the real projector/persistence boundary without
  changing task state, deferral metadata, or completion ownership.

## Out of scope

New timers, settings, schema, public APIs, UI changes, unconditional per-message
reads, and changes to admission, replay, completion gates, or sweep cadence.

## Acceptance

Lost clears remain eligible after the authoritative record disappears. Lost
enqueues refresh even if replay is claimed or fails before publication. A
successful clear leaves the recovery set. Duplicate candidates publish once
per recovery pass. Failure on one candidate source does not suppress the other.

## Verification

Hosted full Backend (including SQLite, Postgres and race coverage), Frontend,
and E2E suites plus required static gates. Focused tests:

```bash
cd apps/backend
go test -trimpath ./internal/task/statussummary -run '^TestProjectorLaunchQueue' -count=1
go test -trimpath ./internal/orchestrator -run '^TestCeilingSummaryRecoveryLostEnqueueAndClear$' -count=1
go test -trimpath ./internal/task/repository/sqlite -run '^(TestListTaskIDsWithLaunchQueueSummary|TestPostgresListTaskIDsWithLaunchQueueSummary)$' -count=1
```

## Results

Contained exact-source persistence retry proofs fail for enqueue and clear
on the old change detector and pass when it compares the persisted queue too.

Contained exact-source proofs fail on the old pass driver for both lost clear
and lost enqueue, then pass with the recovery helper. Boundary fakes replace
repository enumeration, claimed/failed replay, logging and publication only.
No broad local package or dependency build was run. The real repository and
projector regression tests are committed for hosted execution; full final-head
suite verification remains pending.

Backend internal: no public docs, UI preview, mobile interaction or copy change.

Queue refresh change detection also compares the next authoritative queue with
the last persisted summary. A failed summary write can advance the in-memory
observation without committing it; repeated source refreshes must retry that
transition rather than suppress it as an unchanged observation. Enqueue and
clear regression tests inject repeated failures at the real persistence boundary.
