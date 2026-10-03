---
created: 2026-09-29
status: in_progress
requirements:
  - REQ-OFFICE-AUTOMATION-RETRIES-001
system_design:
  - ../../specs/office/system-design/automation-retries.md
legacy_specs: []
---

# Implementation Plan: Durable Automation Retries

## Overview

Deliver a durable retry lifecycle for workspace automations, from persisted policy through attempt scheduling and user-visible history. The implementation is one vertical slice across automation storage, recovery, WebSocket/API projections, the automation editor, and end-to-end coverage.

## Scope

### In scope

- Persist and validate disabled, finite, and infinite retry policies, backoff, and history presentation.
- Preserve immutable policy, trigger, and launch snapshots for every attempt in one retry group.
- Coordinate scheduler claims, task operations, and outbox publication using durable, expiring leases and generation checks.
- Keep stop/disable behavior safe when a live run cannot be stopped.
- Expose complete, bounded retry history and controls without dropping standalone runs or deleting unrelated attempts.
- Preserve legacy webhook interpolation data unless the operator explicitly configures safe JSON pointers.
- Keep public automation documentation and localization aligned with the contract.

### Out of scope

- Retrying workflow-step automations or provider integrations that do not use workspace automation runs.
- Adding retries to unrelated task or plugin operations.
- Automatically changing retry policy based on provider error classification.

## Technical approach

Use the Office automation store as the durable source of ownership and due timestamps. Each original firing owns one retry group; each attempt stores its own run row and exact scheduled time. The lifecycle worker claims due rows transactionally, creates or resumes through a persisted task intent and operation lease, and publishes retry events only while holding the matching outbox token. Recovery preserves active leases and resumes expired operations by idempotent identity. History APIs keep paging bounded; the UI composes groups before status filters and follows due timestamps rather than polling the entire history every second.

## ASCII UI preview

### UI-01: Retry policy and history presentation

Desktop entry point: automation editor retry section and the automation run-history panel. Mobile uses the same controls in the editor's existing stacked section order; the history view keeps attempt rows vertically scrollable and exposes group actions in the row action area.

```text
Retry policy
  [Finite retries v]  Maximum retries [ 3 ]
  Delay [ 60 ] seconds  Backoff [Exponential v]
  History [Timeline v]

Run history
  ● Retry group: Webhook delivery  attempt 2/4  Scheduled
    Next attempt: 12:05 UTC
    [Stop retries] [Delete history]
  ○ Manual run  Succeeded
```

Required structure: retry mode, finite count when applicable, delay/backoff, and history mode remain in one editor section. Timeline grouping and attempt detail are selectable, while a standalone run stays visible beside retry groups. Text and states are localized; the sketch is not a pixel specification.

## Work orders

- [ ] [Task 01: Deliver the durable retry lifecycle](task-01-durable-retry-lifecycle.md)

## Verification strategy

- Run automation and orchestrator Go tests, including the PostgreSQL retry concurrency contract when a PostgreSQL DSN is available.
- Run frontend retry-history and polling unit tests plus focused lint/build checks.
- Run webhook-alert E2E coverage and verify both required delivery identity and retry payload compatibility.
- Run specification validation, specification lint, and public documentation validation.
