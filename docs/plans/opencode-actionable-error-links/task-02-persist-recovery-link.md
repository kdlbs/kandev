---
id: "02-persist-recovery-link"
title: "Persist recovery links"
status: done
wave: 2
depends_on: ["01-normalize-action-url"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-stall-recovery.md"
---

# Task 02: Persist Recovery Links

## Acceptance

- `LastAgentError` persists a validated `remediation_url` without changing the
  plain `TaskSession.error_message` contract.
- Generic recoverable errors and quota-specialized errors both carry the link
  when the normalized provider diagnostic has one.
- The existing lifecycle generation guards still select the winning provider
  error; stale or invalid diagnostics cannot overwrite the persisted link.
- `session.state_changed` includes the structured metadata so connected Office
  clients receive the same value as a reload.

## Verification

```text
cd apps/backend && go test ./internal/task/models ./internal/orchestrator ./internal/agent/runtime/lifecycle -run 'Test(LastAgentError|HandleRecoverableFailure|CreateRecoveryStatusMessage|.*ProviderError)' -count=1
```

Assert the URL is present only in the dedicated field, while `message`,
`error_output`, `error_message`, and generic logs remain URL-free.

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_test.go`
- `apps/backend/internal/orchestrator/recovery_actions_test.go`
- relevant lifecycle/session-state event tests

## Dependencies and risks

Task 01. Reuse the existing `ProviderError` propagation; do not create a second
diagnostic event or a parallel session state. Preserve local dismissal and
last-error stamping behavior.

## Results

Implemented 2026-08-07.

- `models.LastAgentError.RemediationURL` persisted under
  `SessionMetaKeyLastAgentError`; plain `TaskSession.error_message` contract
  unchanged and URL-free.
- `providerRemediationURL` copies only the adapter-validated field from the
  normalized `ProviderError`; `createRecoveryStatusMessage` carries
  `remediation_url` in the action-message metadata independently of quota
  classification (ACP-sourced diagnostics are not quota-classified but still
  show the link). The `task_session.error_changed` event carries it, and
  `session.state_changed` includes it via the persisted `session_metadata`.
- Tests: quota + generic cards carry the URL; nil/invalid diagnostics produce
  no metadata key; `handleRecoverableFailure` persists the URL and keeps
  `error_message` URL-free; absent diagnostics omit the field.

Verification: `go test ./internal/task/models ./internal/orchestrator ./internal/agent/runtime/lifecycle -run 'Test(LastAgentError|HandleRecoverableFailure|CreateRecoveryStatusMessage|.*ProviderError)' -count=1` — passed; full orchestrator (3094 tests) and lifecycle (1962 tests) suites passed.
