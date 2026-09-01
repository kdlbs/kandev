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

### REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002: Visible resume failure feedback

**Intent:** Give the user the real failure cause and a visible next action when session recovery fails.

#### Acceptance criteria

- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.1:** A failed `session.recover` request shows the backend error near the recovery controls. The error remains visible after the request ends and after repeated attempts.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.2:** A failed automatic resume that restores the workspace in read-only mode shows a nonblocking notice. The notice includes the resume failure cause and states that the restored workspace is read-only.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.3:** If resume and read-only workspace restore both fail, the recovery surface shows both causes. It does not replace them with only a generic message.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.4:** A user can select read-only workspace restore after a manual resume failure. Kandev does not silently replace a manual Resume request with read-only restore.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.5:** Desktop, narrow desktop, and mobile task views use the existing inline alert and chat recovery patterns. Recovery actions remain reachable by keyboard and touch without horizontal overflow.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.6:** A Retry recovery control is disabled while its recovery request is in flight on every recovery surface. A repeated attempt cannot create overlapping `session.recover` requests.

### REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003: Explicit continuation after branch loss

**Intent:** Continue the same provider conversation on a new branch when the old branch cannot be recovered.

#### Acceptance criteria

- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.1:** When resume returns an error that matches `worktree.ErrBranchUnrecoverable`, Kandev offers an explicit **Continue on a new branch** action. Kandev does not switch branches before the user selects this action.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.2:** The action keeps the same `TaskSession`, stored resume token, and ACP session identity. It creates a fresh worktree on a unique branch from the task repository's configured base branch.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.3:** The new branch uses the normal worktree branch template and suffix generation. It remains owned by the task environment and does not create a session-owned worktree record.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.4:** After continuation succeeds, the chat contains a persisted warning. The warning states that the original branch is gone, the conversation history continues, and code changes from the lost branch were not recovered.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.5:** The warning has `variant = "warning"` and `kind = "branch_recreated"`. Its structured metadata contains the original branch, new branch, base branch, task session ID, and repository ID.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.6:** Warning persistence is idempotent for one branch replacement. Reconnect, replay, and page reload do not create duplicate messages.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.7:** In a task with multiple repositories, Kandev replaces only worktrees whose branches are confirmed unrecoverable. Valid worktrees continue to use their current branches. Kandev persists one warning for each replaced branch.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.8:** Other resume failures do not offer branch replacement. **Start fresh** keeps its current behavior and remains the only recovery action that intentionally clears the stored conversation identity.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.9:** Attach-only worktree preflight treats a branch as unrecoverable only after a bounded noninteractive probe confirms that the configured remote does not contain it. Authentication, network, timeout, and other probe failures remain ordinary recovery failures.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.10:** If replacement checkout creation succeeds but persistence of the existing task-environment repository record fails, Kandev removes the replacement checkout and its newly created branch. The prior record remains authoritative for retry.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.11:** Once explicit replacement has materialized, Kandev attempts warning persistence on every terminal resume path, including provider startup and readiness failures. A later retry can recover a warning after a failed message write.

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
- A failed resume stays visible with the backend cause and a recovery action.
- An automatic read-only restore states that resume failed and that the
  restored workspace is read-only.
- A confirmed missing branch offers explicit continuation on a new branch from
  the task base branch. The same conversation continues, but lost code does
  not return.

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
- Branch replacement updates the task environment repository record. It does
  not introduce a session-owned worktree record.
- Branch replacement does not clear or replace the stored resume token before
  the provider resumes the existing conversation.
- The branch replacement option is available only for a typed error that wraps
  `worktree.ErrBranchUnrecoverable`. Network, authentication, and transient Git
  errors remain failures.
- Attach-only branch-loss classification uses a bounded noninteractive remote
  probe when local and remote-tracking refs are absent. A missing tracking ref
  alone is not proof that the configured remote branch was deleted.
- Replacement persistence is compensating: if the existing task-environment
  repository update fails after checkout creation, the new checkout and branch
  are removed and the old database record is left unchanged.
- The persisted warning uses an atomic metadata claim before message creation.
  A failed message write releases the claim so a later retry can persist it.

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
- **GIVEN** a manual Resume request fails, **WHEN** the backend returns a
  descriptive error, **THEN** the recovery surface keeps that message visible
  and offers the applicable next actions.
- **GIVEN** automatic resume fails and read-only restore succeeds, **WHEN** the
  task view loads, **THEN** it shows the resume cause and states that the
  workspace is read-only.
- **GIVEN** automatic resume and read-only restore both fail, **WHEN** the task
  view loads, **THEN** it shows both failure causes.
- **GIVEN** the stored worktree branch is absent locally and on its configured
  remote, **WHEN** Resume fails, **THEN** the UI offers **Continue on a new
  branch** and does not run it automatically.
- **GIVEN** the user selects **Continue on a new branch**, **WHEN** recovery
  succeeds, **THEN** the same task session resumes through its stored provider
  identity on a unique branch from the configured base branch.
- **GIVEN** branch continuation succeeds, **WHEN** the chat reloads or replays
  events, **THEN** one warning identifies the old branch, new branch, and base
  branch and states that the old code was not recovered.
- **GIVEN** local and remote-tracking refs are absent but the configured remote
  still contains the branch, **WHEN** attach-only preflight runs, **THEN** it
  returns an ordinary reuse failure and does not advertise branch replacement.
- **GIVEN** attach-only preflight cannot authenticate to or reach the remote,
  **WHEN** the branch refs are absent locally, **THEN** it returns the probe
  failure and does not classify the branch as unrecoverable.
- **GIVEN** replacement checkout creation succeeds but the task-environment
  update fails, **WHEN** recovery returns, **THEN** no replacement checkout or
  branch remains and the prior environment record is unchanged.
- **GIVEN** replacement materializes before provider startup or readiness fails,
  **WHEN** the resume attempt reaches a terminal path, **THEN** one warning is
  persisted or remains retryable without changing the session identity.

## Out of scope

- Silently falling back to a fresh provider-native conversation after Resume.
- Reconstructing or rewriting provider-owned conversation history.
- Automatically mutating npm caches on every normal agent launch.
- Global npm cache cleanup, exact runtime-version pins, rollback, or selection
  of historical runtime versions.
- Relaxing credential-broker authorization for terminal sessions.
- Reconstructing commits, uncommitted files, or other code from a branch that
  no longer exists locally or on the configured remote.
- Automatic continuation on a new branch after branch loss.
- Branch replacement for an authentication, network, timeout, or other
  unconfirmed Git failure.
- New navigation, a new recovery page, or a new mobile-only recovery workflow.
