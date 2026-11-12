---
id: "01-operation-evidence"
title: "Preserve ACP authentication evidence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-AUTHENTICATION-001
acceptance_criteria:
  - AC-AGENTS-SESSION-AUTHENTICATION-001.1
  - AC-AGENTS-SESSION-AUTHENTICATION-001.5
system_design:
  - ../../specs/agents/system-design/session-authentication-recovery.md
---

# Task 01: Preserve ACP authentication evidence

## Summary

Carry typed authentication evidence across the ACP session-operation response and lifecycle wrapping.
Retain the existing fixed bootstrap cause without exposing provider payloads or depending on an auth event.

## In scope

- Add the shared `SessionOperationFailure` projection and its normalization tests.
- Recognize wrapped ACP `-32000` at actual new/load/resume/reset failure boundaries.
- Add optional `ErrorPayload.Details.session_failure`, retaining existing restore-specific response behavior.
- Snapshot advertised methods through an optional accessor, including code-only failures when methods are absent.
- Decode the response into a typed client error only for the matching session operation.
- Map the typed error in `bootstrapFailureFor` to `authentication_required` and fixed safe detail.
- Carry the bounded method snapshot transiently for work order 02.

## Out of scope

Recovery persistence, UI, credential repair, prompt-error policy, new protocol authentication operations, and legacy string parsing.

## Acceptance

1. `TestSessionAuthenticationFailureProjection`, `TestSessionOperationAuthenticationError`, and `TestBootstrapAuthenticationCause` fail before the correction and pass afterward.
   Cover nested wrappers, missing methods, all supported operations, and the actual API/client projection.
2. Permission errors, MCP setup failures, unrelated RPC codes, forged code text, malformed projections, cancellation, timeout, and unsupported transports preserve existing outcomes.
   A generic JSON-RPC internal error does not acquire authentication semantics.
3. The projection obeys the design's method count and field budgets.
   Injected tokens, private paths, and unsafe terminal metadata never enter durable or display-safe evidence.

## Verification

Run from the repository root. Start with the new test names to record the expected red result.
After implementation, run the complete block:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -p 1 ./internal/agentctl/types/... ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle -run 'Authentication|AuthRequired|SessionOperation|BootstrapAuthentication' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agentctl/types/session_failure.go apps/backend/internal/agentctl/server/api/agent.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go apps/backend/internal/agent/runtime/agentctl/agent.go apps/backend/internal/agent/runtime/lifecycle/bootstrap_failure.go
```

If implementation extracts another production Go file, include that file in the lint command before recording results.
Keep new tests in focused files rather than growing existing oversized suites.

## Files likely touched

- `apps/backend/internal/agentctl/types/session_failure.go` and `session_failure_test.go` (new).
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go` and `session_authentication_test.go` (new test).
- `apps/backend/internal/agentctl/server/api/agent.go` and `session_authentication_test.go` (new test).
- `apps/backend/internal/agent/runtime/agentctl/agent.go` and `session_authentication_test.go` (new test).
- `apps/backend/internal/agent/runtime/lifecycle/bootstrap_failure.go` and `bootstrap_authentication_test.go` (new test).

## Dependencies

None.

## Risks

Session load wraps errors in `SessionRestoreError`; preserve its independent reason and unwrap semantics.
Older peers have no optional projection and must retain safe fallback behavior.
Do not broaden the ACP predicate to provider prose or MCP credential failures.

## Parallelism

`sequential`

## Inputs

- [Design: protocol evidence and lifecycle propagation](../../specs/agents/system-design/session-authentication-recovery.md#protocol-evidence).
- [Plan evidence and compatibility matrix](plan.md#confirmed-evidence).
- Existing `auth_required_test.go`, client `agent_session_test.go`, and lifecycle `start_model_bootstrap_failure_test.go`.
- Scoped backend, agentctl, API, and backend test guidance.

## Results

Implemented typed ACP authentication evidence for all four session operations. Safe bounded methods travel in the optional error details projection; the client accepts it only for the matching operation and lifecycle retains the fixed bootstrap cause plus transient method snapshot. Native-state restore-required responses keep their existing code and details.

Verification passed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -p 1 ./internal/agentctl/types/... ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle -run 'Authentication|AuthRequired|SessionOperation|BootstrapAuthentication' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agentctl/types/session_failure.go apps/backend/internal/agentctl/types/session_failure_test.go apps/backend/internal/agentctl/server/adapter/transport/acp/session_failure.go apps/backend/internal/agentctl/server/adapter/transport/acp/session_failure_test.go apps/backend/internal/agentctl/server/adapter/adapter.go apps/backend/internal/agentctl/server/api/agent.go apps/backend/internal/agentctl/server/api/session_authentication_test.go apps/backend/internal/agent/runtime/agentctl/session_failure.go apps/backend/internal/agent/runtime/agentctl/agent.go apps/backend/internal/agent/runtime/agentctl/agent_session_test.go apps/backend/internal/agent/runtime/lifecycle/bootstrap_failure.go apps/backend/internal/agent/runtime/lifecycle/bootstrap_authentication_test.go
```

Review follow-up preserved the typed classification while hardening its transport boundaries. Authentication failures now receive a fixed WebSocket message and classification-only log fields, so `acp.RequestError.Data` is not returned or logged. `TestSessionAuthenticationFailureDoesNotExposeACPErrorData` covers new, load, resume, and reset.

MCP observations are held until the session operation emits its attempt preamble. The ACP adapter commits and queues attempt/evidence events under its ordered update path; authentication rejection discards provisional observations, and built-in MCP preflight failures now include explicit evidence. Focused coverage passed in `internal/mcp/server`, the ACP adapter, process manager, and API packages. The changed-file Go linter reported zero issues, the full backend build passed, and the API package suite passed with an isolated Go cache after the shared cache was invalidated during a concurrent run.
