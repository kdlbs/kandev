---
status: active
system: platform
created: 2026-10-01
owners:
  - kandev
---

# Windows Background Process Console Requirements

## Overview

On Windows, a console program attaches to the console of the process that
starts it. When that process has no console, Windows creates a new console
with a visible window for the child. Kandev can run without a console, for
example when a test harness starts it as a detached process. Every console
helper that it then starts opens a console window that the user did not ask
for.

Platform owns this contract because the helpers belong to several systems
(managed Git, agentctl process management, agent launch, and ACP utilities)
and share one Windows console behavior.

## Terminology

- **Managed helper process:** A console process that Kandev starts in the
  background and stops together with its owner: a managed Git command,
  agentctl, an agent, script, utility, or code-server editor process started
  by agentctl, a Cursor native MCP command, or the agent process tree of the
  ACP and Codex debug tools.
- **Console window:** A terminal window that Windows shows on the desktop for
  a console process.

## Requirements

### REQ-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001: Start managed helper processes without console windows

**Intent:** Background work must not open console windows on the user's
desktop, regardless of how the Kandev process that starts it was started.

**User story:** As a Windows user, I want Kandev's background Git, agent, and
helper processes to stay off the screen, so that running tasks does not open
console windows over my work.

#### Acceptance criteria

- **AC-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001.1:** When Kandev starts a
  managed helper process on Windows, the system shall start it without a
  console window, even when the Kandev process that starts it has no console.
- **AC-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001.2:** When a caller requests a
  new console for a managed Git command or a Cursor native MCP command, the
  system shall reject the command before it starts.
- **AC-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001.3:** When a managed helper
  process runs without a console window, the system shall still pass its input,
  capture its output, and stop it together with its owner.

## Out of scope

- Interactive terminals that run on a pseudo console.
- Processes that the CLI launcher supervises. They keep the launcher's console.
- Other commands that Kandev starts outside the managed helper lifecycle,
  such as the `gh` and `glab` helpers, external editor launches, and plugin
  processes.
- The desktop shell's own console and how the desktop shell starts the
  launcher.
