---
status: active
system: executors
created: 2026-10-14
owners:
  - kandev
---

# SSH passthrough requirements

## Overview

An SSH task must run its selected agent on the SSH target. CLI passthrough
provides direct terminal interaction with that remote agent. A remote workspace
must not require a matching directory or agent installation on the Kandev host.

This capability extends [SSH execution](ssh-executor.md). Executors owns process
locality, controller ownership, and transport recovery. The task system retains
ownership of prompt dispatch and turn completion.

## Terms

- **Remote controller:** The authenticated agentctl instance for one SSH task session.
- **Remote process:** The CLI and its PTY owned by that controller.
- **Reconnect:** Attach terminal I/O to an existing remote process.
- **Resume:** Reuse a verified remote process, or start a process in the preserved
  workspace with the agent's existing conversation-resume behavior.

## Requirements

### REQ-EXECUTORS-SSH-PASSTHROUGH-001: Terminal interaction with a remote CLI

As a user, I want SSH passthrough to run inside my remote sandbox so I can
interact with the CLI without executing agent work on the Kandev host.

#### Acceptance criteria

- **AC-EXECUTORS-SSH-PASSTHROUGH-001.1:** When an SSH session selects CLI
  passthrough, its CLI shall run on the SSH target in that task's workspace.
  Startup shall work with no custom prepare script. A matching host directory
  shall never change the execution location.
- **AC-EXECUTORS-SSH-PASSTHROUGH-001.2:** The remote process shall receive the
  selected profile's existing passthrough options and resolved environment.
  Credential transfer shall follow the selected SSH credential policy.
  The system shall not require the CLI on the Kandev host.
- **AC-EXECUTORS-SSH-PASSTHROUGH-001.3:** The agent terminal shall show remote
  output and send input, resize, and interrupt actions to that remote process.
  Successful input submission shall retain the ordinary running-state behavior.
- **AC-EXECUTORS-SSH-PASSTHROUGH-001.4:** Browser disconnect and reconnect shall
  preserve the remote process. Reconnect shall restore bounded recent output
  without replaying input or starting another CLI.
- **AC-EXECUTORS-SSH-PASSTHROUGH-001.5:** Kandev and profile MCP configuration
  shall use paths and addresses reachable from the remote CLI. Kandev tool calls
  shall reach the backend with the current task and session identity.
  The system shall not modify global agent configuration. Shared project-file
  strategies shall reject a conflicting session context before changing the file.
- **AC-EXECUTORS-SSH-PASSTHROUGH-001.6:** Automatic initial-prompt submission
  shall retain the existing per-agent readiness and submit behavior. Startup
  idle shall not complete work while the initial prompt is pending.
  Transport retries shall not submit that prompt twice.
- **AC-EXECUTORS-SSH-PASSTHROUGH-001.7:** Concurrent startup, reconnect, and
  resume requests shall admit at most one live CLI per SSH session. Sibling
  sessions shall retain independent process ownership and terminal I/O.
- **AC-EXECUTORS-SSH-PASSTHROUGH-001.8:** Events and input requests from a
  replaced process or execution shall not change, complete, or control its
  successor. Duplicate completion events shall not repeat workflow effects.
- **AC-EXECUTORS-SSH-PASSTHROUGH-001.9:** Stop and terminal task cleanup shall
  stop the owned remote CLI and its descendants. Ordinary Stop shall preserve
  the workspace and provider conversation for later resume.
- **AC-EXECUTORS-SSH-PASSTHROUGH-001.10:** After a backend interruption, explicit
  Resume shall reconnect to a verified retained remote process when one exists.
  When the prior process is confirmed absent, Resume shall use the preserved
  workspace and existing provider-resume behavior. Local agent-survival settings
  shall not enable or disable this SSH recovery behavior.
- **AC-EXECUTORS-SSH-PASSTHROUGH-001.11:** Missing authentication, mismatched
  ownership, or unknown remote liveness shall block replacement and input.
  The system shall preserve evidence for retry and shall not bypass authentication.
- **AC-EXECUTORS-SSH-PASSTHROUGH-001.12:** When the remote helper lacks this
  capability or the CLI cannot start, the existing recovery surface shall report
  a safe cause and an actionable detail. No failure shall start a local CLI or
  silently change the session to ACP mode.
- **AC-EXECUTORS-SSH-PASSTHROUGH-001.13:** Desktop and phone shall provide the
  same remote terminal, reconnect, interrupt, and resume outcomes through their
  existing task surfaces. Phone input shall use the existing keybar and touch
  behavior. Terminal content shall not cause document horizontal overflow.
- **AC-EXECUTORS-SSH-PASSTHROUGH-001.14:** Configuration materialization and
  cleanup shall preserve user files and unrelated session files. Symlinks shall
  not redirect configuration writes outside the permitted remote roots.
  Cleanup shall preserve modified files and files still claimed by another session.

## Related contracts

- [Passthrough initial prompt boundary](../../tasks/requirements/passthrough-initial-prompt-turn-boundary.md)
  owns whether a readiness signal can complete a turn.
- [Passthrough queued prompt dispatch](../../tasks/requirements/passthrough-queued-prompt-dispatch.md)
  owns queued prompts and their session identity.
- [SSH transport liveness](ssh-transport-liveness.md) owns transport loss detection.
- [Standalone survival gating](agent-survival-session-state.md) applies to
  standalone recovery. Remote passthrough recovery is defined here.

## Exclusions

New passthrough support for Docker, Sprites, Kubernetes, or plugin executors is
outside this capability. It does not introduce reverse tunnels for host-only
MCP servers, automatic CLI installation, or a new terminal layout.

Passthrough does not gain ACP delivery-journal guarantees or durable automatic
prompt replay. An uncertain prompt write requires user recovery.

## System design

See [SSH passthrough design](../system-design/ssh-passthrough.md).
