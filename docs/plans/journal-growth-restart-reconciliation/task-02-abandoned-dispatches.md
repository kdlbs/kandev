---
id: "02-abandoned-dispatches"
title: "Classify abandoned owner journal dispatches"
status: done
wave: 2
depends_on:
  - 01-mapping-safe-capacity
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 02: Classify abandoned owner journal dispatches

## Summary

Add owner-scoped journal classification at fresh process-manager initialization, before a harness starts or submissions can be admitted. Normalize abandoned retained dispatching records to interrupted_unknown without retirement, outcome invention, payload mutation, or redispatch.

## Regression

Add `TestNewManagerReopenClassifiesAbandonedDispatch` before the correction. The fixture must fail for the observed state mismatch or stale projection, then pass through the production recovery path.

## In scope

- Validate the complete affected owner record set before mutation; preserve terminal, retired, prepared, and accepted records according to existing contracts
- Propagate failed classification as durable initialization failure; keep diagnostic/read-only Open observational
- Reopen tests with matching and mixed conflicting session/incarnation/generation/stream evidence

## Out of scope

- Backend SQL edits, native restore, automatic prompt resend, or relaxed native-resume equality checks

## Acceptance

- Reopening a retained owner journal through NewManager exposes abandoned dispatch as uncertain with unchanged identities/hash/payload; repeating initialization is idempotent.
- A live retained manager is untouched; foreign/future/corrupt evidence fails closed before retirement or harness dispatch. Duplicate admission never calls the harness.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agentctl/journal ./internal/agentctl/server/process -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/journal/journal_submissions.go`
- `apps/backend/internal/agentctl/journal/abandoned_dispatch_test.go (new)`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/abandoned_dispatch_test.go (new)`
- `apps/backend/internal/agentctl/server/process/submission_delivery_test.go`

## Dependencies

01-mapping-safe-capacity.

## Risks

Exclusive journal ownership classifies an abandoned dispatcher; it does not establish termination of an external native harness or authorize restoration.

## Parallelism

`sequential`. No delegation is authorized.

## Inputs

- Linked platform requirements and system designs.
- Plan evidence and existing test patterns.
- Existing journal shutdown, native retirement, and silent restore packages.

## Results

Real owner-journal reopen tests first failed with retained dispatching state, then passed with unknown classification, unchanged payload/hash, and zero redispatch. Conflicting owners reject the whole classification. A restore-only generation preserves earlier unknown history for authenticated retirement. Full process-manager race tests passed; the newer-generation dispatching case additionally validates the retained stream owner before classification. Focused race tests pass.
