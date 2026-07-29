---
spec: docs/specs/agent-resume-runtime-recovery/spec.md
related_specs:
  - docs/specs/agents/runtime-updates.md
created: 2026-07-27
status: implemented
---

# Implementation Plan: Agent Resume and Runtime Recovery

## Overview

Preserve provider-native session identity across failures that happen before a
successful replacement, move the guarded resume-state transition ahead of
GitHub credential issuance, and make explicit managed npm updates repair one
corrupt package execution tree. The changes stay inside existing recovery,
credential, and update surfaces; there is no API or frontend contract change.

## Confirmed root causes

- `handleAgentFailed` classifies any pre-initialization failure with a non-empty
  `executors_running.resume_token` as an unusable saved session and calls
  `clearResumeToken`, even when ACP `initialize` never returned and
  `session/load` was never reached.
- `Executor.ResumeSession` calls `agentManager.LaunchAgent` before
  `persistResumeState`. GitHub credential issuance occurs inside that launch,
  while `githubBrokerScopeAuthorizer` still sees `FAILED` or `CANCELLED` and
  rejects the lease as terminal.
- `CacheUpdateCommand` can reuse an already-extracted `_npx` directory. npm's
  package-content cache can be valid while that execution tree is truncated,
  so another `npm exec` does not necessarily repair it.

---

## Backend

Tasks 01–03 are complete; implementation and focused verification are done.

### Preserve resume identity on startup failure

- Update
  `apps/backend/internal/orchestrator/event_handlers_agent.go` so agent startup
  and ACP initialization failures follow the existing recoverable-failure path
  without automatically clearing the resume token.
- Keep `clearResumeToken` as the implementation of the explicit
  `session.recover` `fresh_start` action. Remove or narrow the obsolete
  automatic `handleResumeFailure` path and its misleading status message.
- Update `apps/backend/internal/orchestrator/event_handlers_test.go` with a
  failing regression fixture containing a resume token, an uninitialized
  execution, and an ACP initialize/transport failure. Assert that the token is
  retained and recovery state/actions remain available.

### Persist STARTING before credential issuance

- Refactor `apps/backend/internal/orchestrator/executor/executor_resume.go` so
  the existing `persistResumeState` guarded `FAILED|CANCELLED -> STARTING`
  transition occurs under the per-session lock after stale-execution cleanup
  but before `agentManager.LaunchAgent`.
- Capture whether the request began from a terminal state before mutating the
  session snapshot, so the existing stale-agent fallback keeps its terminal
  cleanup semantics.
- Add a guarded rollback helper that restores the prior recoverable state when
  launch fails only if the session is still `STARTING`; preserve a concurrent
  `COMPLETED`, `FAILED`, or `CANCELLED` winner.
- Keep
  `apps/backend/internal/backendapp/services.go` terminal-session credential
  rejection unchanged.
- Add executor-level and service-level regression coverage in
  `apps/backend/internal/orchestrator/executor/executor_resume_test.go` and
  `apps/backend/internal/orchestrator/task_operations_resume_test.go`. The fake
  launch boundary must read the repository and reject any lease attempt unless
  the session is already `STARTING`.

### Repair one managed npm execution cache

- Add a deterministic npm execution-cache key helper for
  `agents.ManagedNPMRuntimeSpec` in
  `apps/backend/internal/agent/agents/managed_npm_runtime.go`. The key is the
  first 16 lowercase hex characters of SHA-512 over the trusted package spec,
  matching npm's `_npx` package key.
- Extend the runtime-updater boundary in
  `apps/backend/internal/agent/settings/controller/agent_update.go` with a
  targeted execution-cache invalidation operation. The production
  implementation obtains npm's cache root with direct argv
  `npm config get cache`, requires an absolute clean root, constructs exactly
  `<root>/_npx/<key>`, and removes only that path.
- Update
  `apps/backend/internal/agent/settings/controller/agent_update_job.go` to
  invoke the targeted repair only after the first update command fails, append
  bounded recovery progress, and retry the same built-in update command once.
  A repair or second update failure ends the job.
- Document the operator recovery behavior in
  `docs/public/agents-and-profiles.md`.

---

## Tests

- **What:** a pre-ACP failure with a resume token retains the token and enters
  ordinary recoverable failure handling; explicit fresh start still clears it.
  **File:** `apps/backend/internal/orchestrator/event_handlers_test.go` and
  `apps/backend/internal/orchestrator/session_launch_test.go`.
  **How:** real temporary SQLite repository plus existing mock agent manager and
  direct recovery-service call.
- **What:** terminal resume is persisted as `STARTING` before the launch
  boundary requests credentials, terminal stale cleanup still runs, and a
  launch error cannot strand `STARTING`.
  **File:**
  `apps/backend/internal/orchestrator/executor/executor_resume_test.go` and
  `apps/backend/internal/orchestrator/task_operations_resume_test.go`.
  **How:** repository-observing fake `LaunchAgent`, table-driven prior states,
  and a concurrent-terminal-winner case.
- **What:** deterministic keys match npm for built-in packages and cache
  invalidation cannot escape the resolved `_npx` root.
  **File:**
  `apps/backend/internal/agent/agents/managed_npm_runtime_test.go` and
  `apps/backend/internal/agent/settings/controller/agent_update_test.go`.
  **How:** table-driven key assertions plus a temporary cache root and fake
  direct-command executor.
- **What:** an initial update failure triggers exactly one targeted repair and
  one retry; success still requires refresh, while repair/retry failures are
  terminal.
  **File:**
  `apps/backend/internal/agent/settings/controller/agent_update_test.go`.
  **How:** fake runtime updater call ordering and terminal job snapshots.

No E2E test is planned because the existing Resume, Start fresh, and Update
agent UI contracts do not change; backend tests exercise the repaired behavior.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Preserve resume tokens on startup failure](task-01-preserve-resume-token.md)
- [x] [Task 03: Repair corrupt managed npm execution caches](task-03-managed-npm-cache-repair.md) (`parallel-safe`; disjoint agent-settings/runtime files)

Wave 2:

- [x] [Task 02: Transition resume state before credential issuance](task-02-resume-state-before-credentials.md)

Execution remains sequential in the primary conversation by default. The
parallel-safe label does not authorize delegation.

## Validation commands

- `make -C apps/backend test`
- Targeted commands are recorded in each task file and run before the broad
  backend package check.

## Risks and non-goals

- npm's `_npx` key is an implementation detail; key tests pin the behavior used
  by the supported npm runtime and a repair failure remains bounded.
- Moving `STARTING` earlier changes when observers see resume progress; guarded
  rollback and state-race tests prevent stale active state.
- The plan does not weaken credential authorization, auto-start fresh
  conversations, clean global npm caches, or add launch-time cache mutation.
