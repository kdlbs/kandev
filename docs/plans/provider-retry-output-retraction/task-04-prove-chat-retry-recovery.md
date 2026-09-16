---
id: "04-prove-chat-retry-recovery"
title: "Prove chat retry recovery"
status: done
wave: 4
depends_on:
  - "03-apply-transcript-retractions"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.26
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.27
system_design:
  - ../../specs/platform/system-design/provider-response-attempt-recovery.md
---

# Task 04: Prove Chat Retry Recovery

## Summary

Add a controlled mock-agent response-retry sequence and drive it through a real
task session. Prove on desktop and mobile that abandoned assistant and thinking
rows disappear live, remain absent from the durable transcript, and do not
return after reload.

## In scope

- Mock-agent explicit-ID response and thinking emitters.
- Test-only structured response-attempt reset marker and mock ACP dialect.
- Deterministic `/e2e:response-retry` scenario.
- Desktop and mobile task-chat Playwright coverage.
- Existing frontend deletion-barrier unit test.

## Out of scope

- Production provider shortcuts for the mock marker.
- New task-chat markup, responsive composition, copy, or controls.
- Real provider credentials or network calls in tests.

## Acceptance

- The mock scenario exercises the same normalized reset event and durable
  deletion path as the Codex recognizer without making its marker valid for a
  production dialect.
- Desktop and mobile show only the replacement response after the live reset,
  and API readback plus reload contain no abandoned assistant or thinking row.
- The existing message-deletion barrier still prevents a queued update from
  resurrecting a removed row.

## Verification

```bash
(cd apps/backend && go test -race ./cmd/mock-agent ./internal/agentctl/server/adapter/transport/acp -run 'Test.*ResponseRetry')
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/ws/handlers/messages.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/session/provider-response-retry.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-provider-response-retry.spec.ts -- --retries=0)
```

## Files likely touched

- `apps/backend/cmd/mock-agent/emitter.go`
- `apps/backend/cmd/mock-agent/scenarios.go`
- `apps/backend/cmd/mock-agent/response_retry_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_mock.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_mock_test.go`
- `apps/web/e2e/tests/session/provider-response-retry.spec.ts`
- `apps/web/e2e/tests/session/mobile-provider-response-retry.spec.ts`

## Dependencies

- Task 03 completes the production deletion path exercised by the fixture.

## Risks

- A mock-only marker accepted by another dialect would create a production
  transcript-deletion input.
- E2E assertions that inspect only the DOM can miss a row that returns on
  reload; durable API readback and reload are both required.

## Parallelism

`sequential`

## Inputs

- Provider Error Recovery criteria `.26` and `.27`.
- Test strategy in the response-attempt recovery design.
- Existing desktop and mobile transient-retry chat scenarios.
- Existing frontend message-deletion semantic-barrier test.

## Results

Added explicit-ID mock assistant and thinking emitters, a mock-only structured
reset marker, and a deterministic `/e2e:response-retry` scenario. The mock ACP
dialect translates that marker through the same provider-neutral reset path as
Codex while production dialects ignore it. Desktop and mobile Playwright tests
observe the provisional rows, their live removal, the replacement-only durable
transcript, and the same result after reload.

Verified with:

```bash
(cd apps/backend && go test -race ./cmd/mock-agent ./internal/agentctl/server/adapter/transport/acp -run 'Test.*ResponseRetry' -count=1)
(cd apps/web && pnpm exec vitest run lib/ws/handlers/messages.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/session/provider-response-retry.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-provider-response-retry.spec.ts -- --retries=0)
```
