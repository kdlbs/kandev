---
created: 2026-09-17
status: implemented
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
legacy_specs: []
---

# Implementation Plan: Cursor RetriableError Expansion

## Overview

Extend the existing Cursor ACP transient-error projection to recognize any
bounded, non-empty diagnostic suffix after Cursor's `Error: RetriableError:`
prefix. This covers the observed `[unavailable] PING timed out` and
`Connection stalled` messages while retaining the existing structured error,
same-provider retry owner, retry budget, and effect-safety gate.

The work is one sequential backend slice because the ACP evidence matcher and
the routing catalogue must agree on the same signature contract before the
existing retry path can use it.

## Scope

### In scope

- Broaden current-prompt Cursor ACP control-frame recognition from one HTTP/2
  suffix to any bounded, non-empty `RetriableError` suffix, while vetoing
  cancellation, deadline, and retry-escalation signatures before projection.
- Keep control-frame suppression, notification-barrier ordering, stable safe
  provider-error projection, and Cursor-owned progress clearing unchanged.
- Broaden the existing Cursor catalogue rule to classify the same prefix-based
  diagnostics as high-confidence transient `agent_transport_lost` errors. Use
  the same Unicode-trimmed 256-byte suffix contract in both layers.
- Add positive coverage for the reported messages and negative coverage for
  prose, partial markers, other adapters, stale generations, and cancellation.
- Prove that broad classification still cannot schedule replay without current
  prompt evidence showing no output or tool activity.

### Out of scope

- Generic scanning of arbitrary assistant prose or other adapters.
- Signatures that exist only in ACP `RequestError.Data`.
- New retry counters, delays, policy settings, provider switching, or UI.
- Changes to the generic `transportLostRe` rule or the existing cancellation
  veto.

## Technical approach

- Update `isCursorRetriableStreamReset` in
  `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go`
  to retain Cursor identity, anchored prefix, bounded tail, and non-empty
  suffix checks while accepting the reported variants.
- Keep `adapter_updates.go` generation and role fencing, `adapter_prompt.go`
  notification-barrier settlement, and the fixed sanitized provider diagnostic.
- Update `cursor.retriable_stream_reset.v1` in
  `apps/backend/internal/agent/runtime/routingerr/runtime_rules.go` to use the
  same prefix-and-tail contract. Retain the rule ID and map to the existing
  `CodeAgentTransportLost` invariants.
- Preserve `promptAttemptPreResultSafe` as the authorization boundary. A
  classification match alone remains insufficient for automatic replay.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.12` | ACP dialect tests for `PING timed out`, `Connection stalled`, suppression, barrier settlement, cancellation vetoes, byte bounds, and anchored negative cases. |
| `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.13` | Existing ACP progress-clear and later-marker re-arm tests, extended with a non-HTTP/2 suffix. |
| `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.14` | Routing catalogue tests for both reported suffixes, stable rule identity, transient invariants, collision priority, cancellation veto, and adapter-aligned byte/Unicode-whitespace bounds. |
| `AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.15` | Existing orchestrator transport-loss replay tests, including safe and unsafe prompt evidence. |

## E2E tests

No new browser flow is required. The retry banner and manual recovery surface
are unchanged; this package changes only which backend Cursor diagnostics enter
the existing recovery path. Backend integration tests provide the new coverage
for the diagnostic-to-retry decision.

## Work orders

- [x] [Task 01: Broaden Cursor RetriableError matching](task-01-broaden-cursor-retriable-errors.md)

## Verification results

- Focused ACP, routing, and orchestrator race-enabled suites passed.
- Full ACP adapter race suite passed.
- Full routing catalogue race suite passed.
- The post-barrier ACP regression suite passes for context cancellation,
  deadline, and retry-escalation suffixes without projecting an auto-retryable
  transport error.
- Full orchestrator race suite passed.
- `make -C apps/backend lint` passed with 0 issues.
- Specification catalog validation, specification tests, full specification
  lint, `gofmt` checks, and `git diff --check` passed.

## Risks

- A suffix matcher that is not anchored can classify ordinary provider prose.
- A matcher that accepts an empty or oversized suffix can reintroduce partial
  diagnostic false positives.
- Broad classification must not bypass the existing prompt identity and
  no-output/no-effect replay fence.
