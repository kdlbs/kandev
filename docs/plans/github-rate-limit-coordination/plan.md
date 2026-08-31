---
spec: docs/specs/integrations/requirements/github-rate-limit-coordination.md
created: 2026-08-29
status: completed
---

# Implementation Plan: GitHub Rate-Limit Coordination

## Overview

Implement the approved provider classification, principal-wide admission,
durable Workflow Sync recovery, and operation-local rate failure contract.
The work is backend/protocol/documentation only and uses no long-running test
runtime.

## Dependency order

- [x] [Task 01: Typed provider failure classification](task-01-rate-classification.md)
- [x] [Task 02: Principal-wide request admission](task-02-rate-coordinator.md)
- [x] [Task 03: Workflow Sync retry persistence](task-03-workflow-sync-backoff.md)
- [x] [Task 04: Operation-local rate errors](task-04-operation-rate-errors.md)
- [x] [Task 05: Documentation and verification](task-05-docs-verification.md)

## Risks

- A secondary response can carry normal primary headers. Classification must
  never overwrite the healthy primary bucket with synthetic exhaustion.
- Waiting admission must be cancellable and must not hold an execution slot.
- Workflow Sync migration must be idempotent for fresh and upgraded databases.
- Rate-limit details must stay attached to the failed operation and exclude
  provider bodies, credentials, and internal snapshots.

## Verification strategy

Each work order owns focused Go/race coverage. The final work order runs the
backend test/lint and public documentation gates from the approved task plan.
