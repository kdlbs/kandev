---
id: "01-retire-foreground-idle"
title: "Retire projected foreground idle receipts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-PLATFORM-DELIVERY-RETENTION-002
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-001.1
  - AC-PLATFORM-DELIVERY-RETENTION-001.2
  - AC-PLATFORM-DELIVERY-RETENTION-002.1
  - AC-PLATFORM-DELIVERY-RETENTION-002.2
  - AC-PLATFORM-DELIVERY-RETENTION-005.2
  - AC-PLATFORM-DELIVERY-RETENTION-005.7
  - AC-PLATFORM-DELIVERY-RETENTION-006.6
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
  - ../../specs/platform/system-design/delivery-stream-storage.md
---

# Task 01. Retire projected foreground idle receipts

## Summary

Add `foreground_idle` to the two existing lists of supported control events.
Use the existing repository tests to check retirement through the idle event
and the output behind it.

## In scope

- Add `'foreground_idle'` to `agentDeliveryRetentionControlTypesSQL`.
- Add `streams.EventTypeForegroundIdle` to the nonterminal control case in
  `retentionEventPayloadSupported`.
- Insert an idle event before the final message in `seedRetentionControls`.

## Scope boundaries

Each row needs the existing projection, effect and expiry proofs. Response
reset retirement requires durable evidence of transcript retractions. The
actual terminal event settles a submission after foreground handoff.

## Acceptance

1. Preview and the committed batch agree on the eligible prefix. The batch
   advances the cursor through the idle event and removes the expired inbox
   rows with their exact generic effects.
2. The existing control tests cover recent replay, expiry, protected rows,
   rollback and restart with the updated fixture. Canonical history and
   session metadata survive retirement.

## Test sequence

1. Extend `seedRetentionControls` in `agent_delivery_retention_controls_test.go`
   with `streams.EventTypeForegroundIdle` before the final `message_chunk`.
2. Run `TestDeliveryRetentionControlPrefix` before the allowlist edits.
   Expect preview to stop before the idle event.
3. Add the two allowlist entries. Run the existing control suite on SQLite
   and PostgreSQL.

## Verification

Run the first command before the change to record the failing regression.
Run all commands after implementation. The two-engine checks use an isolated
PostgreSQL test DSN in `KANDEV_TEST_POSTGRES_DSN`.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^TestDeliveryRetentionControlPrefix$/SQLite$' -count=1 -v)
: "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test DSN}"
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^TestDeliveryRetentionControl' -count=1 -timeout=10m)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1 -timeout=10m)
bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/agent_delivery_retention_candidates.go apps/backend/internal/task/repository/sqlite/agent_delivery_retention_proof.go apps/backend/internal/task/repository/sqlite/agent_delivery_retention_controls_test.go
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_candidates.go`
  for the SQL type predicates.
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_proof.go`
  for typed payload eligibility.
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_controls_test.go`
  for the idle event in the existing fixture.

## Dependencies

None.

## Risks

Any protected row still limits how far the same stream can retire. An unresolved
submission keeps its prompt evidence after the idle receipt expires.

## Parallelism

`sequential`

## Inputs

- [Backend delivery retention requirements](../../specs/platform/requirements/backend-agent-delivery-retention.md).
- [Stream storage design](../../specs/platform/system-design/delivery-stream-storage.md#projected-control-notifications).
- [Handoff recovery design](../../specs/platform/system-design/session-recovery-convergence.md#foreground-handoff-and-unproven-sql-completion).
- [Retirement checkpoint decision](../../decisions/2026-10-05-backend-delivery-retirement-checkpoints.md).

## Results

RED confirmed on SQLite: with `foreground_idle` in the fixture and before the
allowlist edits, preview counted 9 of 11 receipts. After the SQL and Go
allowlist edits, the focused prefix test and the race-enabled
`TestDeliveryRetentionControl*` suite passed on SQLite and PostgreSQL using an
isolated PostgreSQL test schema.

Also passed: `go run ./cmd/sqlguard ./internal`, the race-enabled persistence
store conformance suite on SQLite and PostgreSQL, changed-file Go lint, both
specification validators, `make build`, and `git diff --check`. The build
reported that `codesign`/`rcodesign` are unavailable, so the macOS helper
binaries remain unsigned.
