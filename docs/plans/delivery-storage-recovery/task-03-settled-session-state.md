---
id: "03-settled-session-state"
title: "Converge settled session state"
status: complete
wave: 3
depends_on:
  - 02-pressure-freshness
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
system_design:
  - ../../specs/platform/system-design/durable-delivery-failure.md
---

# Task 03: Converge settled session state

## Summary

Correct stale `STARTING` or `RUNNING` state only from an exact settled terminal and current idle runtime proof.
Keep the operation retryable and available for older completed settlement effects.

## In scope

- Add a narrow conditional operation at the settlement repository boundary, with SQLite/PostgreSQL parity.
- Use current binding, actual receipt, contiguous projection, terminal effect/outcome, settled turn, and idle runtime evidence.
- Check every current-owner submission for a successor, including mixed settled/live inventory.
- Integrate with terminal settlement and existing bounded startup/adoption and explicit-recovery callbacks.
- Return state-write errors to the recovery owner instead of completing bookkeeping without convergence.
- Publish committed session-state and summary updates through existing event paths.
- Cover completed-outbox repair, transient write failure, repeated reconciliation, and distinct next-prompt acceptance.

## Out of scope

Synthetic terminals or ready events, workflow evaluation, task movement, queue drain, historical outcome changes, blanket block removal, and direct live database repair.

## Acceptance

1. RED: `TestDeliverySessionStateConvergesAfterSettledCancellation` leaves a seeded real terminal and completed effect with a `RUNNING` session. GREEN: exact idle ownership permits one `WAITING_FOR_INPUT` correction without another prompt or completion effect.
2. A failed correction remains retryable. Reload, startup/adoption, explicit recovery, and repeat reconciliation reach the same committed state without duplicate events or workflow/queue effects.
3. A successor submission, active foreground tool, replacement, stale generation, missing receipt, archive, independent policy, or unavailable evidence prevents unsafe mutation. Independent recovery blocks and native identity remain intact.

## ASCII UI preview

Full contract: [UI-02](plan.md#ui-02-confirmed-delivery-recovery-and-settled-cancellation).
There is no new markup or action.

```text
UI-02, desktop and phone after exact settlement:
  Chat transcript and interruption history
  [ Existing composer ready for an eligible next message ]
```

An independent recovery block can keep the composer disabled.
The repair does not send the draft or repeat the cancelled instruction.
This preview maps to criteria 005.2, 005.3, and 006.5.

## Verification

Use `/tdd`. Install frontend workspace dependencies first if Task 02 did not do so.
Run from the repository root, sequentially:

```bash
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/task/repository/sqlite ./internal/orchestrator ./internal/agent/runtime/lifecycle -run 'DeliverySessionState|DeliveryTerminal|DeliverySettlement|TerminalSettlement|DeliveryHealth' -count=1)
(cd apps/backend && go run -trimpath ./cmd/sqlguard ./internal)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -p 1 -race -trimpath ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/agent_delivery_settlement.go apps/backend/internal/orchestrator/agent_delivery_recovery_state.go apps/backend/internal/orchestrator/event_handlers_streaming.go apps/backend/internal/task/repository/interface.go apps/backend/internal/task/repository/sqlite/agent_delivery_settlement.go
(cd apps/web && pnpm e2e:run --project chromium tests/session/resilient-output-delivery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-resilient-output-delivery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run the dialect-sensitive race cases against an isolated PostgreSQL database as a separate required gate:

```bash
: "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test database}"
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/task/repository/sqlite -run 'DeliverySessionState.*Postgres' -count=1 -v)
```

Record actual PostgreSQL execution. A skipped test does not satisfy this gate.
Use channels to pause owner proof and writer admission while successors, replacements, and foreground tools change.
Inject the state-write failure without exhausting disk capacity.
The retry case must receive no second terminal event.
Test both pending and already-completed effects, an unrelated fence, and one terminal plus one live successor.
An unavailable candidate must not block startup or recovery of another session.
Browser flows use the real backend and mock-agent terminal path.
Capture execution/native identity and pre-turn counts, then prove acceptance of exactly one distinct next instruction.
Include every additional changed Go file in changed-file lint.

## Files likely touched

- `apps/backend/internal/orchestrator/agent_delivery_settlement.go`, `agent_delivery_recovery_state.go`, and their tests.
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`, for error propagation from the waiting boundary.
- `apps/backend/internal/orchestrator/service.go` and `session_delivery_recovery.go`, for existing recovery/adoption hooks.
- `apps/backend/internal/orchestrator/agent_delivery_terminal_projection_test.go` and new `agent_delivery_session_state_test.go`.
- `apps/backend/internal/task/repository/interface.go`.
- `apps/backend/internal/task/repository/sqlite/agent_delivery_settlement.go` and its tests.
- New `apps/backend/internal/task/repository/sqlite/agent_delivery_session_state_test.go` and `agent_delivery_session_state_postgres_test.go`.
- The current lifecycle ownership facade if its idle-proof surface needs a bounded extension.
- `apps/web/e2e/tests/session/resilient-output-delivery.spec.ts` and `mobile-resilient-output-delivery.spec.ts`.
- `docs/public/sessions-and-review.md` if the final recovery description needs the settled-state clarification.

## Dependencies

[Task 02](task-02-pressure-freshness.md), after Task 01.

## Risks

- A persisted ready executor or terminal label alone does not prove current idle ownership.
- A completed outbox cannot be the only candidate filter for existing installations.
- Reusing the full ready handler can repeat workflow or queue side effects.
- Runtime and SQL locks can deadlock through synchronous event publication. Preserve existing lock order and immutable callback evidence.
- Missing incident logs leave the initial failed SQL operation unproven. The synthetic persisted-state regression must prove this repair independently.

## Parallelism

`sequential`

## Inputs

- [Delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md), requirements 005 and 006.
- [Settlement design](../../specs/platform/system-design/durable-delivery-failure.md#settled-session-state-convergence).
- Existing `AgentDeliverySettlementRepository`, `lockSessionTurnWrites`, `lockAgentDeliveryWrites`, session cancellation guard, and terminal effect tests.
- [Prompt-generation decision](../../decisions/0035-version-agent-ready-events-by-prompt-generation.md).

## Results

Implemented `CorrectSettledAgentDeliverySessionState` and `LoadCurrentSettledAgentDeliveryTerminal` at the repository settlement boundary.
The transaction revalidates the session, archive state, current harness generation, exact receipt, contiguous projection, terminal effect and outcome, settled turn, terminal submission, and every current-owner successor.
Orchestrator convergence runs from terminal settlement before outbox completion, from completed-outbox repair, from owned-runtime adoption, and from explicit recovery.
Idle current-execution proof and the existing turn/admission locks gate every correction.
SQLite retry, missing-evidence, archive, successor, and completed-outbox regressions passed.
PostgreSQL concurrency and live-successor tests passed against an isolated PostgreSQL 16 instance.
Desktop and phone resilient-output end-to-end tests passed with one distinct next prompt after a settled cancellation.
Review follow-up added cancel-in-flight guard serialization, an open/reserved successor-turn fence under the writer turn lock, explicit propagation of session-read failures before outbox completion, and a bounded completed-outbox candidate scan in all-session startup reconciliation. Reserved-turn regressions pass on SQLite and PostgreSQL, and injected read failures keep the effect retryable at both correction read boundaries.
