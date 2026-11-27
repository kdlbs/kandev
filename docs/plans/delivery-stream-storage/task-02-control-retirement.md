---
id: "02-control-retirement"
title: "Retire projected control prefixes"
status: done
wave: 2
depends_on: ["01-shell-snapshots"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-PLATFORM-DELIVERY-RETENTION-002
  - REQ-PLATFORM-DELIVERY-RETENTION-003
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-001.1
  - AC-PLATFORM-DELIVERY-RETENTION-001.2
  - AC-PLATFORM-DELIVERY-RETENTION-002.1
  - AC-PLATFORM-DELIVERY-RETENTION-002.2
  - AC-PLATFORM-DELIVERY-RETENTION-002.3
  - AC-PLATFORM-DELIVERY-RETENTION-003.2
  - AC-PLATFORM-DELIVERY-RETENTION-005.1
  - AC-PLATFORM-DELIVERY-RETENTION-005.2
  - AC-PLATFORM-DELIVERY-RETENTION-005.7
  - AC-PLATFORM-DELIVERY-RETENTION-006.6
system_design:
  - ../../specs/platform/system-design/delivery-stream-storage.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention-payloads.md
---

# Task 02: Retire projected control prefixes

## Summary

Permit known, proved control notifications to compact and retire with the existing
expired contiguous prefix. Correct bootstrap pinning without weakening identity,
hash, consumer-effect, terminal/submission or transaction proofs.

## In scope

- Ground each of the nine initial control types in its current consumed-state and
  replay path. Keep the classifier consistent across SQL, reload, decoding and preview.
- Existing raw controls, compact receipts, format-1 hashes and old progress cursors.
- SQLite and PostgreSQL prefix retirement, replay/conflict/expiry, graceful restart,
  interrupted transaction rollback and ownership/projection races.
- Mixed fixtures with a proved control before output and an unresolved/unsupported
  event later in the same stream; every deleted row needs all its own proofs.

## Out of scope

Permission events, response-attempt reset, new authority/settlement effects, broad
nonterminal eligibility, schema migration, checkpoint leap and physical compaction.

## Acceptance

1. A realistic expired bootstrap sequence beginning with agent_capabilities advances
   its retired cursor and removes exact generic effects on both engines. Recent
   controls compact with valid hashes; identical duplicates do not rerun consumers.
2. Unprojected, unknown, malformed, user-role, pending/unknown-effect, corrupt-hash,
   missing-submission and protected-terminal rows remain intact. A single unsafe row
   stops advancement even with eligible rows after it; canonical/current session
   state and terminal/workflow effects remain unchanged.
3. Preview and execution agree; old rows are reached through bounded indexed work,
   restart resumes committed progress and rollback leaves payloads/checkpoints intact.
   Receive/projection races preserve authenticated expiry and cursor bounds.

## TDD evidence

New `agent_delivery_retention_controls_test.go`: `TestDeliveryRetentionControlPrefix`,
`TestDeliveryRetentionControlProtectedBoundary`, `TestDeliveryRetentionControlReplay`
and `TestDeliveryRetentionControlRestart`. Reuse real both-engine repository helpers
from direct/storage/historical tests. The first test must fail on the current zero
cursor. Include nine typed positive shapes and mixed negatives, not a mirrored list-only
unit test. Add/extend storage query-plan coverage if discovery predicates change.

## Verification

PostgreSQL proof requires an isolated database using the repository's existing
test-server setup. Do not use the developer's production data or claim skipped
PostgreSQL subtests passed. Run both command blocks with the DSN set.

```bash
: "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test DSN}"
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Control|Storage|Concurrency|Integration|Historical)|TestAgentDelivery' -count=1 -timeout=15m)
(cd apps/backend && go test -trimpath -race ./internal/task/deliveryretention -count=1 -timeout=3m)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1 -timeout=5m)
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_candidates.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_proof.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_events.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_preview.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_discovery.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_controls_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_replay_projection_test.go`

## Dependencies

Task 01 precedes this order for sequential delivery. The code boundaries are distinct,
but no parallel implementation or additional agents are authorized.

## Risks

Bootstrap submission IDs can be empty; supplied IDs must retain exact identity proof.
Consumer receipt completion does not establish missing workflow or permission authority.
Future control types remain protected until their independent proof is implemented.

## Parallelism

`sequential`

## Inputs

- Stream storage design: projected controls, replay/checkpoints and transaction boundaries.
- Parent retention and independent payload-proof designs.
- Checkpoint and barrier ADRs, existing direct/storage/replay test fixtures.
- Lifecycle `processPreparedAgentEvent` and orchestrator control handlers.

## Results

Completed. RED: realistic bootstrap controls produced zero retireable prefix candidates on both SQLite and PostgreSQL. GREEN: all ten expired fixture events retire with exact generic effects; recent controls retain hash/conflict checks. Mixed unknown, pending, missing-effect, unprojected, user-role, absent-submission, unknown-effect and unsettled-terminal cases stop the prefix. Transaction rollback and repository restart preserve/resume evidence, and current session metadata and canonical messages remain unchanged.

With an isolated PostgreSQL 17 DSN, all task-defined commands passed:

- Repository race command: 364.470 seconds, SQLite and PostgreSQL subtests enabled.
- Delivery-retention worker race package: 1.106 seconds.
- SQL guard: passed.
- Persisted-store conformance race package: 218.474 seconds, PostgreSQL enabled.
- Focused control race regressions: 25.842 seconds after final fixture edits.

No schema, wire, retention format, checkpoint semantics or settlement authority changed.
