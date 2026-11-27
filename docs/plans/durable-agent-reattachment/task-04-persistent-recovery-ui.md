---
id: "04-persistent-recovery-ui"
title: "Persist truthful session recovery and render it on desktop and phone"
status: done
wave: 4
depends_on: ["03-outcome-settlement"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.6
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.2
system_design:
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 04: Persist truthful session recovery and render it on desktop and phone

## Summary

Persist truthful session recovery and render it on desktop and phone. Preserve original ownership and the no-resend contract.

## In scope

Complete the task-owned persistent recovery view model and revisioned notification/hydration path.
Expose reconnecting and uncertain phases without relying on an in-memory FailureCode or a synthetic E2E metadata seed.
Keep DURABLE_DELIVERY_UNCERTAIN compatibility where the composer reads last_agent_error; avoid two conflicting state sources.
Prevent generic prompt rollback, turn completion, failure handlers, and stall settlement from presenting unresolved work as idle/ready.
Keep session recovery independent of the global local-runtime replacement alert; a healthy runtime can host a blocked session.
Retry queries/reconnects only, Stop remains accessible, and only matching delivery notices clear after recovery.
Use six-language localization, existing chat composition, and shared domain logic. Update public recovery guidance.

## Out of scope

Other work orders, contributor-owned executor redial, long-horizon scheduling, and detached MCP policy.

## Acceptance

- All named regressions exercise the actual owning boundary and pass.
- No stale owner, unrelated recovery cause, or automatic resend crosses the repaired path.
- Partial failure remains visible, bounded, and restart-reconcilable without deleting retained evidence.

## Tests

TestDeliveryDisconnectPersistsRecovery triggers an actual service disconnect and reads the stored session through normal API hydration.
TestDeliveryPromptRollbackPreservesUncertainty asserts no false turn completion or ready composer.
TestDeliveryRecoveryNoticeRevision rejects delayed old UI/HTTP updates and preserves unrelated errors.
Add durable-reattachment.spec.ts and mobile-durable-reattachment.spec.ts: actual disconnect, reload, retry, Stop, terminal recovery, and one next prompt.
Retain existing seeded component tests as presentation checks, not proof of the persistence path.

## ASCII UI preview

UI-01: Existing session chat recovery region, above the composer. Labels are illustrative localized copy.

```text
Desktop: [Saved conversation]
         Reconnecting to the agent...                  [Stop]
         (after the bounded window)
         Delivery uncertain. [Retry connection]        [Stop]

Phone:   [Saved conversation]
         Delivery uncertain.
         [Retry connection]
         [Stop]
```

After confirmed running reattachment, show normal running controls; do not enable a second prompt as if idle.
After terminal settlement, clear only this delivery notice and restore normal admission through existing guards.
Keep one chat scroll owner, phone safe-area clearance and 44px targets, and compact desktop controls.
No modal or new navigation is needed. Keyboard access and text status must work without color cues.
Session recovery can coexist with the global local-runtime alert. Stop reports unconfirmed cancellation honestly.
This view covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2 and 006.6.

See [combined preview](plan.md#ascii-ui-preview).

## Verification

Run from repository root after implementation. All commands are independently rooted.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/service ./internal/gateway/websocket -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
make -C apps/backend lint
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test hooks/domains/session/use-session-recovery-actions.test.ts hooks/domains/session/use-session-recovery-actions-guard.test.ts)
(cd apps/web && pnpm lint)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/durable-reattachment.spec.ts tests/session/durable-stream-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-durable-reattachment.spec.ts tests/session/mobile-durable-stream-recovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Persistence evidence must include fresh boot, replay/reopen, pre-change upgrade, interrupted settlement, and PostgreSQL conformance. Record unavailable PostgreSQL explicitly; do not count skipped tests as passed.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/task/service`
- `apps/web/components/task/chat/use-composer-props.ts`
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`
- `apps/web/e2e/tests/session/durable-stream-recovery.spec.ts`
- `apps/web/src/locales`
- `docs/public/sessions-and-review.md`

Add the named regression files beside the owning source packages.

## Dependencies

Task 03: Resolve only delivery blocks proved settled by durable evidence.

## Risks

Concurrent old events and new ownership can race settlement or cleanup. Use immutable identity and compare-and-set at the mutation boundary.

## Parallelism

`sequential`

## Inputs

- [Manifest](plan.md).
- [Requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Design](../../specs/platform/system-design/durable-agent-reattachment.md).

## Results

Completed 2026-09-28.

- Session recovery state persists through the task-owned repository and normal API/event hydration. Reconnecting and uncertain phases remain distinct from local runtime availability, retain `DURABLE_DELIVERY_UNCERTAIN` compatibility, keep Stop available, and do not reopen prompt admission or resend work.
- The backend disconnect regression `TestAgentctlDisconnectPersistsRecoveryAndPublishesWithoutSettlingSession` drives the real service path and reads the hydrated session. Desktop durable disconnect/reload/retry/Stop/terminal coverage passed 3/3 with `pnpm e2e:run --host --no-build --project chromium tests/session/durable-reattachment.spec.ts tests/session/durable-stream-recovery.spec.ts`; mobile coverage passed 2/2 with `pnpm e2e:run --host --no-build --project mobile-chrome tests/session/mobile-durable-reattachment.spec.ts tests/session/mobile-durable-stream-recovery.spec.ts`. Both verify one subsequent prompt and no resend of the uncertain submission.
- Live-survivor E2Es passed 2/2 on desktop and 1/1 on mobile, preserving the primary session identity and visible output through local backend restart.
- Related frontend recovery, rendering, and hydration tests passed 14/14 with `pnpm test lib/state/slices/session/session-merge-delivery-recovery.test.ts lib/state/slices/session/session-merge-goal.test.ts components/task/chat/use-composer-props.test.tsx lib/session-agent-delivery-recovery.test.ts`. The new RED test reproduced an older delivery revision replacing the settled notice and unrelated error; the revision fence now passes it. `pnpm lint`, `pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed. Public recovery guidance was updated in `docs/public/sessions-and-review.md` and `docs/public/operations.md`; `node --test scripts/validate-public-docs.test.mjs` passed 62/62 and `node scripts/validate-public-docs.mjs` accepted all 47 published pages.
- `go test -race ./internal/task/service ./internal/gateway/websocket -count=1` passed. The complete lifecycle race suite, orchestrator race suite, and focused SQLite/orchestrator settlement tests passed across the verified runs. The real disconnect, recovery revision, and prompt-admission regressions passed in the focused multi-package race run. SQL guard and SQLite store conformance passed. PostgreSQL conformance was skipped because `KANDEV_TEST_POSTGRES_DSN` is not configured.
- Native Windows/macOS process verification remains a release gate. No changes were committed.


### Latest-main configuration startup integration, 2026-10-06

Command autocomplete confirmation and retained-agent readiness now share the
observed startup execution identity. A live session can promote agentctl to
ready without losing that identity before the matching STARTING event.
The identity is consumed at STARTING or a live transition; a later startup
cannot restore stale confirmation. The ordering regression failed before the
fix, and the added later-startup regression also failed before its fix.

- Focused session handlers and state tests passed 134/134 with
  `pnpm exec vitest run lib/ws/handlers/session-confirmed-config-lifecycle.test.ts
  lib/state/slices/session/session-slice.upsert.test.ts
  lib/ws/handlers/agent-session.test.ts lib/ws/handlers/session-models.test.ts`.
- Command autocomplete, model hydration, and confirmed/startup configuration
  tests passed 75/75 across the seven affected suites.
- `pnpm lint`, `pnpm run typecheck`, and `pnpm run build:e2e` passed.
- The owning command autocomplete system design records the consumed execution
  identity. Public behavior remains the established same-execution confirmation
  and new-execution invalidation contract.

- Mobile managed E2E passed 2/2 with `pnpm e2e:run --host --no-build
  --shards 1 --project mobile-chrome
  tests/chat/mobile-cancel-turn-availability.spec.ts
  tests/session/mobile-provider-interruption-continuation.spec.ts -- --retries=0`.
  The cancellation test uses the shared settlement observation barrier to
  assert pending/disabled UI before forwarding the actual settlement frames.
  It still checks 44-pixel reachability and eventual background settlement.
- Catalog validation, full specification lint, 36 linter tests, changed-E2E
  ESLint/Prettier, and whitespace checks passed.
