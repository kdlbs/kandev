---
id: "03-cancellation"
title: "Preserve canceled recovery"
status: done
wave: 3
depends_on: ["02-graphql"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-003
  - REQ-INTEGRATIONS-GITHUB-RATE-004
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-RATE-003.1
  - AC-INTEGRATIONS-GITHUB-RATE-003.2
  - AC-INTEGRATIONS-GITHUB-RATE-003.4
  - AC-INTEGRATIONS-GITHUB-RATE-003.6
  - AC-INTEGRATIONS-GITHUB-RATE-004.1
system_design:
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 03: Preserve canceled recovery

## Summary

Preserve durable recovery when the actual request context is canceled. Manual attempts bypass scheduling without clearing the saved state before success.

## In scope

- Remove the eager persistent reset from prepareManualSync.
- Keep the prior recovery state until the attempt reaches a durable outcome.
- Use a five-second context.WithoutCancel-derived context only for terminal persistence.
- Keep the later existing or observed retry deadline after cancellation.
- Keep provider work and reconciliation canceled. Preserve workspace identity and authorization.
- Avoid a follow-up read with the dead request context when that masks an available operation error.
- Use a subsequent authorized GET for disconnected callers. Do not promise delivery after disconnect.

## Out of scope

Other work orders, external GitHub writes, and unrelated refactoring.

## Regression evidence

Add TestManualSyncCanceledContextPersistsRecovery and TestHTTPForceSyncCanceledRequestPersistsRecovery. Cancel the actual request after admission begins. Read the real store with a fresh context. Include an existing later deadline, cancellation without provider metadata, restart, and final successful recovery.

## Acceptance

- Actual cancellation stores a safe failure and a durable retry boundary.
- A new service instance does not automatically retry before that boundary.
- Successful explicit recovery clears the state, while configuration changes remain serialized.

## Verification

Run from the repository root after implementation:

```bash
(cd apps/backend && go test ./internal/workflowsync -run 'Test.*(Cancel|Recovery|Retry|Backoff|ForceSync|Circuit)' -count=1)
(cd apps/backend && go test -race ./internal/workflowsync -count=1)
```

## Files likely touched

- apps/backend/internal/workflowsync/service.go
- apps/backend/internal/workflowsync/backoff.go
- apps/backend/internal/workflowsync/handlers.go
- apps/backend/internal/workflowsync/service_test.go
- apps/backend/internal/workflowsync/handlers_test.go
- apps/backend/internal/workflowsync/backoff_test.go
- apps/backend/internal/workflowsync/store_test.go

## Dependencies

02-graphql. Execute sequentially.

## Risks

Do not import Office config-sync as a dependency. Reuse its small bounded-persistence pattern locally. Do not detach provider work.

## Parallelism

`sequential`

## Inputs

RATE-003 and RATE-004. System design: Workflow Sync recovery. Pattern: internal/office/configsync/reconcile_run.go:recordWriteContext.

## Results

Completed 2026-09-27. Manual sync no longer clears durable recovery before
provider work. Failed and successful terminal writes use a five-second context
that preserves request values while detaching only the database write from
request cancellation. Retry boundaries retain the later existing deadline.
The HTTP handler stops its follow-up read after a disconnect, leaving the
durable result available through a later authorized GET. Regression coverage
cancels actual service and HTTP request contexts, reloads persisted state,
checks restart scheduling, and verifies successful recovery.

Validation passed:

- `go test ./internal/workflowsync -run 'Test.*(Cancel|Recovery|Retry|Backoff|ForceSync|Circuit)' -count=1`
- `go test -race ./internal/workflowsync -count=1`
