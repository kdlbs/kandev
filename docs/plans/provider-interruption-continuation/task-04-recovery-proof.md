---
id: "04-recovery-proof"
title: "Prove isolated desktop and phone recovery"
status: completed
wave: 4
depends_on:
  - "03-recovery-feedback"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
acceptance_criteria:
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.4
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 04: Prove isolated desktop and phone recovery

## Summary

Use controlled provider frames and deterministic restore outcomes to prove the
complete user flow. Assert dispatched prompt identity and persisted state as
well as recovery UI, so a success label cannot conceal original-prompt replay.

## In scope

- Mock-only typed support using existing mock dialect provenance. Add named
  scenarios for output-only, completed read, pending read, write, unknown work,
  resumed progress before settlement, transient/hard restore failure, and
  ambiguous continuation acceptance. Prove missing native identity through
  real repository/lifecycle tests rather than a mock browser metadata override.
- Persist mock episode counters/accepted-prompt markers across owned process
  restarts and clean them in CloseSession/E2E reset, like the overload fixture.
  Original user prompt and continuation must be distinguishable without
  exposing real prompts or provider credentials.
- Shared bounded E2E helper for seeding and state evidence; feature enablement
  via `backend.restart(overrides)` and baseline restore, not inherited env.
- Desktop/phone scenarios from the plan's matrix; reload, two viewers, actual
  cancellation after admission, queued human prompt priority, backend restart,
  and live-adoption negative case.
- Keep `/transport-lost` data-only manual recovery and existing overload replay
  tests intact. Compare rendered UI-01/UI-02 structure, phone 44px controls,
  expanded technical details, and no document overflow.

## Out of scope

Live-user-session reproduction, real network outages, Docker daemon scenarios,
new provider signatures, mock support accepted by production adapters, and
generic full-suite verification.

## Acceptance

- Fixtures prove restoration of the same conversation, exactly one admitted
  continuation, no original-prompt resend, partial history preservation, and
  no successful workflow event from the interrupted turn. Unsafe cases are manual.
- Real cancellation, supersession, transient restore failure, reload/multi-viewer,
  and restart/adoption outcomes match the backend episode contract; persisted
  evidence agrees with visible state and notices disappear after successful cleanup.
- Both viewport projects discover and pass their focused tests; phone controls
  measure at least 44px and recovery/details fit without horizontal overflow.

## Verification

```bash
(cd apps/backend && go test -race ./cmd/mock-agent ./internal/agentctl/server/adapter/transport/acp -run 'Test(MockInterruptionContinuation|CursorContinuationEvidence|ContinuationNativeOnlyRestore)' -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/session/provider-interruption-continuation.spec.ts tests/session/transient-retry.spec.ts tests/session/transient-retry-transport-lost.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-provider-interruption-continuation.spec.ts tests/session/mobile-transient-retry.spec.ts)
```

Run project commands sequentially. Managed runner rebuilds production assets and
host mock/backend; do not reuse stale `--no-build` artifacts or pass all-worker
overrides. Record discovered counts and results. Use causal observations and
API polling; time-based assertions are not evidence of a dispatch outcome.

## Files likely touched

- `apps/backend/cmd/mock-agent/{handler,scenarios,emitter}.go` and focused tests
- Mock-only ACP dialect hook and tests
- `apps/web/e2e/helpers/provider-interruption-continuation.ts` (new)
- `apps/web/e2e/tests/session/provider-interruption-continuation.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-provider-interruption-continuation.spec.ts` (new)
- `apps/web/e2e/pages/session-page.ts` only for reusable scoped selectors
- Mock reset/close fixture cleanup where counters are owned

## Dependencies

Tasks 01-03 complete. The native proof from Task 01 remains a distinct support
prerequisite; deterministic mocked restore is not its replacement.

## Risks

A mock that takes a separate recovery route will miss production wiring. Route
it through the same typed snapshot and admission logic with mock-only provenance.
Cleanup omissions can contaminate another worker-scoped session.

## Parallelism

`sequential`

## Inputs

Plan E2E matrix and UI-01/UI-02; design Validation strategy; e2e and mobile-parity
skills; mock-agent AGENTS.md and current transient-retry helpers/specs.

## Results

Added mock-only attested RPC errors through the production safety ledger and
persisted same-ID output/read/write/pending/unknown scenarios. Mock and ACP wire
race tests pass. The desktop suite now has 14 continuation checks, alongside
six existing replay/copy regressions. Sequential Docker runs cover the matrix;
the final seven-case run rebuilt the latest source and passed every affected
acceptance/startup/restart path. The existing first-turn overload regression
also passed its isolated rerun after a timeout in the longer matrix run.

Rendered evidence checks same native ID, one original prompt, one continuation,
filtered restored history, successful turn settlement, transient and hard
restore failure, ambiguous acceptance, queued human work, waiting and accepted
cancellation, reload, two viewers, and restart with and without surviving work.
Missing native identity is proven at the real repository/lifecycle boundary,
not through an artificial browser metadata override. Earlier host Chromium
launches failed before application entry; all rendered verification uses the
managed Docker runner with one worker. Phone results are recorded in the plan.

Integration failures produced compiling regressions before production fixes:
startup error ownership, stopped-executor grace, accepted context lifetime,
shutdown notice persistence, and finishing startup ownership at acceptance.
Scoped race tests and final desktop integration checks pass after these fixes.
