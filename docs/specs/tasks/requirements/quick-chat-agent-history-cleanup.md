---
status: active
system: tasks
created: 2026-10-14
owners:
  - kandev
---

# Quick Chat Agent History Cleanup Requirements

## Overview

Quick Chat is disposable. Deleting or expiring one already removes its Kandev task, sessions,
and scratch or worktree materialization through the task deletion path
([Quick Chat Sessions, Persistence, and Expiration](quick-chat-expiration.md)). The agent CLI
that served the chat keeps its own conversation transcript outside Kandev, so the transcript
survives the chat and appears in tools that search every agent conversation.

The task system owns this contract because the cleanup is part of the Quick Chat deletion
lifecycle. The agent CLIs own their history formats; Kandev removes only the files that the
deleted Quick Chat session demonstrably created.

Confirmed persistence behavior (host-local executors, 2026-10-14):

- **Claude Code** (`claude-acp`, ACP and passthrough) writes one transcript per agent session to
  `<claude config dir>/projects/<encoded cwd>/<agent session id>.jsonl`, plus an optional
  sidecar directory `<agent session id>/` (subagent transcripts). The config dir is
  `$CLAUDE_CONFIG_DIR` when set, otherwise `~/.claude`. The encoded cwd replaces every
  non-alphanumeric character of the absolute working directory with `-`. Kandev's stored resume
  token for the session equals the agent session id. A repo-less Quick Chat runs in
  `<kandev home>/quick-chat/<kandev session id>`, so its project directory is unique to that
  Kandev session; a repository-backed Quick Chat runs in its task worktree.
- **Codex** (`codex-acp` and `codex-app-server`) writes one rollout per thread to
  `<codex home>/sessions/YYYY/MM/DD/rollout-<timestamp>-<thread id>.jsonl`, where codex home is
  `$CODEX_HOME` when set, otherwise `~/.codex`. The rollout's first `session_meta` line records
  the thread id and cwd. Kandev's stored resume token equals the thread id. Codex also indexes
  the thread in Codex-owned shared stores (`state_5.sqlite`, `session_index.jsonl`).

## Terminology

- **Quick Chat task:** an ephemeral task that satisfies the restorable utility-chat predicate in
  [Quick Chat Sessions, Persistence, and Expiration](quick-chat-expiration.md), including
  configuration chats.
- **Agent history:** the files an agent CLI writes to its own history store for one agent
  session.
- **History identity:** the Quick Chat session's agent type, recorded agent session id (resume
  token), recorded working directory, and host-local executor.

## Requirements

### REQ-TASKS-QUICK-CHAT-AGENT-HISTORY-001: Remove agent history with its Quick Chat

**Intent:** A deleted Quick Chat removes its agent transcripts while preserving CLI-managed shared stores.

**User story:** As a Kandev user, I want deleting or expiring a Quick Chat to also remove the
agent's transcript of it, so that disposable chats do not accumulate in my agent history.

#### Acceptance criteria

- **AC-TASKS-QUICK-CHAT-AGENT-HISTORY-001.1:** When a user permanently deletes a Quick Chat
  served by Claude Code on a host-local executor, the system shall remove that session's Claude
  transcript and sidecar directory, and shall remove the Claude project directory when it is
  left empty.
- **AC-TASKS-QUICK-CHAT-AGENT-HISTORY-001.2:** When the Quick Chat expiration sweep deletes an
  ordinary Quick Chat, the system shall perform the same agent-history cleanup as manual
  deletion.
- **AC-TASKS-QUICK-CHAT-AGENT-HISTORY-001.3:** When a user deletes or the sweep expires a Quick
  Chat served by Codex on a host-local executor, the system shall remove the rollout file whose
  name and `session_meta` match the session's thread id and working directory.
- **AC-TASKS-QUICK-CHAT-AGENT-HISTORY-001.4:** When the agent history is already absent, the
  config dir is missing, or no resume token was recorded, the system shall complete the Quick
  Chat deletion exactly as before and shall not report an error.
- **AC-TASKS-QUICK-CHAT-AGENT-HISTORY-001.5:** When a history file cannot be removed, the system
  shall log a warning and shall otherwise complete the Quick Chat deletion unchanged.
- **AC-TASKS-QUICK-CHAT-AGENT-HISTORY-001.6:** When a Quick Chat resets context, starts fresh,
  or rotates to another provider conversation, deletion shall remove every retained history
  identity created by that chat, including identities whose resume token was later cleared
  or whose operational executor row was removed.

### REQ-TASKS-QUICK-CHAT-AGENT-HISTORY-002: Never remove unrelated history

**Intent:** Cleanup is scoped by the Quick Chat's own identity, never by broad name matching.

#### Acceptance criteria

- **AC-TASKS-QUICK-CHAT-AGENT-HISTORY-002.1:** When a non-Quick-Chat task is deleted or
  archived, the system shall not remove any agent history.
- **AC-TASKS-QUICK-CHAT-AGENT-HISTORY-002.2:** When a Quick Chat is deleted, the system shall
  remove only paths derived from that session's history identity, and shall leave other
  transcripts in the same or other project directories, other agent sessions, and paths whose
  names merely contain `quick-chat` untouched.
- **AC-TASKS-QUICK-CHAT-AGENT-HISTORY-002.3:** When a derived path is not a regular file or
  directory inside the resolved agent history root, or a Codex rollout's recorded id or cwd does
  not match, the system shall not remove it.
- **AC-TASKS-QUICK-CHAT-AGENT-HISTORY-002.4:** When an execution used a non-local executor
  (Docker, SSH, Sprites, Kubernetes, plugin), the system shall not use that execution's
  identity to remove host agent history. Earlier recorded host-local executions retain
  their own cleanup scope.

## Out of scope

- Cleanup of shared, append-only agent stores that mix sessions: Claude `history.jsonl`,
  `file-history/`, `session-env/`, `todos/`; Codex `state_5.sqlite`, `session_index.jsonl`,
  `history.jsonl`. These are agent-owned and edited concurrently by running CLIs.
- Codex history for a passthrough session without a recorded thread id.
- Agent config-dir overrides set only in an agent or executor profile environment; the backend
  process environment is used.
- Agents other than Claude Code and Codex.
- Retroactive cleanup of history left by Quick Chats deleted before this change.
- History on remote executors; those environments are removed by their own executor cleanup.
- Any UI change. The existing delete confirmation and expiry behavior are unchanged.
