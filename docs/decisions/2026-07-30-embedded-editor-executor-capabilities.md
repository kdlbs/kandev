# ADR-2026-07-30-embedded-editor-executor-capabilities: Derive Embedded Editor Availability from the Active Executor

**Status:** accepted
**Date:** 2026-07-30
**Area:** backend, frontend, protocol

## Context

Kandev's embedded VS Code integration starts code-server through agentctl inside a task session's
execution environment. The original Windows guard instead used `runtime.hostOS` from the SPA boot
payload, which describes the Kandev control-plane process. That is correct only for host-backed
Local and Worktree executors. It incorrectly disables Linux Docker, Remote Docker, Sprites, and
supported SSH environments whenever the Kandev backend itself runs on Windows.

The task-session status contract already identifies the active session's executor and runtime.
Availability is needed before the user launches an editor, and the launch API must enforce the
same answer so a stale or handcrafted client cannot bypass the UI.

## Decision

Embedded-editor availability is a capability of the active session execution environment, not of
the browser or the Kandev installation as a whole.

The backend is the single authority for this capability. `task.session.status` returns a typed
`capabilities.embedded_vscode` boolean. The frontend consumes that boolean and does not maintain an
executor-type allowlist.

The backend resolver maps executor runtime ownership as follows:

- Standalone Local and Worktree execution uses the Kandev host platform and supports code-server
  on Linux and macOS, but not Windows.
- Docker, Remote Docker, and Sprites execute in supported Linux environments.
- SSH supports code-server while Kandev's SSH runtime accepts only Linux and macOS targets. If SSH
  later accepts Windows, its capability must use persisted or live remote-platform metadata rather
  than remaining unconditionally true by executor type.
- Missing executors, unknown executor types, and unknown runtimes fail closed.

The same resolver is used by task-session status projection and by the editor service. Requests to
open `internal_vscode` are rejected with the existing editor-unavailable error when the resolved
capability is false. Other editors retain their existing enabled, installed, and configuration
rules.

The frontend treats a missing capability field as false, filters the topbar editor candidates
before resolving the saved default, and keys the value to the currently active session. A saved
preference is never rewritten merely because it is incompatible with one session.

The editor-specific `runtime.hostOS` boot field is removed because it has no remaining consumer.
General host diagnostics, if needed later, should use a diagnostic contract rather than influence
task-runtime capabilities.

## Consequences

- Windows users regain embedded VS Code for Linux-backed Docker and other supported remote task
  runtimes while native Windows Local and Worktree sessions remain protected.
- Availability changes when the active session changes, even within one task.
- Capability policy lives beside backend runtime knowledge and can evolve without shipping a
  matching frontend executor list.
- UI filtering and API enforcement cannot drift if both use the shared resolver.
- Until session status arrives, the embedded editor is hidden. Other configured editors remain
  available.
- Adding a new executor or expanding an existing executor to another platform requires an explicit
  capability decision and tests.

## Alternatives Considered

1. **Keep using the Kandev backend host OS.** Rejected because it conflates the control plane with
   Docker, sandbox, and remote execution environments.
2. **Hardcode supported executor names in React.** Rejected because it duplicates backend runtime
   knowledge, cannot safely represent future platform-dependent SSH support, and offers no API
   enforcement.
3. **Expose only executor type and let every client infer support.** Rejected because every client
   would need to reproduce the capability matrix and update in lockstep.
4. **Probe code-server only after the user clicks.** Rejected because unsupported actions remain
   visible, failures arrive late, and saved-default fallback cannot be resolved beforehand.
5. **Replace code-server with Microsoft VS Code Server.** Rejected for this change because it is a
   separate product and licensing/integration decision and is unnecessary for Windows-hosted Linux
   executors.
