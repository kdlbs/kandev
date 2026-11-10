---
id: "01-start-model-transport-failure"
title: "Pass startup model transport failures through"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.35
system_design:
  - ../../specs/agents/system-design/session-startup-failure-explanations.md
---

# Task 01: Pass startup model transport failures through

## Summary

Return a startup `SetModel` failure with typed host-stream disconnection
evidence unchanged instead of wrapping it as a model application failure.

## In scope

- `isStartModelTransportError` in `start_model.go`, recognizing typed
  stream-disconnection, cancellation, and deadline identities.
- The strict failure branches of `applyStartModelPolicy`,
  `applyUniqueAdvertisedVariation`, and `applyAdvertisedFallback`.

## Out of scope

Auto-fallback, method-not-found, permission-mode application, and the
presentation of the generic bootstrap cause.

## Verification

- `go test ./internal/agent/runtime/lifecycle/ -run 'TestStartModelTransportFailureIsNotReportedAsModelApplicationFailure|TestStartModelProviderRejectionStaysModelApplicationFailure'`
- The existing start model tests in `start_model*_test.go`.

## Review remediation verification

- `cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'Test.*StartModel|TestApplicationFailureIdentifies|TestRequiredEmptyStartModel|TestApplyStartModelPolicy|TestStrictStartModel|TestInitializeAndPromptWithLayers_ProviderRestoredOmitsSavedStartupSettings' -count=1`
- Policy tests cover exact and compatible requested models, unique variation,
  and advertised fallback. Opaque provider replies mentioning transport loss
  remain application failures; cancellation and deadlines keep their meaning.
- Real backend WebSocket consumers cover startup, native resume, context reset,
  and the Kubernetes model helper. A pending SetModel loses its actual client
  stream. Provider rejection retains safe model evidence. Failure sends no
  prompt, while a successful startup control dispatches one.
- The fixtures do not launch a provider, exercise a real cluster, establish
  native-platform behavior, or distinguish opaque remote ACP failures.
- Verify the current-base merge result before protected delivery. Exact-head
  CI, review dispositions, and merge remain external delivery gates.
