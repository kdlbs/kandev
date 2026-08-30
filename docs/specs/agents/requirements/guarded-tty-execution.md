---
status: draft
system: agents
created: 2026-08-30
owners:
  - Kandev
---

# Guarded TTY Execution Requirements

## Overview

Some agent diagnostics require a real terminal, but an ordinary Codex ACP
command does not let the model request one. The agent system owns the explicit
model tool, its provider capability contract, and the evidence that connects a
model request to a confined terminal execution.

The capability is deliberately narrower than an interactive terminal. It runs
one bounded command inside the active task execution and leaves the existing
non-TTY command path unchanged.

## Terminology

- **Guarded execution:** The already-attested agent process tree confined to
  its task worktree, mounts, credentials, Docker policy, and executor limits.
- **TTY receipt:** Durable evidence that connects one model tool request to the
  provider dispatch, terminal output, completion, and exit status.
- **Bridge capability:** A versioned provider response proving that the active
  bridge implements the guarded TTY contract.

## Requirements

### REQ-AGENTS-GUARDED-TTY-EXECUTION-001: Explicit bounded TTY tool

**Intent:** A guarded agent can deliberately request terminal semantics for a
bounded diagnostic without receiving an interactive process API.

#### Acceptance criteria

- **AC-AGENTS-GUARDED-TTY-EXECUTION-001.1:** When an active Codex ACP bridge
  positively reports the supported guarded-TTY capability, the ordinary task
  agent shall receive one explicit TTY execution tool in its current MCP tool
  catalog.
- **AC-AGENTS-GUARDED-TTY-EXECUTION-001.2:** When the provider, bridge version,
  task session, or execution does not support the capability, the tool shall
  be absent from the model's catalog.
- **AC-AGENTS-GUARDED-TTY-EXECUTION-001.3:** The tool shall accept only a
  non-empty bounded command argument vector. It shall not accept caller-chosen
  cwd, environment, sandbox policy, permission profile, process identity, TTY
  mode, standard-input writes, resize, attach, or other interactive controls.
- **AC-AGENTS-GUARDED-TTY-EXECUTION-001.4:** A successful request shall run in
  the active task worktree with both standard input and standard output attached
  to a terminal, return bounded output, complete exactly once, and report the
  provider exit status.
- **AC-AGENTS-GUARDED-TTY-EXECUTION-001.5:** A bounded command containing
  `test -t 0`, `test -t 1`, `stty`, `pwd`, and `git status` shall return terminal
  and repository output from the assigned worktree with exit status zero.
- **AC-AGENTS-GUARDED-TTY-EXECUTION-001.6:** Timeout, cancellation, output
  overflow, disconnect, provider failure, or duplicate completion shall
  terminate with one stable result and shall not leave a reusable process
  handle.

### REQ-AGENTS-GUARDED-TTY-EXECUTION-002: Confinement and durable evidence

**Intent:** Operators can prove that each model-selected TTY command stayed
inside the same security boundary as the active agent and did not weaken
ordinary command delivery.

#### Acceptance criteria

- **AC-AGENTS-GUARDED-TTY-EXECUTION-002.1:** Before provider dispatch, Kandev
  shall bind the request to server-derived execution, task, task-session,
  workspace, owner, and current-agent identities. Caller-supplied identities
  shall not widen that scope.
- **AC-AGENTS-GUARDED-TTY-EXECUTION-002.2:** TTY dispatch shall occur only
  inside the already-guarded provider process tree and shall preserve its
  worktree, sandbox, mounts, attestation, Docker restrictions, and credential
  isolation.
- **AC-AGENTS-GUARDED-TTY-EXECUTION-002.3:** Missing capability, stale or
  replaced execution, cross-task or cross-workspace identity, malformed
  receipt, or confinement mismatch shall fail closed before command dispatch.
- **AC-AGENTS-GUARDED-TTY-EXECUTION-002.4:** Kandev shall durably record one
  server-generated attestation ID that binds the model command arguments to the
  exact provider method, requested and dispatched TTY state, provider process
  identity, trusted cwd, bounded output evidence, timestamps, completion, and
  exit or denial status.
- **AC-AGENTS-GUARDED-TTY-EXECUTION-002.5:** Audit records and logs shall not
  contain credentials, environment contents, unrestricted output copies, or
  caller-supplied sandbox configuration.
- **AC-AGENTS-GUARDED-TTY-EXECUTION-002.6:** Existing non-TTY Codex ACP command
  output and exit delivery shall remain prompt while TTY requests are supported
  or active.
- **AC-AGENTS-GUARDED-TTY-EXECUTION-002.7:** A deployment that disables Codex
  `unified_exec` at guarded process launch shall retain that configuration and
  all other launch settings; the TTY path shall not require or re-enable it.

## Out of scope

- Interactive shells, REPLs, password prompts, standard-input forwarding,
  terminal resize, attach, or long-lived terminal process management.
- Host-side execution or a second command executor outside the guarded agent
  process tree.
- TTY support for providers other than Codex ACP.
- Replacing Kandev's executor, credential, Docker, workspace, or permission
  systems.
