---
id: "02-log-delivery-recovery-refusals"
title: "Log why delivery recovery stays blocked"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
---

# Task 02: Log why delivery recovery stays blocked

## Summary

When boot-ready or explicit Resume leaves an `agent_delivery` recovery block
open, emit one WARN line with a closed refusal reason. If an explicit Resume
attempt owns the session, its boot-ready observation uses DEBUG to avoid a
second WARN. Today the error is dropped and only
`agent.boot_ready: delivery recovery remains required` is logged at DEBUG, so a
stuck session cannot be diagnosed.

## Regression test (write first, must fail)

New file `apps/backend/internal/orchestrator/native_resume_refusal_log_test.go`:

- Use an observed zap core. Drive `prepareBootDeliveryReadiness` and the
  explicit Resume reconciliation with a fake lifecycle that refuses with each
  sentinel the proof path can return (`ErrDeliveryRecoveryBlocked`,
  `ErrDurableAdoptionEvidenceUnavailable`, `ErrDurableAdoptionBlocked`, a plain
  error). Assert exactly one WARN entry per refusal with `session_id`,
  `block_id` and a `reason` from the closed set in the design section
  "Identity-less incident blocks", and no payload or token field.
- A successful resolution emits no WARN.

## Implementation

1. In `orchestrator/native_resume_readiness.go`, keep the returned error from
   `finishLoadedNativeDeliveryBlock` / `resolveDeliveryProtocol` and log it once
   at WARN before returning `false`. Map errors to the closed reason set with a
   small classifier; unknown errors map to `unclassified` and log the error text.
2. Where lifecycle refusals are currently anonymous (`return nil,
   ErrDeliveryRecoveryBlocked` in `lifecycle/loaded_native_reconciliation.go`
   and the proof helpers), wrap them with a typed reason so the classifier can
   distinguish `live_work`, `peer_record_missing`, `peer_record_mismatch`,
   `generation_superseded`, `native_identity_mismatch`, `journal_unresolved` and
   `projection_pending`. `errors.Is` against the existing sentinels must keep
   working.
3. The explicit Resume path (`RecoverSessionWithOptions`) logs the same line
   when it returns the recovery-required error.

## Out of scope

- Any change to what is accepted or refused (Task 03).

## Acceptance

- The new test passes with `-race`; existing native-resume tests pass unchanged.

## Result

- Added observed-log coverage for boot readiness and explicit Resume refusal
  paths, including sentinel classification, live work, and successful recovery.
- Preserved lifecycle refusal errors through readiness reconciliation and
  attached closed reason codes at lifecycle proof boundaries.
- The regression tests failed before logging was added and pass afterward.
- Passed:
  `go test -trimpath -tags fts5 -race ./internal/orchestrator/ -run 'NativeResume|DeliveryRecovery|RefusalLog' -count=1`
- Passed:
  `go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle/ -run 'NativeResume|LoadedNative' -count=1`
- Review fixes: recovery causes stay out of client-facing error text and are
  logged server-side; refusal reasons distinguish unavailable executions,
  unavailable evidence, and Office ownership. Bare catch-all sentinels map to
  `unclassified`. An active explicit Resume keeps the single WARN, while its
  boot-ready refusal is DEBUG. Regression tests cover these cases.

## Commands

```bash
cd apps/backend
go test -trimpath -tags fts5 -race ./internal/orchestrator/ -run 'NativeResume|DeliveryRecovery|RefusalLog|PrepareBootDeliveryReadinessLogsInternalCauseOnce|BootDeliveryRefusal' -count=1
go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle/ -run 'NativeResume|LoadedNative' -count=1
cd ../.. && bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```
