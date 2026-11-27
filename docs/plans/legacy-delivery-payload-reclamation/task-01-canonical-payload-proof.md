---
id: "01-canonical-payload-proof"
title: "Separate canonical raw-body proof"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-PLATFORM-DELIVERY-RETENTION-002
  - REQ-PLATFORM-DELIVERY-RETENTION-003
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-001.1
  - AC-PLATFORM-DELIVERY-RETENTION-001.2
  - AC-PLATFORM-DELIVERY-RETENTION-001.4
  - AC-PLATFORM-DELIVERY-RETENTION-002.1
  - AC-PLATFORM-DELIVERY-RETENTION-002.3
  - AC-PLATFORM-DELIVERY-RETENTION-003.1
  - AC-PLATFORM-DELIVERY-RETENTION-003.2
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention-payloads.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 01: Separate canonical raw-body proof

## Summary

Allow independently committed nonterminal raw bodies to compact despite a protected
earlier terminal or missing legacy submission bookkeeping. Retain the exact generic
effect, compact hash receipt and all unresolved authority and settlement evidence.

## In scope and owned files

- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_candidates.go`:
  separate `can_prune` proof from the stricter `can_retire` proof.
- `agent_delivery_retention_proof.go`, `agent_delivery_retention_events.go` and
  `agent_delivery_retention_preview.go` in that package: exact-effect validation,
  existing typed decoding/hash path and consistent bounded preview.
- New `agent_delivery_retention_historical_test.go`; existing active, replay, direct,
  partial-terminal, bounds and shared-barrier tests only where the corrected contract
  changes an assertion.

## Out of scope

No missing submission creation, inferred full settlement, checkpoint leap, workflow
reevaluation, prompt dispatch, canonical message modification or live database edit.
Keep strict terminal/prompt retention and format-1 compatibility.

## Acceptance

1. A mixed SQLite/PostgreSQL fixture with an earlier pending terminal, completed later
   output, active/uncertain prompts and legacy missing submission metadata compacts
   only rows with the independent proof. The exact terminal/prompt/claim/block data,
   canonical contents and protected receipts remain unchanged.
2. Invalid cursor identity, user role, unknown type/effect, pending effect, incomplete
   generic receipt, mismatched effect key, bad hash/version/byte count and unprojected
   output retain the existing fail-closed behavior. Identical compact replay is a
   duplicate; changed payload/envelope fails; ACK reconciliation does not resend.
3. Read-only preview and execution use the same eligibility. Crash/owner replacement
   tests preserve atomic hash/body/tombstone updates and the old retired cursor.

## TDD and exact verification

First add `TestDeliveryRetentionIntegrationHistoricalCanonicalPayloads` and run it
against the current implementation. Require failure because the eligible later raw
body remains, not fixture or database setup failure. Include mixed-state negative
rows so the test distinguishes per-row proof from a stream-wide shortcut.

From `apps/backend`:

```sh
go test ./internal/task/repository/sqlite -run '^TestDeliveryRetentionIntegrationHistoricalCanonicalPayloads$' -count=1
go test ./internal/task/repository/sqlite -run '^TestDeliveryRetention(IntegrationHistoricalCanonicalPayloads|IntegrationActiveCanonicalGrace|Events|PartialTerminal|CompleteTerminalOutcomes|Hash|HashEnvelope|StorageDirectValidation|StorageDirectPrefix|StorageDirectMixedPrefix|Concurrency)' -count=1
go test -race ./internal/task/repository/sqlite -run '^TestDeliveryRetentionConcurrency' -count=1
```

Run the first two commands with a disposable `KANDEV_TEST_POSTGRES_DSN` as well as SQLite.
If unavailable, record the skipped subtests and leave PostgreSQL evidence outstanding.
Record exact failures, final results and any changed test names here.

## Risks and results

The existing first-terminal test is a retirement guard; removing it wholesale is
incorrect. Missing prompt metadata cannot become admission authority. Reuse existing
hash normalization rather than a maintenance-only hash implementation.

Results: complete. The historical canonical-body regression reproduced zero eligible
output before the fix and now passes on SQLite and disposable PostgreSQL. Exact-key,
pending-effect, unchanged-history, protected-terminal and conflicting-replay assertions
passed. The work-order focused tests and race concurrency checks passed. The legacy
terminal-pinning test now requires independent later bodies to compact; the cross-stream
reload fixture now assigns exact generic keys after changing streams. Backend lint and
SQL guard passed. No terminal or prompt authority was synthesized.
