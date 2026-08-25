---
status: active
system: agents
created: 2026-07-27
owners:
  - Kandev
---
# Agent Resume and Runtime Recovery Requirements

## Overview

Preserve the observable behavior documented for Agent Resume and Runtime Recovery.

## Requirements

### REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001: Agent Resume and Runtime Recovery

**Intent:** Preserve the observable behavior documented for Agent Resume and Runtime Recovery.

#### Acceptance criteria

- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.1:** A process startup, ACP initialize, or transport failure does not discard the stored resume token. Resume retries the same provider-native session.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.2:** The stored token is cleared only when the user explicitly chooses **Start fresh**, or is replaced after the agent successfully creates a new provider-native session.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.3:** An authorized resume moves the task session to `STARTING` under the existing per-session resume lock before request assembly reaches scoped GitHub credential issuance. This makes the session eligible for a lease without weakening the credential broker's terminal-session rejection.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.4:** A successful resume persists the non-secret Git credential routing snapshot while the session is still guarded `STARTING`, so the task detail view does not retain an earlier workspace/executor credential policy.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.5:** If request assembly, credential issuance, or launch fails after that early transition, Kandev restores the prior recoverable session state unless another terminal transition won the race.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.6:** A completed turn remains represented by the task's review state while its persisted response and session lifecycle state settle. After a backend restart and automatic resume, the prior transcript remains visible and the task returns to the Turn Finished review bucket once the session is again `WAITING_FOR_INPUT`; it does not settle in Backlog or Running.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.7:** The explicit managed-runtime update path may invalidate only the deterministic `_npx` execution directory for the selected built-in package after an initial update failure, then retry once and run the normal ACP capability probe.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.8:** **GIVEN** a valid OpenCode resume token, **WHEN** the OpenCode child exits before answering ACP `initialize`, **THEN** Kandev shows the normal recovery action and retains the same token for the next Resume attempt.

## Migrated source detail

## Broken behavior

An agent process or ACP handshake can fail before Kandev has attempted to load
the stored provider-native session. Kandev currently treats every such failure
as proof that the saved session is unusable and clears the operational resume
token. A later retry therefore starts a new provider-native conversation even
though the original session remains valid.

A failed or cancelled GitHub-backed session also requests a new scoped
credential lease while its persisted state is still terminal. Resume assembles
the request before persisting `STARTING`, and request assembly itself issues
the lease; moving the transition only ahead of `LaunchAgent` therefore leaves
the broker boundary too early. The credential broker correctly rejects the
terminal session, so the user-visible Resume action cannot reach agent launch.

Managed npm runtimes can retain a truncated or otherwise corrupt extracted
`_npx` execution tree even when npm's content-addressable package cache is
healthy. Re-running the current update command can reuse that tree and fail
without repairing it.

## Expected behavior

- A process startup, ACP initialize, or transport failure does not discard the
  stored resume token. Resume retries the same provider-native session.
- The stored token is cleared only when the user explicitly chooses
  **Start fresh**, or is replaced after the agent successfully creates a new
  provider-native session.
- An authorized resume moves the task session to `STARTING` under the existing
  per-session resume lock before request assembly reaches scoped GitHub
  credential issuance. This makes the session eligible for a lease without
  weakening the credential broker's terminal-session rejection.
- A successful resume persists the non-secret Git credential routing snapshot
  while the session is still guarded `STARTING`, so the task detail view does
  not retain an earlier workspace/executor credential policy.
- If request assembly, credential issuance, or launch fails after that early
  transition, Kandev restores the prior recoverable session state unless
  another terminal transition won the race.
- A completed turn remains represented by the task's review state while its
  persisted response and session lifecycle state settle. After a backend
  restart and automatic resume, the prior transcript remains visible and the
  task returns to the Turn Finished review bucket once the session is again
  `WAITING_FOR_INPUT`; it does not settle in Backlog or Running.
- The explicit managed-runtime update path may invalidate only the
  deterministic `_npx` execution directory for the selected built-in package
  after an initial update failure, then retry once and run the normal ACP
  capability probe.

## Persistence and security constraints

- `task_sessions.metadata.acp.session_id` and
  `executors_running.resume_token` continue to identify the provider-native
  conversation until explicit fresh start or successful replacement.
- A failed pre-session launch must not blank either persisted identity.
- Resume state changes use the existing guarded session transition and
  publication path; direct unguarded state writes are not introduced.
- The GitHub credential broker continues to reject `COMPLETED`, `FAILED`, and
  `CANCELLED` sessions. Recovery obtains a lease only after the authorized
  resume has persisted `STARTING`.
- Runtime cache repair accepts only package names from built-in agent metadata,
  resolves npm's cache root through direct argv, and removes only
  `<cache>/_npx/<package-key>`. It never accepts a caller-provided path or runs
  a global cache clean.

## Regression scenarios

- **GIVEN** a valid OpenCode resume token, **WHEN** the OpenCode child exits
  before answering ACP `initialize`, **THEN** Kandev shows the normal recovery
  action and retains the same token for the next Resume attempt.
- **GIVEN** a resume attempt whose ACP transport disconnects before
  `session/load` completes, **WHEN** the attempt fails, **THEN** Kandev retains
  the token so a later healthy process can retry it.
- **GIVEN** a failed GitHub-backed session, **WHEN** the user selects Resume,
  **THEN** the session is persisted as `STARTING` before
  `buildResumeRequest` requests the credential lease and the launch can
  proceed.
- **GIVEN** a resume selects a credential policy different from the previous
  attempt, **WHEN** credential setup succeeds, **THEN** the non-secret
  `git_credential_snapshot` is persisted before launch and reflects the new
  policy.
- **GIVEN** that request construction, credential issuance, or launch fails
  after the early `STARTING` transition, **WHEN** recovery handling completes,
  **THEN** the session is recoverable and no stale `STARTING` state remains.
- **GIVEN** a task whose completed-turn response is persisted before its
  session lifecycle reaches its settled state, **WHEN** the backend restarts
  and the task page reloads, **THEN** the prior transcript remains visible and,
  after automatic resume settles at `WAITING_FOR_INPUT`, the task is shown in
  the Turn Finished review bucket rather than Backlog or Running.
- **GIVEN** an extracted managed npm runtime is corrupt, **WHEN** the first
  explicit update attempt fails, **THEN** only that package's deterministic
  execution directory is invalidated, the update runs once more, and success
  is reported only after ACP initialization succeeds.
- **GIVEN** the targeted cache repair or retry also fails, **WHEN** the update
  job becomes terminal, **THEN** it reports the bounded failure and performs no
  additional retry or broad cache deletion.

## Out of scope

- Silently falling back to a fresh provider-native conversation after Resume.
- Reconstructing or rewriting provider-owned conversation history.
- Automatically mutating npm caches on every normal agent launch.
- Global npm cache cleanup, exact runtime-version pins, rollback, or selection
  of historical runtime versions.
- Relaxing credential-broker authorization for terminal sessions.
- New recovery buttons or other frontend behavior.
