---
created: 2026-10-11
status: done
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
legacy_specs: []
---

# Implementation Plan: Start a feeder-created automation task only once

## Overview

An automation can create its task in a step that another step pulls from.
`CreateTask` runs that feeder pull before it returns, and the pull's
`task.moved` auto-starts the task when the pulling step has `auto_start_agent`.
The automation then starts the same task itself. With the session ceiling full,
the second start is refused as a conflicting deferral; with free capacity, it
fails because the first start is still preparing the workspace. Either failure
fails the run and deletes the task together with the first start. Issue #4323
reports the gap.

## Scope

- Stamp server-managed initial-start ownership before feeder publication.
- Use the existing run and launch ledgers, immutable identity and metadata CAS.
- Preserve queued and uncertain ownership across instances and recovery.
- Release only definitive admission or termination; publish task updates.
- Protect ordinary create, replacement and merged metadata on SQLite and PostgreSQL.
- Preserve continuation identity and all existing retry/manual recovery fences.

Rendered UI, runtime flags and persistence migrations are outside this change.
The private marker is not a new retry ledger.

## Technical approach

The automation request supplies its admitted run identity through private create
fields. Task construction stamps the marker before feeder events. An unbound
internal start receives a fresh token. Generic workflow auto-start yields to the
marker and can reconcile only definitive ledger evidence. Queued dispatches
retain their marker. Exact queue drops and bound legacy starts release by CAS;
durable retry markers remain until their exact launch is confirmed stopped and
the run is terminal. Missing evidence, failed Stop and restart alone retain the
fence. Continuation refresh cannot replace an earlier initial-start owner.

## Work orders

- [x] [Task 01: Single start owner for automation tasks](task-01-single-start-owner-for-automation-tasks.md)

## Verification results

Contained extracted-source RED proofs exposed unresolved startup release and
stale cleanup erasing a successor. The actual-source GREEN suite passes with
race detection using explicit fake repository, ledger, logger and publisher
boundaries. Repository/caller coverage is committed for hosted Backend CI,
including registered REST/WS creation and SQLite/PostgreSQL metadata contracts.
Full actual Backend, Frontend and E2E results are an external exact-head delivery
gate. Their immutable run/head receipts are retained outside these tracked
implementation records; local extracted proofs do not establish those results.

## Risks

Unresolved legacy ownership remains fenced when termination cannot be proven.
Explicit task/session recovery remains available under existing retry controls.
An unavailable metadata write leaves ownership in place until later ledger-based
reconciliation succeeds; capacity expiry does not authorize replay.
