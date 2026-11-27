---
id: "07-native-turn-steering"
title: "Native same-turn steering and delivery receipts"
status: pending
wave: 6
depends_on:
  - "04-controls-and-usage"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-EXPLICIT-STEERING-001
  - REQ-PLATFORM-EXPLICIT-STEERING-002
  - REQ-PLATFORM-EXPLICIT-STEERING-003
acceptance_criteria:
  - AC-PLATFORM-EXPLICIT-STEERING-001.1
  - AC-PLATFORM-EXPLICIT-STEERING-001.2
  - AC-PLATFORM-EXPLICIT-STEERING-001.3
  - AC-PLATFORM-EXPLICIT-STEERING-001.4
  - AC-PLATFORM-EXPLICIT-STEERING-001.5
  - AC-PLATFORM-EXPLICIT-STEERING-002.1
  - AC-PLATFORM-EXPLICIT-STEERING-002.2
  - AC-PLATFORM-EXPLICIT-STEERING-002.3
  - AC-PLATFORM-EXPLICIT-STEERING-002.4
  - AC-PLATFORM-EXPLICIT-STEERING-002.5
  - AC-PLATFORM-EXPLICIT-STEERING-003.2
  - AC-PLATFORM-EXPLICIT-STEERING-003.4
system_design:
  - ../../specs/platform/system-design/explicit-turn-steering.md
---

# Task 07: Native same-turn steering and delivery receipts

## Summary

Implement the provider-neutral explicit active-turn delivery path with Codex
turn/steer as the first native mapping. Reuse existing runtime boundaries while
preserving ACP semantics, queued messages, and the root turn's completion owner.

## In scope

- Normalized steering mode/availability/turn_ref through agent capabilities,
  runtime, session DTO/boot and websocket projection. Retain old supports_steering
  behavior; no native same-turn advertisement through an old boolean alone.
- `features.sameTurnSteering` / `KANDEV_FEATURES_SAME_TURN_STEERING`, off in all
  profiles, restart-required; current typed registry/config/frontend contract.
  Native path additionally requires codexAppServer, not agentBackgroundWork or
  claudeMidTurnSteering. Test independent flags and disabled side-effect paths.
- Typed Codex TurnSteerParams/Response and optional exact-turn capability. Keep
  `expectedTurnId` pinned, reject review/compact/unsupported shapes and do not
  apply ACP gate transfer or start a new root generation.
- Explicit active_turn intent at the existing message submission boundary,
  carrying turn_ref and client_delivery_id. Bypass next-turn queue only for this
  explicit mode; keep queue entries unchanged and legacy automatic sends intact.
- Session-admission reservation, one pending native send per root session, and
  correct drain release when completion/cancellation races dispatch. Native
  acknowledgement must come from turn/steer response, not agentctl's existing
  asynchronous fire-and-forget steer acknowledgement.
- Durable receipt plus one pending transcript message in an atomic transaction,
  payload fingerprints, replay lookup, crash uncertainty, delivery revisions,
  native echo correlation and definite versus ambiguous error mapping.
- Forward authorization, target identity, deadlines and typed outcomes through
  orchestrator/executor, runtime/lifecycle, agentctl and each executor transport.
  Unsupported exact-turn calls must never fall through to ordinary Prompt.
- Preserve root/child tool and question ownership, one completion and existing
  usage attribution. Validate inputs/attachments and changed turn-level options
  before dispatch, retaining the future-turn configuration for queue delivery.

## Out of scope

Child steering, new provider implementations, changes to legacy ACP handoff,
queue schema or ordering, automatic retry/fallback, and UI presentation (Task 08).

## Acceptance

1. TestNativeSteerWithQueuedMessages leaves the queue byte-for-byte logically
   unchanged while the active Codex turn receives input; TestNativeSteerSequential
   accepts another instruction after ack without waiting for root completion.
2. TestNativeSteerCompletionRace, TestNativeSteerUncertainReceipt,
   TestNativeSteerEchoDedup and TestNativeSteerSingleCompletionAndUsage prove exact
   ownership, replay protection, honest outcomes and attribution through the
   real orchestration/transport boundary, not only a mocked client function.
3. Feature matrix, unauthorized/stale/unsupported/busy input, old-client and ACP
   regressions pass; production SQLite/PostgreSQL receipt+message round trips
   and migration replay preserve legacy data. No real-provider claim from mocks.

## TDD and verification

Add named behavioral tests before implementation. Proposed test files below
are implementation targets. Use barrier schedules before dispatch, after native
write, after root completion, and after a successor starts. No timing sleeps.
From repository root, after workspace installation if this is a fresh worktree:

```bash
(cd apps/backend && go test -race ./pkg/codexappserver ./internal/agentctl/server/adapter/transport/codexappserver ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/orchestrator/handlers)
(cd apps/backend && go test ./internal/task/service ./internal/task/repository/sqlite ./internal/runtimeflags ./internal/common/config ./internal/profiles)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test ./internal/task/repository/sqlite -run '^TestPostgresSteeringReceipt' -count=1)
(cd apps/web && pnpm test -- lib/state/slices/features/features-contract.test.ts)
(cd apps/web && pnpm run typecheck)
git diff --check
```

Use the existing isolated PostgreSQL harness. Missing DSN/skips are incomplete
acceptance. Task 06 owns separate fake-executor and optional live conformance.

## Files likely touched

- `apps/backend/pkg/codexappserver/types.go`, `client.go`, proposed `client_steer_test.go`;
  native adapter proposed `steer.go` / `steer_test.go` and capability event emission.
- Existing optional adapter/runtime capabilities, `internal/agentctl/server/api/agent.go`,
  runtime agentctl prompt transport, lifecycle session prompt dispatch and
  `manager_interaction.go`; proposed focused native steering integration tests.
- Existing orchestrator `steer.go`, executor interaction and message-submission
  handlers; proposed `native_steer.go`, `native_steer_test.go` and handler tests.
- Proposed task model/service/repository `steering_receipt*.go` and tests,
  migration registration and interfaces; existing message metadata/delivery DTOs.
- Config, runtimeflags registry, root/embedded profiles, session capability DTOs,
  frontend feature defaults and normalized session event types.

## Dependencies

Task 04 provides shared controls/transport ownership patterns. This task does
not require enabling background-work visibility. Task 08 also waits for Task 05
so changes to the shared composer integrate sequentially.

## Risks

Existing agentctl steering acknowledges before provider response; copying it
would falsely report accepted delivery. Native user-message echoes can duplicate
persisted input. A timed-out RPC can still act remotely; receipts prevent retry,
not uncertainty. Generic UI defaults must not masquerade as changed overrides.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/explicit-turn-steering.md).
- [Design](../../specs/platform/system-design/explicit-turn-steering.md).
- [ADR](../../decisions/2026-09-27-explicit-same-turn-steering.md).
- Existing `steer_test.go`, runtime agentctl `prompt_steer_test.go`, lifecycle
  steering generation tests, pinned TurnSteerParams schema, and
  [legacy ACP plan](../mid-turn-steering/plan.md) as baseline evidence only.

## Results

Pending.
