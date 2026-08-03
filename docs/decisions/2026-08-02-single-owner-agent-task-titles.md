# ADR-2026-08-02-single-owner-agent-task-titles: Assign Agent Task Titles to One Session

**Status:** accepted
**Date:** 2026-08-02
**Area:** backend, frontend, protocol, workflow

## Context

ADR-2026-07-31 introduced a pending marker that lets the first agent replace a prompt-derived
provisional task title. Session prompt and MCP-mode selection currently read only that task-level
marker. If several sessions launch before one of them successfully sets the title, each can receive the
instruction and tool. That makes pending state a race window rather than a durable ownership boundary.

The feature was also introduced as opt-in. Agent-generated titles are now the desired default, but an
existing user who explicitly disabled the preference must not be opted back in.

## Decision

A prompt-first task begins with `tasks.metadata.agent_title_pending=true` and no owner. When an
eligible task-mode session begins its initial turn, Kandev atomically claims the task by writing
`tasks.metadata.agent_title_owner_session_id=<session-id>`. Session preparation alone does not claim.
Config, Office, and External sessions are ineligible; structured and passthrough task-mode sessions are
eligible.

The claim compare-and-set requires pending state and an absent owner, is idempotent for the same
session, and rejects every different session. Kandev performs it before recording or composing the
initial prompt, configuring title-capable MCP mode, or starting the agent process. Workspace/agentctl
preparation alone uses ordinary task mode and does not claim. A persistence error fails the launch
closed. Once a session owns the title, ownership is not reassigned when its process fails, it ignores
the instruction, or its title call fails. A retry or resume of that same session can retain the
capability while the title is pending.

Prompt composition and MCP-mode resolution require both pending state and an owner ID equal to the
launching session. The existing internal `task-title-pending` MCP mode value remains stable, but only
the owner resolves to it. The MCP title mutation also compares the server-injected session ID with the
persisted owner; catalog gating is not the authorization boundary.

A successful owner title update atomically changes the title and removes both metadata keys. An
ordinary human title update also removes both keys. Metadata merge logic excludes both internal keys
from stale pending snapshots so an unrelated task update cannot restore resolved ownership. Newly
persisted ownership is published through the normal task-update event path.

The portable `agent_generated_task_titles` user preference defaults to `true` for new settings and for
stored JSON that omits the field. An explicitly stored `false` remains false. Backend decoding and
frontend boot/WS fallbacks use the same missing-versus-explicit distinction.

## Consequences

- Concurrent workflow agents expose title-generation instructions and schema to exactly one session.
- A failed owner does not make later sessions spend context or unexpectedly rename the task; the
  provisional title remains the fallback.
- Ownership survives backend, executor, and browser restarts without a new table or schema migration.
- Pending tasks created before the owner key exists remain compatible: their next eligible initial-turn
  launch claims ownership.
- Claim failure aborts an initial launch instead of allowing prompt/catalog state to disagree.
- New and legacy users get the prompt-first creation flow unless they explicitly opted out.
- Test fixtures that intentionally exercise manual-title creation must save `false` explicitly instead
  of relying on an omitted preference.

## Alternatives Considered

1. **Continue gating only on `agent_title_pending`.** Rejected because concurrent or failed launches
   can give multiple sessions the same one-shot responsibility.
2. **Remove pending state as soon as a session launches.** Rejected because it loses the distinction
   between an unresolved provisional title and a completed title, and weakens the human-versus-agent
   compare-and-set.
3. **Reassign ownership after launch failure.** Rejected because later agents would receive surprise
   instructions and schema; the chosen contract is one session, with the provisional title as fallback.
4. **Mark the owner only in session metadata.** Rejected because selecting one winner across concurrent
   sessions would require a task-wide query/constraint and would make task-title mutation depend on a
   second row's lifecycle.
5. **Use a new task column or claim table.** Rejected because the ownership state is small, temporary,
   and naturally part of the existing pending-title metadata lifecycle.
6. **Flip every stored preference to true.** Rejected because it would overwrite explicit user opt-outs.
