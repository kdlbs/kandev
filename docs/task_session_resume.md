# Task Session Resume

How sessions are recovered and resumed after a backend restart or agent failure.

---

## Overview

When the backend restarts, running agent processes are lost but all session state is persisted in SQLite (`TaskSession`, `ExecutorRunning`). The system uses a **lazy recovery** pattern: sessions are not immediately restarted. Instead, when a user navigates to a session, the frontend detects `NeedsResume` and triggers the resume flow, which relaunches agentctl + the agent process.

Two resume strategies exist depending on agent capabilities:

| Strategy | When used | What happens |
|----------|-----------|--------------|
| **Native resume** (ACP `session/load`) | Agent has `NativeSessionResume: true` and a stored resume token | Agent restores its own conversation context via ACP protocol |
| **History injection** (fresh start) | Agent has `HistoryContextInjection: true`, no resume token | Agent starts fresh; conversation history is injected into the first user prompt |

Agents with neither flag simply boot idle with no context restoration.

---

## Resume Flow

```
Frontend                    Backend (Orchestrator)              Lifecycle Manager
   |                              |                                   |
   | task.session.status          |                                   |
   |----------------------------->|                                   |
   |   { needs_resume: true }     |                                   |
   |<-----------------------------|                                   |
   |                              |                                   |
   | task.session.resume          |                                   |
   |----------------------------->| ResumeTaskSession()               |
   |                              |   executor.ResumeSession()        |
   |                              |------> buildResumeRequest()       |
   |                              |------> LaunchAgent() (agentctl)   |
   |                              |------> persistResumeState()       |
   |                              |------> startAgentProcessOnResume()|
   |                              |          |                        |
   |                              |          | (async goroutine)      |
   |                              |          |----------------------->| StartAgentProcess()
   |                              |          |                        |   configureAndStartAgent()
   |                              |          |                        |   initializeACPSession()
   |                              |          |                        |   dispatchInitialPrompt()
   |                              |          |                        |     -> (see three-way switch)
   |                              |          |                        |
   |                              |          | callback: restore      |
   |                              |          | WAITING_FOR_INPUT      |
   |<--- WS events (state) ------|----------|------------------------|
```

---

## Entry Points

### 1. Frontend auto-resume (primary)

**`hooks/domains/session/use-session-resumption.ts`**

When a user opens a session page, the `useSessionResumption` hook:
1. Sends `task.session.status` to backend
2. If `needs_resume && is_resumable` -> sends `task.session.resume`
3. Uses `hasAttemptedResume` ref to prevent duplicate resume requests

### 2. `GetTaskSessionStatus` (status evaluation)

**`internal/orchestrator/task_operations.go`** ~line 629

Backend method backing `task.session.status`. Evaluates resume eligibility through 4 cases:

| Case | Condition | NeedsResume | Reason |
|------|-----------|:-----------:|--------|
| 1 | Agent is running in memory | false | Already running |
| 2 | Agent not running but was recently seen | false | Stale execution |
| 3 | Resume token exists + active state | true | `agent_not_running` |
| 4 | No token but `ExecutorRunning` exists + active state | true | `agent_not_running_fresh_start` |

Active session states: `WAITING_FOR_INPUT`, `STARTING`, `RUNNING`.
Terminal states (block resume): `FAILED`, `COMPLETED`, `CANCELLED`.

### 3. `ensureSessionRunning` (lazy resume on prompt)

**`internal/orchestrator/task_operations.go`** ~line 548

Fallback path: when `PromptTask` is called but no in-memory execution exists (post-restart), it triggers `ResumeSession()` and polls until `WAITING_FOR_INPUT` is reached (500ms intervals, 90s timeout).

### 4. `handlePromptWithResume` (prompt retry)

**`internal/task/handlers/message_handlers.go`** ~line 393

If a prompt fails with `ErrExecutionNotFound`, the handler calls `ResumeTaskSession()`, waits for ready, and retries the prompt.

---

## Resume Execution (Backend)

### `ResumeTaskSession`

**`internal/orchestrator/task_operations.go`** ~line 402

1. Load session + `ExecutorRunning` from DB
2. Validate worktree paths exist on disk
3. Reject terminal states
4. Call `executor.ResumeSession(ctx, session, startAgent=true)`
5. On failure: mark session + task `FAILED`

### `Executor.ResumeSession`

**`internal/orchestrator/executor/executor_resume.go`** ~line 133

1. **`validateAndLockResume()`** -- acquire per-session mutex, verify no existing execution
2. **`buildResumeRequest()`** -- build `LaunchAgentRequest` from stored config
3. **`LaunchAgent()`** -- create agentctl instance (runtime-level)
4. **`persistResumeState()`** -- update session state to `STARTING` in DB
5. **`startAgentProcessOnResume()`** -- async goroutine to start agent subprocess

### `buildResumeRequest` (prompt decision)

**`internal/orchestrator/executor/executor_resume.go`** ~line 270

This is where the system decides whether to auto-prompt the agent:

```go
// Case A: Native resume -- has resume token
if running.ResumeToken != "" && startAgent {
    req.ACPSessionID = running.ResumeToken  // triggers session/load
    req.TaskDescription = ""                 // don't auto-prompt
}

// Case B: Fresh-start resume -- no token, was WAITING_FOR_INPUT
else if startAgent && session.State == WAITING_FOR_INPUT {
    req.TaskDescription = ""  // don't auto-prompt; boot idle
}

// Case C: Otherwise -- TaskDescription stays populated (initial launch)
```

Clearing `TaskDescription` is critical: it controls whether `dispatchInitialPrompt` sends an automatic prompt or lets the agent boot idle.

### `startAgentProcessOnResume`

**`internal/orchestrator/executor/executor_resume.go`** ~line 488

Runs `StartAgentProcess` in a background goroutine. On success, the callback restores the session to `WAITING_FOR_INPUT` and the task to `REVIEW` (if the session was previously in `WAITING_FOR_INPUT`). This uses a captured `previousState` variable because `persistResumeState` mutates the session state to `STARTING` before the goroutine runs.

---

## Agent Process Initialization

### `StartAgentProcess`

**`internal/agent/lifecycle/manager_startup.go`** ~line 22

1. Wait for agentctl HTTP server ready (60s timeout)
2. Configure agent (command, env, working directory)
3. Start agent subprocess via agentctl
4. Initialize ACP session (`session/new` or `session/load`)
5. Call `dispatchInitialPrompt()`

### ACP session creation: `session/load` vs `session/new`

**`internal/agent/lifecycle/session.go`** ~line 118

```
If agent has NativeSessionResume: true AND existingSessionID is non-empty:
  -> Try session/load (ACP protocol)
  -> On failure (method not found / capability false): fallback to session/new
Otherwise:
  -> session/new
```

### `dispatchInitialPrompt` (three-way switch)

**`internal/agent/lifecycle/session.go`** ~line 338

| Condition | Behavior |
|-----------|----------|
| `taskDescription` is non-empty | Send it as the initial prompt (standard launch) |
| `taskDescription` empty + `shouldInjectResumeContext()` returns true | Set `needsResumeContext = true`, mark agent ready (idle). History injected on first user prompt. |
| Neither | Mark agent ready (idle, no context) |

---

## History Injection

For agents that cannot natively restore their session (no ACP `session/load`), conversation history can be injected into the first user prompt after resume. This is opt-in via the `HistoryContextInjection` flag on `SessionConfig`.

### Recording

**`internal/agent/lifecycle/session_history.go`**

`SessionHistoryManager` stores history as JSONL files at `~/.kandev/sessions/{sessionID}.jsonl`.

Entry types: `user_message`, `agent_message`, `tool_call`, `tool_result`.

Recording happens in:
- `SendPrompt()` -- records user messages (`session.go` ~line 510)
- `handleAgentEvent()` -- records agent messages and tool calls (`manager_events.go`)

### Injection

When `shouldInjectResumeContext()` returns true (checks `HistoryContextInjection` flag + history file exists), the system defers injection:

1. `dispatchInitialPrompt()` sets `execution.needsResumeContext = true`
2. Agent boots idle, session goes to `WAITING_FOR_INPUT`
3. User sends a message
4. `SendPrompt()` calls `buildEffectivePrompt()` (`session.go` ~line 382)
5. `buildEffectivePrompt()` detects `needsResumeContext = true`, calls `GenerateResumeContext()`
6. Sets `resumeContextInjected = true` (one-time injection)

### Resume context format

`GenerateResumeContext()` (`session_history.go` ~line 207) produces:

```
RESUME CONTEXT FOR CONTINUING TASK

=== EXECUTION HISTORY ===
[USER]: <message>
[ASSISTANT]: <message>
[TOOL CALL: tool_name]
[TOOL RESULT: tool_name] <result>

=== CURRENT REQUEST ===
<user's actual prompt>

=== INSTRUCTIONS ===
You are continuing work on the above task...
Do not repeat work that was already completed.
```

Content is truncated: user/agent messages to 2000 chars, tool results to 500 chars.

### Relevant flags on `AgentExecution`

**`internal/agent/lifecycle/types.go`** ~line 67

| Field | Purpose |
|-------|---------|
| `historyEnabled` | Gates both recording and injection. Set from `SessionConfig.HistoryContextInjection`. |
| `needsResumeContext` | Set true when history exists and should be injected on next prompt. |
| `resumeContextInjected` | Set true after context has been injected (prevents double injection). |

---

## Backend Restart Recovery

### Runtime-level recovery (`RecoverInstances`)

**`internal/agent/lifecycle/manager_lifecycle.go`** ~line 21

On `Manager.Start()`, each runtime recovers what it can:

| Runtime | Recovery behavior |
|---------|------------------|
| **Docker** | Lists containers with `kandev.managed=true`, recovers running ones, reconnects agentctl |
| **Standalone** | Returns nil -- processes are transient, restarted on resume |
| **Sprites** | Returns nil -- reconnected via metadata on resume |
| **Remote Docker** | Returns nil |

### Lazy recovery pattern

After a backend restart:
1. Docker containers may be recovered immediately (agentctl still running)
2. Standalone agent processes are NOT recovered at runtime level
3. All session state persists in SQLite (`ExecutorRunning` retains worktree paths, resume tokens)
4. When frontend connects and opens a session, `GetTaskSessionStatus()` returns `NeedsResume: true`
5. Frontend auto-triggers `task.session.resume`
6. `ResumeSession()` creates a new agentctl instance and restarts the agent process
7. Depending on agent capabilities: native resume via token, or fresh start with history injection

---

## Session State Transitions

### Normal resume (success)

```
Before restart:    Session = WAITING_FOR_INPUT    Task = REVIEW

Resume starts:     Session -> STARTING            (persistResumeState)

Agent ready:       Session -> WAITING_FOR_INPUT   (startAgentProcessOnResume callback)
                   Task    -> REVIEW

User sends msg:    Session -> RUNNING             Task -> IN_PROGRESS
Agent completes:   Session -> WAITING_FOR_INPUT   Task -> REVIEW
```

### Resume failure (token invalid)

```
Resume attempted:  Session -> STARTING
Token rejected:    Session -> WAITING_FOR_INPUT   (not FAILED)
                   Task    -> REVIEW
                   ResumeToken -> cleared
                   Status message shown to user
```

The session is NOT marked as failed -- the user can send a new message to start a fresh session.

---

## Agent Resume Capabilities

| Agent | NativeSessionResume | HistoryContextInjection | Notes |
|-------|:---:|:---:|-------|
| Claude Code | - | - | Uses `--resume` CLI flag for its own session restore |
| Codex | yes | - | ACP `session/load` restores context |
| Copilot | yes | - | ACP `session/load` restores context |
| Auggie | - | yes | Fresh start + history injection on first prompt |
| Gemini | - | - | Boots idle, no context restoration |
| Amp | - | - | Uses `ForkSessionCmd` / `ContinueSessionCmd` |
| OpenCode | yes | - | ACP `session/load` restores context |

Agent configs are defined in `internal/agent/agents/`.

---

## `ExecutorRunning` Model

**`internal/task/models/models.go`** ~line 383

Persists runtime state for a session across backend restarts:

| Field | Purpose |
|-------|---------|
| `ResumeToken` | ACP session ID for native resume (`session/load`) |
| `LastMessageUUID` | For `--resume-session-at` flag |
| `ContainerID` | Docker container ID (for recovery) |
| `AgentctlURL` / `AgentctlPort` | agentctl connection info |
| `WorktreeID` / `WorktreePath` / `WorktreeBranch` | Git worktree info (validated on resume) |
| `AgentExecutionID` | Links to in-memory `AgentExecution` |
| `Status` | Current executor status string |

The resume token is stored/updated in `storeResumeToken()` (`internal/orchestrator/event_handlers.go` ~line 45), called from three event handlers:
- `handleACPSessionCreated` -- on initial session creation
- `handleSessionStatusEvent` -- on session status updates
- `handleCompleteStreamEvent` -- on stream completion

If the agent's profile does NOT have `NativeSessionResume`, the token is cleared to prevent stale resume attempts.

---

## Key Files

| File | Contains |
|------|----------|
| `internal/orchestrator/task_operations.go` | `GetTaskSessionStatus`, `ResumeTaskSession`, `ensureSessionRunning` |
| `internal/orchestrator/executor/executor_resume.go` | `ResumeSession`, `buildResumeRequest`, `persistResumeState`, `startAgentProcessOnResume` |
| `internal/agent/lifecycle/session.go` | `dispatchInitialPrompt`, `shouldInjectResumeContext`, `buildEffectivePrompt`, `SendPrompt` |
| `internal/agent/lifecycle/session_history.go` | `SessionHistoryManager`, `GenerateResumeContext`, `HasHistory` |
| `internal/agent/lifecycle/manager_startup.go` | `StartAgentProcess`, `initializeAgentSession` |
| `internal/agent/lifecycle/manager_lifecycle.go` | `RecoverInstances` (runtime-level recovery) |
| `internal/agent/lifecycle/types.go` | `AgentExecution` fields (`historyEnabled`, `needsResumeContext`) |
| `internal/task/models/models.go` | `TaskSession`, `ExecutorRunning`, session state constants |
| `internal/orchestrator/event_handlers.go` | `storeResumeToken` |
| `hooks/domains/session/use-session-resumption.ts` | Frontend resume trigger |
