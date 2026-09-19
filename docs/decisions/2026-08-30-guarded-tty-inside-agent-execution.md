# ADR-2026-08-30-guarded-tty-inside-agent-execution: Keep Guarded TTY Dispatch Inside the Active Agent Execution

**Status:** accepted
**Date:** 2026-08-30
**Area:** backend, agentctl, protocol, security

## Context

Codex App Server can allocate a TTY through `command/exec`, while ordinary
Codex ACP model command events do not carry a TTY selection. Kandev needs a
model-callable terminal diagnostic without adding a host command runner or
bypassing task confinement.

Some deployments wrap codex-acp in a launch-time bubblewrap guard and disable
Codex `unified_exec`. That guard exposes no per-command authorization API. ACP
permission resolution also answers provider-originated prompts after the fact;
it is not a general pre-dispatch command policy.

## Decision

Kandev exposes guarded TTY execution as a capability-gated built-in MCP tool.
The backend binds the request to the exact live task execution, and agentctl
invokes a versioned ACP extension on that execution's existing codex-acp
connection. Only the codex-acp bridge may call App Server `command/exec`, and it
must derive cwd and sandbox state from the active session and force `tty: true`.

The codex-acp bridge and App Server child must remain inside the already-guarded
agent process tree. The extension cannot spawn a host executor, select another
workspace, accept security configuration from the model, mutate
`CODEX_CONFIG`, or require `unified_exec`.

Kandev records a server-generated attestation ID before dispatch and finalizes
it with the validated provider receipt. Tool advertisement requires both a
backend allow capability and an exact live bridge capability response.

## Consequences

TTY-sensitive commands can run with truthful App Server dispatch evidence while
retaining executor, worktree, mount, credential, Docker, and deployment-guard
boundaries. Unsupported bridges never receive a misleading tool.

The feature requires a coordinated codex-acp release and Kandev pin. The
backend-to-agentctl call is nested inside an in-flight MCP request, so the
full-duplex stream and cancellation paths need explicit race and deadlock
coverage.

Deployment launch policy remains outside public Kandev source. Product tests
must prove coexistence with the documented `unified_exec=false` contract, and
PRs must distinguish deployment configuration from the upstream bridge gap.

## Alternatives Considered

- **Add TTY to ordinary model `commandExecution`.** Rejected because the
  current event carries no TTY choice and Kandev cannot attest a provider schema
  change it does not own.
- **Allocate a PTY in agentctl or its shell API.** Rejected because that is a
  second command executor and does not prove App Server `command/exec` dispatch.
- **Use ACP permission resolution as the command guard.** Rejected because it
  resolves live provider prompts and provides no launch confinement or generic
  pre-dispatch policy.
- **Call App Server from a host support process.** Rejected because the process
  would not be the model-selected guarded execution.
