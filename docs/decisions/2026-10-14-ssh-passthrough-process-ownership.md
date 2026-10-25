# ADR-2026-10-14-ssh-passthrough-process-ownership: Own the SSH agent PTY in agentctl

**Status:** accepted
**Date:** 2026-10-14
**Area:** backend, protocol

## Context

SSH controllers already run on the target and expose authenticated HTTP and
WebSocket APIs through a local SSH forward. Passthrough currently selects the
backend's standalone interactive runner and requires a standalone MCP port.
The combination therefore fails during bootstrap or executes against the host
filesystem if a matching path exists.

The user selected remote passthrough support. Process ownership must remain
independent of browser lifetime and must prevent duplicate agents during retry.
The selected architecture is an implementation target, not a shipped capability.

## Decision

The per-session remote agentctl process owns the SSH agent's PTY and CLI tree.
Reuse `process.InteractiveRunner` and the existing authenticated controller
transport. The backend owns agent command selection, prompt policy, task state,
and workflow decisions. The browser receives the existing binary terminal protocol.

Controller requests and events retain controller, session, process, and
generation identity. An ambiguous start or write result must be reconciled by
identity before retry. Missing ownership or liveness evidence blocks replacement.

MCP strategies remain pure agent-owned emitters. Filesystem operations execute
beside the CLI. Shared file materialization rules belong in a neutral common
package used by the backend and agentctl. Remote claims record ownership and
fingerprints so separate session controllers cannot remove each other's files.

Standalone passthrough keeps its existing backend-owned runner and survival
exclusion. SSH reconnect and explicit recovery use authenticated remote process
evidence independently of the local agent-survival flag.

## Consequences

The SSH sandbox owns agent work, credentials, and terminal buffers. A browser
reconnect can retain the same remote process. A backend interruption can recover
that process without a second CLI when ownership remains verifiable.

Agentctl must own runner shutdown, bounded event retention, and file claims.
Lifecycle must route every passthrough operation through the selected transport,
including automatic prompts, model changes, reset, Stop, and fallback.

The capability requires compatible uploaded helpers. Older retained helpers fail
with an explicit capability error. They never trigger a local fallback.

## Alternatives Considered

- **Launch a host `ssh -t` process:** This introduces another SSH authentication
  and process lifetime path. It bypasses the existing controller and its
  credential policy. Host MCP files and addresses would still need correction.
- **Run the CLI through an ordinary remote shell:** The shell endpoint does not
  provide agent readiness, process identity, and turn callbacks. Sending a shell
  command also loses structured argv and creates quoting and duplicate-start risks.
- **Patch only the MCP port:** This removes the first failure but leaves host
  CLI execution and host configuration paths unchanged.
- **Reject SSH passthrough:** This is smaller, but does not provide the remote
  sandbox behavior selected by the user.

## References

- [SSH passthrough requirements](../specs/executors/requirements/ssh-passthrough.md)
- [SSH passthrough design](../specs/executors/system-design/ssh-passthrough.md)
- [Agent-owned MCP strategies](0014-passthrough-mcp-injection-strategies.md)
- [Initial prompt ownership](2026-09-01-passthrough-initial-prompt-turn-boundary.md)
