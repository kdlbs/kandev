---
id: "04-prove-overload-containment"
title: "Prove overload containment"
status: done
wave: 4
depends_on:
  - "01-coalesce-agent-stream-ingress"
  - "02-isolate-replaceable-session-delivery"
  - "03-bound-frontend-stream-work"
plan: "plan.md"
spec: "../../specs/platform/requirements/bounded-task-status-delivery.md"
---

# Task 04: Prove Overload Containment

## Acceptance

- A deterministic mock ACP command emits a reported configurable reasoning
  burst suitable for pressure tests without a huge prompt or real-time delay.
- While one session emits the burst, desktop and mobile can switch to a quiet
  task/session and complete a correlated message action without waiting for the
  noisy backlog.
- The noisy session's final persisted reasoning is byte-for-byte exact;
  lifecycle publications, gateway pending work, and browser store work remain
  bounded and materially below source chunk count.
- Playwright attaches source, publication, replacement/eviction, frame/byte,
  latency, exact-content, and subscription-churn evidence.
- Broad backend/frontend checks pass and plan/task statuses record exact
  command results.

## TDD sequence

1. RED: add mock-agent parser/emitter tests for a configurable reasoning burst
   and an explicit produced-count marker.
2. RED: add the desktop pressure test and require exact transcript content plus
   quiet-session navigation/action progress during the active burst.
3. RED: run the same interaction through the native mobile task-switcher sheet
   and assert no horizontal overflow.
4. GREEN: fix only integration wiring or deterministic diagnostics; do not
   weaken burst-produced, exact-content, boundedness, or cross-session-progress
   assertions.
5. REFACTOR: reuse WebSocket traffic capture helpers, attach compact evidence,
   run broad verification, and update plan/task statuses/results.

## Files likely touched

- `apps/backend/cmd/mock-agent/script.go` or a focused scenario file
- mock-agent parser/emitter tests
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/conversion_test.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- new `apps/web/e2e/tests/session/session-stream-overload-isolation.spec.ts`
- new `apps/web/e2e/tests/session/mobile-session-stream-overload-isolation.spec.ts`
- `apps/web/e2e/helpers/session-stream-overload.ts`
- `apps/web/e2e/helpers/ws-traffic.ts`
- focused API/session page helpers
- this plan and task files for completion evidence

## Verification

```bash
cd apps/backend && go test ./cmd/mock-agent/...
cd apps/web && pnpm e2e:run tests/session/session-stream-overload-isolation.spec.ts \
  -- --project=chromium
cd apps/web && pnpm e2e:run tests/session/session-stream-overload-isolation.spec.ts \
  -- --project=mobile-chrome
make -C apps/backend test
make -C apps/backend lint
cd apps/web && pnpm exec vitest run
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run lint
```

## Dependencies

Tasks 01–03 must be complete so E2E observes the final containment stack.

## Parallelism

Sequential final integration/QA task.

## Inputs

- Incident task/session and event-count baseline in the parent plan.
- Existing `session-stream-budget.spec.ts`, `ws-traffic.ts`, mock-agent script
  commands, and mobile task-switcher E2E patterns.

## Risks

- A burst that finishes before interaction begins does not prove concurrency;
  synchronize on a produced/start marker and hold completion until the quiet
  action begins.
- Raw byte totals vary by fixture data and are diagnostic, not the correctness
  oracle.
- E2E must verify server-side production count so coalescing cannot make a
  quiet producer look successful.

## Output contract

Report the diagnostics exposed by the current proof harness: source chunk
count, exact persisted byte count, gateway frame/byte summaries, quiet-session
response, desktop/mobile results, verification commands, and files changed.
Lifecycle publication, DB/message update, replacement/eviction, maximum-depth,
browser-store update, latency, and content-hash counters are not exposed by
this E2E harness and are out of scope for this task.

## Verification results

- `go test ./cmd/mock-agent/...` — passed (184 tests); parser/default/cap,
  exact content, and script execution are covered.
- `go test -race ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/gateway/websocket ./cmd/mock-agent` — passed (2,633 tests).
- `make -C apps/backend test` — passed on the final run.
- `rtk proxy golangci-lint run ./...` — passed (0 issues).
- Desktop Chromium E2E — passed (1 test, 13.4s): exact 2,000-chunk persisted
  reasoning, quiet follow-up response, no horizontal overflow, and fewer
  received noisy replacements than source chunks.
- Mobile Chrome E2E — passed (1 test, 8.3s): native session-sheet switch,
  quiet follow-up response, exact reasoning, bounded noisy updates, and no
  horizontal overflow.
- A full web Vitest/typecheck/lint verification was completed for the changed
  package; focused Vitest covered 7 new tests.

The ACP/process handoff now applies cancellation-aware per-agent backpressure
instead of dropping normalized stream events when its bounded queue fills. The
E2E evidence attaches source chunk count, exact persisted byte count, gateway
frame/byte summaries, and quiet-session progress without transcript content.

## Files changed

- `apps/backend/cmd/mock-agent/reasoning_burst.go`
- `apps/backend/cmd/mock-agent/reasoning_burst_test.go`
- `apps/backend/cmd/mock-agent/script.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/conversion_test.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/helpers/session-stream-overload.ts`
- `apps/web/e2e/tests/session/session-stream-overload-isolation.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-stream-overload-isolation.spec.ts`
