---
status: draft
system: agents
created: 2026-09-24
owners:
  - kandev
---

# Cursor MCP OAuth bridge requirements

## Overview

Cursor stores MCP OAuth credentials per project. A new task worktree therefore lacks credentials that exist in another local project.
The agent system owns this capability because it owns profile preferences and provider launch behavior.

## Requirements

### REQ-AGENTS-CURSOR-AUTH-001: Local credential sharing

**Intent:** Local Cursor sessions inherit MCP credentials from existing projects.

#### Acceptance criteria

- **AC-AGENTS-CURSOR-AUTH-001.1:** With sharing enabled, a new local Cursor execution shall inherit credentials from eligible non-task projects. Different server identities shall coexist.
- **AC-AGENTS-CURSOR-AUTH-001.2:** For duplicate server identities, the most recently modified source auth file shall win. Equal timestamps shall use ascending project-path order, with the first path winning.
- **AC-AGENTS-CURSOR-AUTH-001.3:** Both Cursor ACP and terminal profiles with the Cursor MCP strategy shall support sharing. Sharing shall work without profile-configured MCP servers.
- **AC-AGENTS-CURSOR-AUTH-001.4:** A workspace and its canonical filesystem path shall resolve to the same Cursor project identity.
- **AC-AGENTS-CURSOR-AUTH-001.5:** Absent Cursor data or absent eligible auth files shall produce a silent no-op. Other agents and remote executions shall not receive host credentials.
- **AC-AGENTS-CURSOR-AUTH-001.6:** Each eligible launch shall refresh the shared snapshot from current source files. Kandev shall preserve each selected server object without interpretation.

### REQ-AGENTS-CURSOR-AUTH-002: Profile control

**Intent:** Users can choose whether each Cursor profile shares local credentials.

#### Acceptance criteria

- **AC-AGENTS-CURSOR-AUTH-002.1:** Existing and new Cursor profiles shall default to enabled. An explicit disabled value shall survive save, reload, restart, duplication, and unrelated profile edits.
- **AC-AGENTS-CURSOR-AUTH-002.2:** Applicable profile settings shall expose a labeled checkbox. The description shall explain local cross-project sharing and next-launch timing.
- **AC-AGENTS-CURSOR-AUTH-002.3:** On the next disabled launch, Kandev shall remove only that workspace's link to its unified auth file. It shall preserve other links and regular files.
- **AC-AGENTS-CURSOR-AUTH-002.4:** Disabling shall not remove the shared file or change running sessions. If link removal fails, the new execution shall fail with a credential-free error.
- **AC-AGENTS-CURSOR-AUTH-002.5:** Desktop and phone users shall save, discard, and reload the same value. Keyboard access, visible focus, and localized labels shall remain available.
- **AC-AGENTS-CURSOR-AUTH-002.6:** The phone checkbox label shall provide a touch target of at least 44px. The settings page shall have no horizontal overflow.
- **AC-AGENTS-CURSOR-AUTH-002.7:** Profile APIs and settings discovery shall expose the saved boolean and its default. They shall preserve omission separately from explicit false.

### REQ-AGENTS-CURSOR-AUTH-003: Credential file safety

**Intent:** The bridge preserves user files and keeps credentials out of application data and logs.

#### Acceptance criteria

- **AC-AGENTS-CURSOR-AUTH-003.1:** Task-project auth files and symlinked source entries shall never contribute credentials to the shared snapshot.
- **AC-AGENTS-CURSOR-AUTH-003.2:** A regular destination auth file shall remain byte-for-byte unchanged. Directories and special files shall also remain unchanged.
- **AC-AGENTS-CURSOR-AUTH-003.3:** Updates shall expose complete JSON snapshots and complete symlink targets. Concurrent Kandev launches shall not expose partial writes.
- **AC-AGENTS-CURSOR-AUTH-003.4:** Kandev shall create shared credential files with owner-only read/write permissions. Credentials shall never appear in logs, APIs, events, or browser state.
- **AC-AGENTS-CURSOR-AUTH-003.5:** Invalid or unreadable source files shall not prevent valid independent sources from contributing. Diagnostics shall omit credential contents.
- **AC-AGENTS-CURSOR-AUTH-003.6:** If no valid source remains, Kandev shall not create a new link to an old snapshot. Existing user files shall remain unchanged.
- **AC-AGENTS-CURSOR-AUTH-003.7:** Enabled bridge failures shall preserve existing files and allow normal Cursor startup. Cursor can then request authentication through its existing flow.

## Scope and limits

The checkbox controls bridge preparation, not OAuth revocation or credential isolation between processes that use the same workspace.
A running process can retain credentials already loaded into memory.
Profile choices share a filesystem destination when they use the same canonical workspace.

The user chose newest-file conflict precedence and removal of the bridge link on the next disabled launch.
File modification time does not prove credential freshness or token validity.

## Out of scope

- OAuth login, token validation, token refresh, and server configuration discovery.
- Host credential transfer to containers, SSH, Kubernetes, or other remote runtimes.
- Per-server account selection, background synchronization, and revocation of cached tokens.
- Reading real developer credentials during automated verification.

## Implementation plans

- [Cursor auth bridge plan](../../../plans/cursor-mcp-oauth-bridge/plan.md)
