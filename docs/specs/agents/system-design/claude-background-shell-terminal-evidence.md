---
status: current
system: agents
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-010
---

# Claude background shell termination evidence

## Ownership and scope

Agents owns provider workload identity and termination evidence. This design
extends the [shared background-work design](background-work.md). Task persistence
and desktop/phone selectors consume its existing normalized observations.

| Requirement | Design boundary |
| --- | --- |
| REQ-AGENTS-BACKGROUND-WORK-002 | Exact shell identity, duplicate handling, retained history |
| REQ-AGENTS-BACKGROUND-WORK-005 | Shared running count and History selection |
| REQ-AGENTS-BACKGROUND-WORK-010 | Claude terminal evidence and conservative fallback |

## Verified provider behavior

Official `@agentclientprotocol/claude-agent-acp` 0.97.0 retains a display terminal
after a Bash command detaches. Its `withRunningTerminal` helper suppresses the
launch result's terminal exit. Its `endBackgroundTerminal` sends a separate
`tool_call_update` when the command's task ends. This update names the original
`toolCallId` and repeats that ID as `_meta.terminal_exit.terminal_id`.
Both `exit_code` and `signal` can be null. Null proves no outcome. A non-null
signal proves the command was terminated by that signal and still carries no
exit code.

The inspected 0.81.2 bridge emits terminal-exit metadata with the launch result
and has no equivalent retained-terminal path. The same metadata name therefore
does not establish detached-command termination across all bridge versions.

The [rejected async-task proposal](../../../decisions/2026-11-01-claude-async-task-lifecycle-capability.md)
remains rejected. This design consumes existing `tool_call_update` metadata.
It does not advertise `asyncTasks`, decode unknown SDK union variants, or add
task-notification turns. The pinned Go SDK already preserves this metadata.

## Recognition boundary

Keep recognition in a Claude dialect helper beside
`dialect_claude_prompt_suggestions.go`. The adapter supplies immutable connected
agent information, the current tool update, and its tracked shell payload.

For null-code termination, require all of these conditions:

1. The configured provider uses the Claude dialect.
2. Initialize identifies the exact inspected official bridge name and version
   `0.97.0`. An unverified version retains the existing fallback.
3. The payload belongs to an already recognized detached shell in this session.
4. The terminal envelope is an object with a nonempty `terminal_id` equal to the
   current `toolCallId`. The bridge uses this same ID for its display terminal.
5. The envelope has a null `exit_code` and a valid `signal` field: null or a
   nonempty string. Invalid non-null exit-code types or malformed signal values
   do not qualify as unknown-outcome evidence.

The verified version also handles a fast command whose terminal exit accompanies
the launch result: that bridge suppresses the exit while the command remains live.
Keep the existing numeric-exit path for other providers and older bridges.
Missing agent information, foreign providers, mismatched IDs, malformed envelopes,
and unbound tools establish no new termination evidence. Do not settle the
oldest row or infer completion from silence or output text.

Version qualification uses existing `Adapter.agentInfo`. It adds no operator
configuration or feature flag. Extend compatibility only with another verified
provider fixture. Model names, including Opus, do not participate.

## Adapter and durable projection

`convertToolCallResultUpdateForGeneration` applies recognized evidence under its
existing tool-state lock. It marks `BackgroundWorkPayload.Ended`, retires the
matching active tool, and emits a terminal tool update for existing accounting.
The tool invocation can be complete while its workload outcome remains unknown.
For the verified bridge, a detached-shell binding remains tracked after the
foreground prompt ends, so a later tool-scoped terminal frame can still settle
the exact job. The typed terminal observation also retires the session's
execution-scoped background-accounting registration for that tool call; a live
sibling or successor execution is unaffected. Unverified providers keep their
prompt-end settlement path.

Attach an existing `streams.WorkloadRunObservation` through
`AgentEvent.BackgroundWork` for the same workload and source call. A null code
maps to `RunStateEnded` with `ExitCode=nil`. A reported numeric code retains its
actual success or failure and code. Do not convert null to zero. Preserve the
adapter's existing work/run identity and capability declaration. Use observation
time for `FinishedAt` when the provider supplies no timestamp.

`recordBackgroundWorkObservationFromToolPayload` already prefers this typed
observation. Use that path so `buildObservationFromNormalized` does not map the
unknown outcome to `completed`. Preserve the committed projection's revision and
origin association before publishing the existing session websocket update.
No schema, DTO, public event, or alternate persistence path is necessary.

Duplicate terminal evidence must not recreate active tool state or another
workload. Existing terminal monotonicity rejects delayed nonterminal reopening.
A terminal frame without its session's launch binding cannot settle another
job. No event creates a prompt, changes a successor turn, or emits root completion.

## Existing desktop and phone behavior

The existing shared running selector excludes `ended` rows. The chip decreases
from two jobs to one when the first job ends. It disappears after the second.
History retains both rows with unavailable outcomes and null exit codes.

Reuse the shipped desktop overview/detail tabs and phone full-height overview.
No component, navigation, layout, copy, or localization change is required.
Preserve an open detail, selection, search, and the composer draft during updates.
If the last job leaves an open summary, use existing dismissal and focus behavior.

## Recovery and rollout limits

Already consumed null-exit frames cannot repair stored rows automatically after
deployment. This package adds no transcript scan, raw-log replay, or database
backfill. Existing verified owning-process termination remains the fallback for
historical stale rows and unsupported bridge versions. A reload alone cannot
repair a persisted `running` record.

## Verification

Use sanitized frames from the [repair package](../../../plans/claude-background-terminal-exit/plan.md).
Adapter cases cover delayed null exit, numeric exit, fast completion, legacy
launch exit, foreign/malformed evidence, duplicate exit, and late launch replay.
Use two simultaneous jobs to prove that one exit retires only its matching job.

An integration test carries the adapter event through the existing durable
projection and publication path. It asserts `ended`, no exit code, preserved
identity, and no duplicate or root completion. Desktop and mobile browser tests
use a mock bridge with the verified initialize tuple and captured wire shape.
They assert persisted state before checking the count, History, and draft.
The fixture identity is test-owned and restored after the scenario. Production
code must not bypass the evidence rule for the mock provider.
