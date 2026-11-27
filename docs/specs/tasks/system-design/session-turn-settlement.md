---
status: draft
system: tasks
requirements:
  - REQ-TASKS-SESSION-TURN-SETTLEMENT-001
  - REQ-TASKS-SESSION-TURN-SETTLEMENT-002
---

# Session turn settlement design

## Ownership and requirement mapping

Tasks owns session execution lifecycle. The lifecycle manager implements this
contract under `apps/backend/internal/agent/runtime/lifecycle/`.

| Requirement | Design boundary |
| --- | --- |
| REQ-TASKS-SESSION-TURN-SETTLEMENT-001 | Callback lease, completion flow, and regression coverage below |
| REQ-TASKS-SESSION-TURN-SETTLEMENT-002 | [Agent-started turn finalization](#agent-started-turn-finalization) and [MCP startup reports](#mcp-startup-reports) |

## Callback lease

`AgentExecution.withStartupAttempt` holds `startupCallbackMu.RLock` across
generation validation, callback mutation, and event publication. It captures
the attempt ID while that lease protects the startup identity.

`BindResumeAttempt` takes the exclusive lock before the execution-store lock.
Startup replacement also takes the exclusive lock. Retain this ordering and
the outer lease throughout completion processing.

Code inside a leased callback must use helpers that do not acquire another
startup callback lease. A second read lock can deadlock when an exclusive
writer waits for the first read lock. A generation check without a lease does
not protect subsequent mutation or publication.

## Completion flow

`handleAgentEventWithStartupGeneration`, `handleAgentEvent`, and
`handleAgentEventAfterContextReset` acquire the outer lease. Direct completion
callers use `handleCompleteEvent`, which also acquires it. These entry points
pass the captured attempt ID into `handleCompleteEventLeased`.

`claimPromptCompletion` retains its current admission rules. For an eligible
generation-zero success, `finishPromptCompletion` signals the completion
channel before `handleCompleteEventMarkState` publishes readiness.

The state helper calls `markReadyEventForExecution` with the leased execution,
`events.AgentReady`, synchronous publication, and `event.AttemptID`. It must
not call the public `MarkReady` wrapper from inside the lease. Document that
the internal state helper requires a held lease; direct tests must establish
that precondition or enter through `handleCompleteEvent`.

The existing Ready-to-Running fallback for an empty synthetic turn remains.
It publishes Running before Ready. The readiness helper retains status
persistence and captures the event payload before publication.

Numbered completions retain their prompt-generation claim and captured payload.
Errors retain `markCompletedWithTurnIDAndAttempt`; foreground-idle and stream
disconnect paths retain their leased completion-signal helper.

## Compatibility and failure behavior

The callback lease applies to all providers. It rejects callbacks from obsolete
startup generations before state changes. The `dispatchedPromptPending` guard
still governs unnumbered completions.

Ordinary public `MarkReady` and `MarkBootReady` callers retain their lease
acquisition. Ordinary turn-end publication remains synchronous, as specified
by [ADR 0035](../../../decisions/0035-version-agent-ready-events-by-prompt-generation.md).

The correction prevents new deadlocks at this boundary. It does not release
locks in an already-deadlocked backend. Existing workspace-refresh latency and
idle-reaper policy remain separate concerns.

## Verification boundary

Exercise the actual dispatcher and completion helper with a pending
`BindResumeAttempt` writer. Prove writer contention before releasing the
completion barrier; elapsed sleep alone does not establish contention.

Use a child test process with a bounded timeout for the deadlock regression.
This allows the parent test to terminate the faulty process without leaking
blocked goroutines. On success, join all goroutines before reading shared
state. Assert Ready publication, originating attempt identity, writer progress,
and subsequent prompt admission.

Cover both Running and already-Ready synthetic completions. Retain stale
startup, stale prompt, pending-dispatch, error, and foreground-idle cases.

## Agent-started turn finalization

`acp/adapter_async_complete.go` owns the idle finalizer for ACP updates that
arrive with no prompt turn. It synthesizes the completion that
`REQ-TASKS-SESSION-TURN-SETTLEMENT-001` then settles.

`asyncTurnFinalizer.activeTools` holds only tools with evidence of foreground
activity. A tool enters it only from an update with a non-empty status. A
status-less update can reach the finalizer for a tool the adapter has already
retired, because Claude's PostToolUse hook publishes a metadata-only
`tool_call_update`. That update has no later terminal frame, so tracking it
would suspend the idle timer permanently. It neither enters `activeTools` nor
creates a finalizer. It also does not count as turn content, so it cannot
synthesize a completion for a turn that never started.

A `usage_update` whose `_claude/origin` identifies a human or task-notification
turn is the provider's end-of-turn boundary. When no prompt turn owns the
session, that boundary clears `activeTools` and arms the normal idle timer. The
timer stays because trailing frames can follow the boundary. Silence alone still
does not end a turn with an active foreground tool.

### Backend turn start

The lifecycle manager's `recordActivity` (`manager_events.go`) flips a Ready
execution back to Running when an agent-started turn begins. It must use the
same evidence as the finalizer: a `tool_update` with an empty `ToolStatus`
does not flip the execution. Otherwise the backend marks the session busy for a
turn the finalizer will never close, because the finalizer ignores that update.

The adapter can synthesize `in_progress` for a status-less update that carries
a title, input, content, or locations. The stream carries `ToolStatusMissing`
so both consumers can recognize these updates. A status-less update can
refresh activity timestamps while the execution stays Ready.

### MCP startup reports

Codex ACP can report MCP startup failures after the session RPC succeeds. The
reporter sends a failed `tool_call` with kind `other`, a reserved `mcp_startup.`
ID, and a title of the form `mcp__<server>__startup`.
[Codex ACP 2.8.0 reporter](https://github.com/agentclientprotocol/codex-acp/blob/ad09e0ead18511a65a0761cac832aa3087219b10/src/tool-calls/reporters/McpStartupReporter.ts#L29-L38).

The ACP adapter recognizes that shape for Codex ACP and emits a
`mcp_startup_warning` with the report ID and sanitized provider text. It does
this before tracking tool or prompt activity. The lifecycle callback reuses
the same check for old agentctl events and publishes the warning before its
activity handler. The warning follows existing ownership and delivery checks
as session metadata.

The orchestrator uses the existing idempotent session message writer to store
one status warning for the report in an already-completed `lifecycle_only`
turn. The shared chat renders the warning. Boot readiness and real turn
completion drive queued prompt delivery. [Resume prompt queue design](resume-prompt-queue.md).

## Implementation plans

- [Completion callback lease repair](../../../plans/completion-callback-lease/plan.md).
- [Stuck turn and queue delivery repair](../../../plans/stuck-turn-queue-delivery/plan.md).
- [Send Now claim loss repair](../../../plans/send-now-claim-loss/plan.md).
- [Codex ACP MCP startup warning repair](../../../plans/codex-acp-mcp-startup-warning/plan.md).
