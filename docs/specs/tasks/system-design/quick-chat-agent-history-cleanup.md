---
status: current
system: tasks
requirements:
  - REQ-TASKS-QUICK-CHAT-AGENT-HISTORY-001
  - REQ-TASKS-QUICK-CHAT-AGENT-HISTORY-002
created: 2026-10-14
owners:
  - kandev
---

# Quick Chat Agent History Cleanup System Design

## Purpose and boundaries

The task service's deletion pipeline already removes a Quick Chat's sessions, executor rows, and
scratch directory (`performTaskCleanup` → `cleanupQuickChatDirs` in
`internal/task/service/service_tasks.go`). Manual deletion (`DeleteTask`) and the expiration
sweep (`runQuickChatExpiration` → `deleteExpiredQuickChatTask`) both reach that pipeline, so one
hook there serves both paths. This design adds an agent-history step to that pipeline. It does
not change the deletion API, events, UI, the expiration predicate, or any agent launch path.

The agent CLIs own their history formats. Kandev deletes only files it can prove belong to the
deleted session, and never edits agent-owned shared indexes.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-QUICK-CHAT-AGENT-HISTORY-001` | [Control flow](#control-flow), [Path derivation](#path-derivation), [Failure and recovery](#failure-and-recovery) |
| `REQ-TASKS-QUICK-CHAT-AGENT-HISTORY-002` | [Eligibility](#eligibility), [Path derivation](#path-derivation), [Security](#security) |

## Components and responsibilities

- **`agenthistory` package** (`internal/task/service/agenthistory/`): pure path derivation and
  guarded removal for each supported agent. No database or service dependencies; the home
  directory and environment arrive in the identity.
  - `Identity{AgentName, ResumeToken, WorkDir, Env map[string]string, HomeDir}`.
  - `Remove(ctx, Identity) error` dispatches on `AgentName` to the Claude or Codex remover and is
    a no-op for any other agent.
- **Task service** (`quick_chat_agent_history.go`) captures the history identity for each Quick
  Chat session when deletion is admitted, before executor rows are deleted, and calls the remover
  after `cleanupQuickChatDirs`.

## Data and contracts

Current identity is read from the executor row. Accepted local provider identities are also
retained under `task_sessions.metadata.quick_chat_agent_history` before a later rotation can
discard them. Each record carries the provider name, native resume token, working directory,
and runtime. No schema change is required.

Retention shares the existing executor upsert or resume-token transaction. The token CAS and
its history write commit together, so a rejected stale execution cannot append an identity.
The existing session-turn write lock serializes writers on PostgreSQL; SQLite serializes its
writers. The metadata update changes only the history key, preserving unrelated session data.
History bookkeeping preserves the session activity timestamp, so retaining an identity during
runtime persistence does not make an idle Quick Chat ineligible for expiration.
Repeated identities are deduplicated. Only eligible Quick Chats and standalone runtimes
produce host-history records.

| Field | Source |
| --- | --- |
| `AgentName` | `task_sessions.agent_profile_snapshot.agent_name` (`claude-acp`, `codex-acp`, `codex-app-server`) |
| `ResumeToken` | `executors_running.resume_token` for the session |
| `WorkDir` | `executors_running.worktree_path` (the standalone launch's workspace path, which is the agent cwd), else `<kandev home>/quick-chat/<session id>` |
| host-local | `executors_running.runtime == standalone` (local and worktree executors) |
| `Env`, `HomeDir` | the backend process environment (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`) and home directory, which host-local agent processes inherit |

Deletion captures retained records even when the operational executor row is absent or its
current token is empty, and includes any remaining current local identity. Non-local records
are excluded independently. The captured identities are persisted in the durable task-resource cleanup job snapshot as
`agent_history` (additive, absent-tolerant), so a cleanup retried after a restart still has them.

## Eligibility

History cleanup runs for a session only when all hold:

1. The task is a Quick Chat task (`is_ephemeral`, empty `workflow_id`, origin is not
   `automation_run`). Managed plugin conversations keep their own lifecycle and are excluded.
2. The session's runtime stop did not fail. A confirmed-dead, resume-safe executor row may
   remain preserved for row/worktree lifecycle reasons without preserving deleted-chat history.
3. The executor runtime is host-local (`standalone`).
4. `AgentName` is a supported agent and `ResumeToken` is a single safe path segment (no
   separators, not `.`/`..`, non-empty).

## Path derivation

Config-dir overrides are literal paths. A nonempty override that is not absolute is skipped
because the backend cannot recover the child process's path resolution safely; it never falls
back to a different default store. Whitespace in an absolute override remains part of the path.

**Claude Code.** `root = $CLAUDE_CONFIG_DIR or $HOME/.claude`. `encoded = WorkDir` with every
rune outside `[A-Za-z0-9]` replaced by `-` (matches observed directory names such as
`-home-alex--kandev-quick-chat-<uuid>`). Remove:

- `root/projects/<encoded>/<token>.jsonl` (regular file only),
- `root/projects/<encoded>/<token>/` (directory only),
- then `root/projects/<encoded>/` if it contains nothing, or only an empty `memory/` directory.

Claude truncates and hashes encoded names longer than its path limit. When `encoded` exceeds 200
characters the remover skips the session rather than guessing the hashed name.

**Codex.** `root = $CODEX_HOME or $HOME/.codex`. Traverse three real directory levels below
`root/sessions/` and select `rollout-*-<token>.jsonl` names. The configured root is a literal
path, including bracket characters. For each match, read only the first
line and require `type == "session_meta"`, `payload.id == token`, and `payload.cwd == WorkDir`
before removing it. Codex-owned `state_5.sqlite` and `session_index.jsonl` are not modified.

## Control flow

Identities are captured wherever a deleting cleanup job snapshots its inventory, so every
delete route is covered:

```text
Quick Chat tab close -> HandoffService.DeleteTaskTree
  -> PrepareTaskResourceCleanupWithOptions (cascade_delete)
       snapshot.agent_history = capture(task, sessions)   (deleting triggers only)
DeleteTask / deleteExpiredQuickChatTask (quick_chat_expire)
  -> deleteTaskWithManagedClaim
       envCleanup.agentHistory = capture(task, sessions)  -> snapshot.agent_history
cleanup job / async cleanup
  -> stop agents -> performTaskCleanup
       delete executor rows, cleanupQuickChatDirs
       agenthistory.Remove(identity) for each captured target
```

History removal uses the failed-stop set, separately from the union used to preserve executor
rows and worktrees. Agents are stopped before cleanup, so the CLI cannot rewrite the transcript after removal.
Archive triggers never capture identities.

## Failure and recovery

- Missing root, missing files, or no resume token: no-op, no error.
- Removal or read error: logged at Warn with task/session ids and path, not added to the
  deletion error list, so the existing deletion outcome is unchanged (AC-001.5).
- Context cancellation stops further removals, matching the surrounding cleanup.
- Codex index rows: verified with codex-cli 0.162.1 on 2026-10-14. After its rollout is removed,
  a thread is omitted from `thread/list` (the source of resume pickers), and an explicit resume
  fails with "failed to resolve rollout path ... file does not exist". The Codex-owned
  `state_5.sqlite` row (including the thread's preview and first user message) and any
  `session_index.jsonl` name entry remain; Kandev does not edit them.

## Persistence

No database schema change. Session metadata retains additive, absent-tolerant history records,
and the cleanup job snapshot carries the additive `agent_history` field. The records die with
the session row; archive keeps them available for later deletion. Tokens lost before this
retention mechanism existed cannot be reconstructed by scanning unrelated history.
Removal is best-effort and idempotent; a failed removal is logged and leaves the file.

## Security

- The configured history root is resolved once and opened through pinned `os.Root` handles.
  Each project and date component is checked with `Lstat`, opened relative to its pinned
  parent, and compared with its original identity. Symlinked components and transcripts are
  skipped. Reads and removals stay relative to retained handles when a lexical ancestor is
  renamed or replaced; a leaf identity is checked again before removal.
- Claude's empty project directory is pruned only after a transcript or sidecar belonging
  to the selected session was removed. A missing session cannot prune an unrelated empty
  project whose working directory encodes to the same name.
- Only exact names derived from the session's identity are removed; no prefix or substring
  matching on directory names.
- Non-local executors never touch the host filesystem.

## Observability

Debug log per removed path; Warn on failures. No new metrics.

## Related decisions

- [ADR 0038: Quick Chat Repository Isolation](../../../decisions/0038-quick-chat-repository-isolation.md)
