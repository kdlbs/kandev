---
status: shipped
created: 2026-07-27
owner: Kandev
---

# Agent Resume and Runtime Recovery

## Broken behavior

An agent process or ACP handshake can fail before Kandev has attempted to load
the stored provider-native session. Kandev currently treats every such failure
as proof that the saved session is unusable and clears the operational resume
token. A later retry therefore starts a new provider-native conversation even
though the original session remains valid.

A failed or cancelled GitHub-backed session also requests a new scoped
credential lease while its persisted state is still terminal. The credential
broker correctly rejects terminal sessions, so the user-visible Resume action
cannot reach the agent launch.

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
  per-session resume lock before launching the agent. This makes the session
  eligible for a scoped GitHub credential lease without weakening the
  credential broker's terminal-session rejection.
- If launch fails after that early transition, Kandev restores the prior
  recoverable session state unless another terminal transition won the race.
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
  **THEN** the session is persisted as `STARTING` before the credential lease
  is requested and the launch can proceed.
- **GIVEN** that launch fails after the early `STARTING` transition, **WHEN**
  recovery handling completes, **THEN** the session is recoverable and no
  stale `STARTING` state remains.
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
