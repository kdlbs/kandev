# 0033: Durable Plan Implementation Start Marker

**Status:** accepted
**Date:** 2026-07-09
**Area:** backend, frontend

## Context

Plan mode can create a reviewed task plan, then the user may start implementation from the chat composer, the plan toolbar, or by typing an equivalent instruction directly. The UI needs to know whether to keep showing a prominent Implement action after the user starts implementation. Inferring that from messages is ambiguous because the agent could be asked to implement without using a button, messages can be edited or summarized, and page refreshes must preserve the answer.

## Decision

`task_plans` stores an implementation-start marker: timestamp, session id, and actor. The marker is set by an explicit backend action when the UI starts implementation from an Implement button. The write is idempotent: the first implementation start wins, and later clicks or plan edits do not clear or replace it.

Task plan reads, WebSocket plan events, and frontend task-plan state include the marker. The plan toolbar shows its accent Implement action only when the current draft has non-empty content and the persisted marker is absent. Before starting implementation from the toolbar, the frontend saves any unsaved plan edits, sends the normal implement prompt to the target session, then marks the plan implemented so the button remains hidden after refresh.

## Consequences

The UI no longer needs to infer implementation state from chat history, so refreshes and multi-client updates behave consistently. Keeping the marker on `task_plans` also avoids coupling the toolbar to any one agent/session message format.

Direct typed implementation requests that bypass the button are not detected automatically; they remain intentionally outside this marker because the system cannot reliably distinguish them from discussion or partial implementation requests.

## Alternatives Considered

1. **Infer from session messages.** Rejected because prompt text, hidden system blocks, summaries, and manual user instructions make detection unreliable.
2. **Use browser-local state.** Rejected because it would not survive another client, DB-backed task reload, or durable page refresh semantics.
3. **Clear the marker when the plan changes.** Rejected because editing the plan after starting implementation should not reintroduce the primary Implement CTA for the same task.
