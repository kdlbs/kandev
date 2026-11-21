---
id: "02-durable-recovery"
title: "Persist authentication recovery"
status: done
wave: 2
depends_on:
  - "01-operation-evidence"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-AUTHENTICATION-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-SESSION-AUTHENTICATION-001.1
  - AC-AGENTS-SESSION-AUTHENTICATION-001.2
  - AC-AGENTS-SESSION-AUTHENTICATION-001.3
  - AC-AGENTS-SESSION-AUTHENTICATION-001.4
  - AC-AGENTS-SESSION-AUTHENTICATION-001.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.24
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.26
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.5
system_design:
  - ../../specs/agents/system-design/session-authentication-recovery.md
---

# Task 02: Persist authentication recovery

## Summary

Retain the established authentication decision across startup routing, durable causes, transcript metadata, and recovery actions.
Make the result independent of generic display text, a retained error object, or a live failed execution.

## In scope

- Remove the managed-runtime-only restriction from auth evidence propagation in `handleAgentStartFailed`.
- Add one structured authentication predicate shared by recovery metadata and actions.
- Carry the safe method snapshot from work order 01 through failure data and persist it before cleanup.
- Keep task category, bootstrap cause, and routing code in their existing domains.
- Cover both orchestrator and executor durable failure projections.
- Reopen the test repository and reconstruct current/history error state without in-memory capabilities.
- Exercise connection-scoped startup event ordering with captured exact-execution and attempt identities.
- Preserve absent native identity for initial failure and retained identity for supported restoration.

## Out of scope

UI markup, credential mutation, automatic login, journal ownership exceptions, schema changes, and new recovery actions.

## Acceptance

1. The real `handleAgentStartFailed` path persists `authentication_required`, safe detail, and `is_auth_error: true` without `ManagedRuntimeStartupError`.
   Cached `cursor_login`, empty methods, unavailable caches, and cleanup before hydration preserve the same classification.
2. Repository reopen retains the current cause, matching transcript metadata, and safe method snapshot.
   Initial failure offers authentication-aware restart without Resume; supported restore keeps its valid token and eligible Resume.
3. Before/after/absent/duplicate auth-event cases converge without duplicate current controls or unrelated session mutation.
   Stale attempts, cancelled sessions, successor executions, unsafe method fields, and unrelated errors retain their guards and classifications.

## Verification

Run from the repository root. First record the new focused tests failing against the uncorrected recovery projection.
After implementation, run:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race -p 1 ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle -run 'Authentication|BootstrapFailureProjection|BootstrapFailureSuccessorFence|AuthenticateBySessionID' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/event_handlers_agent.go apps/backend/internal/orchestrator/auth_error.go apps/backend/internal/orchestrator/watcher/watcher.go apps/backend/internal/orchestrator/executor/launch_failure.go
```

Include any extracted production Go files in the lint command before recording results.
Use channel barriers for event races, not sleeps.
Repository-reopen tests must read persisted rows after closing and reopening their disposable database.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`, `auth_error.go`, and `watcher/watcher.go`.
- `apps/backend/internal/orchestrator/startup_authentication_recovery_test.go` (new).
- `apps/backend/internal/orchestrator/recovery_authentication_test.go` (new).
- `apps/backend/internal/orchestrator/executor/launch_failure.go` and `executor_launch_failure_classification_test.go`.
- `apps/backend/internal/agent/runtime/lifecycle/authentication_startup_ordering_test.go` (new).
- Existing lifecycle delivery bootstrap fixtures and authenticate delegation tests, as needed for integration coverage.

## Dependencies

Work order 01 supplies the typed operation error and safe method snapshot.

## Risks

Do not set a routing code as a task error category or replace specific causes with generic wrapper text.
Do not clear a valid resume token or admit a rejected startup event to obtain method data.
Failure-message repair and repeated events must retain their existing idempotency.

## Parallelism

`sequential`

## Inputs

- [Design: recovery projection, ordering, and retry](../../specs/agents/system-design/session-authentication-recovery.md#recovery-projection-and-durable-reads).
- Existing `recovery_actions_test.go`, `event_handlers_agent_error_routes_test.go`, and bootstrap successor-fence tests.
- Lifecycle `delivery_stream_bootstrap_preflight_test.go` and `manager_interaction_session_ops_test.go`.
- Work order 01 results and current task/session ownership contracts.

## Results

Implemented one structured authentication predicate for startup routing, transcript metadata, and recovery actions. The orchestrator carries the bounded method snapshot transiently, persists the normalized authentication cause before cleanup, and only consults cached methods when the same execution still owns the session. The executor retains the generic bootstrap category and persists the same safe authentication cause. Added repository-reopen, code-only guidance, stale/cancelled ownership, resume eligibility, cached-method identity, and connection-scoped event-order coverage.

Verification passed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race -p 1 ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle -run 'Authentication|BootstrapFailureProjection|BootstrapFailureSuccessorFence|AuthenticateBySessionID' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/event_handlers_agent.go apps/backend/internal/orchestrator/auth_error.go apps/backend/internal/orchestrator/watcher/watcher.go
```
