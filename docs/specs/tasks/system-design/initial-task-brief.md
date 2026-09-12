---
status: current
system: tasks
requirements:
  - REQ-TASKS-INITIAL-TASK-BRIEF-001
created: 2026-09-12
updated: 2026-09-12
owners:
  - kandev
---

# Initial task brief system design

## Boundary and mapping

The task service owns accepted prompt content. The repository selects initial
content inside its existing session write transaction. The handler dispatches
the committed result. This extends the existing first-prompt boundary.

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-INITIAL-TASK-BRIEF-001` | Admission, Composition and delivery, Transcript, Failure and compatibility |

## Admission

`MessageHandlers.wsAddMessage` already resolves session changes after
`ProcessOnTurnStart`. Eligibility belongs to that resolved session, including
a CREATED source redirected to a fresh workflow session. The service verifies
task/session ownership and excludes Office, ephemeral, and configuration contexts.

Use a server-only admission option on `CreateMessageRequest`. It carries a
prepared initial-content candidate and its corresponding trusted prompt context.
This option is not a public request field and is never accepted from client metadata.
Existing callers without the option keep their current behavior.

Select between ordinary and initial candidates inside the existing repository
user-message transaction, before `assignUserMessageBoundary` allocates an ordinal.
Read `task_session_prompt_seq` with the same transaction and session write lock.
An existing row, including `last_seq = 0`, means that another prompt owns the
first boundary. Do not use CREATED state, transcript length, title ownership,
or the eventual ordinal alone as proof of eligibility.

Extend the shared boundary used by ordinary, idempotent, plan-comment, and
queued plan-comment message writes. Keep candidate selection, message insertion,
ordinal allocation, comment consumption, and queue insertion atomic where those
operations are already atomic. Preserve SQLite writer serialization and the
PostgreSQL session advisory lock. A new counter table or migration is unnecessary.

The candidate uses a task-description snapshot read from authorized task data.
Validate that snapshot against the task row during final admission. A concurrent
description edit rejects stale admission for recomposition, rather than saving
an unintended stale brief. Do not run saved-prompt lookups inside the write transaction.

Return the selected content and selected trusted-context identity to the service.
The same selection determines persistence, queue content, and agent dispatch.
On rollback, restore caller-visible content and metadata as well as timestamps
and prompt ordinal. Otherwise retries can retain a failed selection.

## Composition and delivery

For an eligible candidate, put the trimmed brief before the additional
instruction, separated by two newlines. Exact equality after outer trimming
produces one copy. Empty descriptions use the ordinary candidate.

Prepare both candidates through the existing server-owned saved-prompt and
system-context pipeline before admission. Resolve title ownership, canvas
capabilities, and session transitions once, then reuse their canonical snapshots. Keep each candidate paired with its
trusted expansion. Fingerprint the original request before either transform.
An idempotent replay returns the saved winner even after the description changes.

`wsAddMessage` already assigns `req.Content = message.Content` before dispatch.
Extend that handoff to use the selected trusted context. Do not independently
prepend the brief inside `StartCreatedSession`, `startTask`, or the browser.

The real created-session launch must preserve both visible sections through
workflow composition. Compose the applicable workflow instructions once and
retain the accepted direct content as user input. If a replacing step template
would discard this content, append the direct content after that template and
use the existing composed-prompt launch option to prevent a second replacement.
Keep automatic workflow-entry composition unchanged. Test empty step prompts,
`{{task_prompt}}`, and nonempty templates without that placeholder.

The saved user row and final dispatch must contain the same visible brief and
instruction. Existing canonical system-context additions retain their authority.
Passthrough delivery retains visible text without hidden saved-prompt expansion.
Keep submitted attachments, entity references, and plan-comment claims intact.
`initial_prompt_preview` remains display-only and cannot supply prompt authority.

## Transcript

Keep `useProcessedMessages` and `shouldShowTaskDescriptionFallback` eligibility
unchanged. Before admission, the synthetic description row represents the brief.
After admission, prompt #1 replaces it and contains the brief plus instruction.
No synthetic historical prompt, extra ordinal, or permanently pinned banner is needed.

Reuse the existing transcript and Prompt History renderers. The nearest mobile
exemplar is `components/task/task-layout.tsx` and its `SessionMobileLayout`.
Phone Chat remains a full-height destination with one transcript scroll owner.
Composer position, safe-area handling, touch targets, and navigation remain unchanged.
Long first prompts remain reachable through existing prompt expansion/history controls.

## Failure and compatibility

A history, snapshot, or persistence error stops admission before dispatch.
Existing launch-error behavior handles failures after message acceptance. This
package does not promise provider-level exactly-once delivery after a process crash.

Race tests must include a direct request competing with
`ClaimInitialPromptFallback`, two distinct direct messages, and a same-ID retry.
A zero-valued fallback reservation suppresses direct brief insertion.
Deleted user rows cannot make an existing prompt-counter row eligible again.

Existing launch, workflow-entry, and Office callers do not opt into this direct
admission option. No public API field, database migration, or new UI control is needed.
This design records a local extension of existing ownership, so no separate ADR is required.

## Related contracts

- [Workflow first-prompt ownership](workflow-step-agent-start-ownership.md#prompt-history-contract)
- [Saved-prompt delivery](saved-prompt-delivery.md)
- [Transcript history visibility](../../ui/system-design/task-prompt-transcript-visibility.md)
- [Server-owned saved-prompt expansion](../../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md)

## Implementation plans

[Initial task brief fix package](../../../plans/initial-task-brief/plan.md)
