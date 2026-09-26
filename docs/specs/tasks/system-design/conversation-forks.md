---
status: current
system: tasks
requirements:
  - REQ-TASKS-CONVERSATION-FORK-001
  - REQ-TASKS-CONVERSATION-FORK-002
  - REQ-TASKS-CONVERSATION-FORK-003
  - REQ-TASKS-CONVERSATION-FORK-004
  - REQ-TASKS-CONVERSATION-FORK-005
  - REQ-TASKS-CONVERSATION-FORK-006
created: 2026-09-23
owners:
  - kandev
---

# Conversation fork system design

## Boundary and mapping

The task service owns compilation, snapshot retention, and destination admission.
The browser selects source identifiers and options. It never supplies authoritative compiled history.
The orchestrator consumes an admitted context reference through existing launch paths.
The agent runtime receives fresh execution identity and an ordinary text context block.

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-CONVERSATION-FORK-001` | Source read, Compilation |
| `REQ-TASKS-CONVERSATION-FORK-002` | Compilation, Attachments, Prompt composition |
| `REQ-TASKS-CONVERSATION-FORK-003` | Estimates, API, Interface |
| `REQ-TASKS-CONVERSATION-FORK-004` | Admission and delivery, Prompt composition |
| `REQ-TASKS-CONVERSATION-FORK-005` | Persistence, Authorization and failures |
| `REQ-TASKS-CONVERSATION-FORK-006` | Interface |

## Existing code and extensions

Existing code provides these integration points:

- `task/service/service_conversation_source.go` authorizes revision-bearing reads.
- `task/repository/sqlite/conversation_source.go` reads source rows with normalized timestamp and ID ordering.
- `task/service/attachment_service.go` owns attachment staging and private file storage.
- `backendapp/adapters.go` forwards admitted snapshot reads and provenance updates through the orchestrator message-creator boundary.
- `task/handlers/task_http_handlers.go` owns HTTP task creation and initial launch preparation.
- `orchestrator/session_launch.go` defines `LaunchSessionRequest` and `LaunchSession`.
- `orchestrator/task_create_prompt.go` and `workflow_start_prompt.go` own parts of initial prompt composition.
- `components/task/chat/messages/message-actions.tsx` owns the message action row.
- `components/task/new-session-dialog.tsx` and `new-subtask-dialog.tsx` provide destination forms.
- `components/task-create-dialog-submit.tsx` submits new tasks.
- `components/task/chat/messages/prompt-mention-components.tsx` supplies the chip interaction precedent.
- `lib/services/session-launch-helpers.ts` constructs browser launch requests.

Backend paths in this list are relative to `apps/backend/internal/`.
Browser paths are relative to `apps/web/`.
New `conversation_fork*.go` and `conversation-fork*.ts[x]` files below are proposed files, not existing APIs.

`SessionHistoryManager.GenerateResumeContext` is not the compiler.
Its fallback keeps only 50 messages and truncates each message to 2,000 characters.
`useSummarizeSession` also does not provide the snapshot boundary or a complete paged read.

## Source read

Add a repository operation that reads the selected range in one bounded database read transaction.
Read the session, cutoff, applicable turns, clarification answers, and message rows from that same snapshot.
Use SQLite transaction isolation and a PostgreSQL repeatable-read transaction.
Do not hold the transaction across browser requests, attachment copying, or tokenization.

Reuse source ordering: normalized `created_at`, then message ID.
Resolve both range endpoints from authorized rows. Timestamp equality must use the same ID tie-break as the source reader.
The default start is the earliest available row. An optional starting message narrows the range without extending the cutoff.
Query only the allowlisted content columns and bounded evidence projections.
Reject excessive row count or payload size before loading unbounded content.

Existing page methods use separate transactions. Repeated calls alone do not create a consistent snapshot.
Do not build the fork from the browser viewport cache or plugin-facing subscriptions.
The new operation returns copied rows, source revision, source labels, and omission metadata for compilation.

A user row becomes eligible after durable acceptance, including while its assistant turn remains active. An assistant cutoff requires a terminal turn and persisted final content.
Legacy assistant rows without sufficient completion evidence are unavailable as cutoffs.
Earlier partial output from an interrupted turn is labeled interrupted historical output.
Turn status never expands the selected range to include later messages or later answers.
Clarification answers completed after the cutoff are excluded, even if they now exist on an earlier question record.

## Compilation

Use a deterministic compiler version, initially `conversation-fork-v1`.
The output has a server-written introduction, source labels, explicit roles, chronological text, and an omission summary.
All content appears as historical data. It grants no permissions, title ownership, workflow signals, or destination instructions.
Escape compiler delimiters in source content. Do not rely on source-supplied role labels.

| Source content | Projection |
| --- | --- |
| `message`, `content` | Sanitized visible text with user or assistant role |
| Answered `clarification_request` | Question and accepted answer once, within cutoff |
| Tool rows when the include switch is on | Tool name, recorded input, output, and completion status as inert text |
| `thinking`, permissions, logs, progress, status, scripts | Omitted with category counts |
| `agent_plan`, `todo` | Omitted as runtime controls in version 1 |
| Hidden `<kandev-system>` blocks | Removed through `sysprompt.StripSystemContent` |
| Attachments | Inventory by safe descriptor, excluded until selected |

Do not serialize arbitrary metadata. Tool input/output fields need explicit adapters for the stored tool types.
Unknown types stay excluded and appear in omission counts.
Retained evidence contains no tool-call IDs that can trigger provider replay.
A source payload already removed by retention appears unavailable. The compiler cannot reconstruct it.
The include switch defaults to false and applies to the entire selected range. There is no individual tool-output picker.
If included tool evidence is unavailable, compilation fails visibly. The user can disable tool inclusion, narrow the range, or retry.
Never truncate selected text silently. Selections beyond the snapshot storage bounds fail with a storage-limit error.
Model context size does not impose a compilation limit.

Historical `@name` references remain literal text. The preview explains that hidden saved-prompt definitions are excluded.
The historical segment never enters saved-prompt lookup, entity expansion, workflow action parsing, or system-content trust promotion.
Preserve the reviewed compiled bytes in the ordinary user-prompt content. A separate server-authored trusted boundary instruction tells the model to treat that segment as historical data; snapshot text never enters the trusted system-context parameter.

Default history is one source session. A source first prompt can already contain an earlier admitted fork.
Use its persisted fork provenance to include that earlier context once, as nested historical context.
Do not traverse source-task ancestry or concatenate the same snapshot from both metadata and expanded text.
Normal payload bounds also apply to nested context.

## Persistence

Add `task_conversation_forks` with these fields:

- Snapshot ID, owner ID, workspace ID, source task/session/message IDs, and optional start message ID.
- Source revision, copied source labels, compiler version, compiled text, and content hash.
- Selection manifest, omission counts, included message count, UTF-8 byte count, and estimate metadata.
- Attachment-copy descriptors, creation time, expiry, lifecycle state, destination task ID, and optional destination session ID.
- Separate draft-request and destination-request IDs, each with an immutable request fingerprint.

Unique owner/request constraints protect both request kinds. The draft and destination receipts never share a namespace.

States are `draft`, `attached`, and `discarded`.
Expiry is a condition on drafts. Reads and admission enforce it even before cleanup runs.
A draft is immutable. Changing the range, evidence, or attachments creates a replacement draft.
Model selection changes only estimate metadata, not compiled content or the content hash.

Initial engineering defaults are 24-hour draft expiry, 4 MiB compiled UTF-8 text, and 10,000 selected source rows.
Limit each owner to 20 active drafts. Clean expired drafts before rejecting a quota request.
Bound candidate listings to 100 entries per page and compiler work to the source limits.
Return a limit error without creating a partial snapshot. Tests inject smaller bounds.
These are internal constants for the initial implementation, not new operator settings.

Store full text only in the snapshot record. Messages carry a small server-authored reference and provenance.
Do not copy full text into task descriptions, task metadata, boot payloads, or session summary events.
An admitted first-message projection resolves the reference for human preview and provider delivery.
The snapshot belongs to exactly one destination creation request. A retry returns that same destination.
A separate intentional fork creates a separate snapshot.

Task destinations keep a `destination_complete` receipt marker separate from the `attached` snapshot state.
The create handler marks the receipt complete only after synchronous destination setup succeeds.
An idempotent retry of an incomplete receipt returns an unsettled result so the handler can finish that setup.
If setup fails, rollback restores the fork to `draft` and its copied attachment claims to staging in one transaction before deleting the partial task.
This keeps a retryable snapshot from being mistaken for a completed destination or lost with a failed task create.

Attached snapshots remain until their owning destination is deleted under existing task/session cleanup rules.
A pending task-bound snapshot survives until the first session binds it or the task is deleted.
Source foreign keys must not cascade deletion into attached destination snapshots.
Source IDs become provenance only after attachment. Workspace deletion removes its snapshots and copied attachments.
Cleanup must coordinate with admission so an expiring draft cannot disappear during a successful attach.
Discarded drafts release compiled text, selection, omissions, and estimate data after the state change succeeds.
Expiry cleanup returns expired attachment descriptors with deleted rows so private staged files can be removed; attached copies do not expire with drafts.

This is an explicit user-created context artifact, not a second live conversation journal.
The [source reconciliation decision](../../../decisions/2026-09-16-conversation-source-reconciliation.md) remains authoritative for ordinary history reads.
The hash verifies this immutable artifact. It does not prove event delivery or cache freshness.
Memory-only drafts lose previews on restart. Compile-at-launch can change reviewed content.
Neither alternative satisfies this feature. This local rationale is sufficient without a new global ADR.

## Attachments

The existing attachment registry prohibits reuse of session-bound claims across destinations.
Never pass source attachment IDs directly to destination launch.

For selected available files, stream copies through `AttachmentService` into private draft-owned staging.
Authorize source reads and derive storage paths on the server.
Record the original descriptor and a destination copy descriptor separately.
Enforce existing file-count and aggregate-byte limits across copied and newly uploaded attachments.
No base64 bytes enter snapshot JSON, messages, or the shared WebSocket.

Copy all selected files before the snapshot becomes ready. On failure, remove temporary copies and keep the previous ready draft intact.
Bind copied attachment claims with snapshot admission. Do not steal or transfer ownership from the source.
Reference-counting source files is outside this package. Physical copies preserve destination retention without changing the attachment ownership contract.
Failed or expired draft cleanup removes only its copies.
On first-prompt delivery, merge retained-copy descriptors with newly uploaded attachments and pass the combined list through ordinary destination materialization and error handling.
Enforce the existing aggregate count and byte limits before runtime dispatch. Do not re-add copied attachments on retries or later sessions.
Task creation checks copied plus newly uploaded attachments before creating the destination, and launch checks the same combined batch before materialization.
The compiled text identifies attachment positions by safe display names and ordinal, not source paths.

## Estimates

Add a task-owned estimator that uses the existing `tiktoken-go/tokenizer` dependency.
Keep the MCP tool estimator contract unchanged.
Return `estimated_tokens`, `method`, `model_id`, `context_limit`, `limit_source`, and an unmeasured-attachment indicator.
Use a supported exact tokenizer mapping only when the installed metadata establishes that mapping.
Otherwise use `o200k_base` as a labeled approximation, never an exact cross-provider count.
Do not add guessed model-name-to-window tables.

Resolve a limit through the selected profile's existing model metadata, including `ModelEntryDTO.ContextWindow` where available.
Unknown or ambiguous model identities return a null limit and no percentage.
Runtime-reported usage belongs to the destination session after launch. It does not replace the pre-launch estimate.
The percentage is `estimated fork tokens / known context limit` and is labeled approximate.

Estimate the complete compiled historical block, including its wrapper and omission text.
Show the new user request separately. Runtime instructions, tool schemas, output reservation, and binary attachment costs remain additional overhead.
Estimates are informational, including percentages above 100%. Do not add a context-budget admission gate or threshold warning.
Do not summarize, shorten, or switch models automatically. The provider can accept or reject the submitted context.
Use existing launch or prompt failure handling for provider context errors. No fork-specific recovery flow is required.
Existing storage, attachment, and transport byte limits remain separate infrastructure constraints.
If a launch resolves a different model, retain its normal model reporting. Estimate freshness cannot delay or reject launch.

## API

Proposed authenticated first-party endpoints:

| Endpoint | Contract |
| --- | --- |
| `GET /api/v1/task-sessions/:id/fork-candidates` | Bounded range, tool evidence, and attachment choices through a required cutoff |
| `POST /api/v1/task-sessions/:id/conversation-forks` | Compile a draft from cutoff, optional start, `include_tool_evidence`, attachment IDs, and selected profile/model |
| `GET /api/v1/conversation-forks/:id` | Small descriptor, provenance, exclusions, expiry, and estimate |
| `GET /api/v1/conversation-forks/:id/content` | Authorized full compiled text with private, no-store caching |
| `POST /api/v1/conversation-forks/:id/estimate` | Recalculate for selected profile/model without changing content |
| `DELETE /api/v1/conversation-forks/:id` | Discard an unattached draft and its staged copies |

Candidate pages are advisory. Compilation independently validates each range endpoint and selected attachment ID in its consistent source read.
Source changes can invalidate a choice and return a recoverable error.
Draft creation includes a client-generated request ID for network retries without quota leaks.
The response returns the snapshot ID and hash. Clients never submit authoritative text, owner IDs, or storage paths.
Full preview uses the content endpoint rather than an oversized WebSocket message.

Extend existing HTTP task creation and `LaunchSessionRequest` with an optional `conversation_fork_id` and creation request ID.
Reject the field for resume, restore, prepare-only, or other intents that cannot own a new first conversation.
Internal task preparation receives an already admitted reference, not public attachment authority.
The exact response DTOs retain current task/session result shapes plus a bounded fork descriptor.

## Admission and delivery

1. Authorize the snapshot, source, and requested destination. Resolve profile and workspace choices through existing services.
2. Verify expiry, content integrity, attachment availability, and storage/transport byte limits before runtime side effects.
3. Atomically create the destination and bind the snapshot plus copied attachment claims.
4. Commit the creation receipt and immutable request fingerprint with that binding.
5. Publish task/session events and schedule launch only after commit.
6. Bind a task's pending fork to exactly one first session through the existing first-prompt ownership boundary. Only a successful atomic claim adds the fork ID to that session's delivery metadata; task provenance is not inherited as session delivery metadata.
7. Persist one user message with its admitted fork reference and the new request. Merge copied attachments into the first launch's current attachment list, then dispatch the reconstructed prompt.

When an already-bound first launch fails without a live execution, retry the same destination session.
Reuse its persisted first user message and resolve the same frozen snapshot and attachment copies for redispatch with message recording skipped.
Do not append another fork block or first user message. Normal later sessions never consume task-level fork provenance.

The transaction requires a narrow repository admission operation. Sequential create-then-attach calls are insufficient.
Reuse the current task creation and session initialization semantics inside that operation.
Do not build a second launcher or invent a new workflow scheduler.
An idempotent retry returns the saved destination. A changed fingerprint under the same request ID returns conflict.
An attached snapshot cannot create another destination.

New tasks have no parent. Child tasks set the source task as parent through normal validation.
New agents retain the source task ID and receive a new session ID.
Execution workspace rules are destination-specific:

- A new agent attaches to the source task's current execution workspace.
- A new task requires a separate execution workspace and cannot inherit or share the source environment.
- A child task exposes the existing `inherit_parent` versus `new_workspace` choice.

These destinations remain in the same Kandev workspace for authorization and task grouping.
Validate the execution workspace rule on the server, not only in the form.
A Local executor that would reuse the source checkout cannot satisfy separate-workspace mode.
Offer an isolation-capable executor or a typed unsupported error. Do not silently share or change the selected mode.
A separate workspace does not copy the source's uncommitted files. Use existing task defaults for its initial Git baseline.
Do not introduce a fork-specific source-commit default or a required base-selection step. Existing repository selectors remain available.
Same-task sessions obey [additional-session workspace reuse](additional-session-workspace-reuse.md).
Fork history never grants an exemption from workspace safety, profile compatibility, permission, or capacity checks.

Create-without-start keeps a task-bound pending snapshot. It does not require an agent runtime. Task-level provenance remains durable after the first claim without making later sessions eligible for fork delivery.
Carry the admitted reference through workflow auto-start, explicit start, CREATED-session start, and capacity deferral.
Each route consumes the same initial-message ownership claim, rather than inferring eligibility from visible message count.
Failed starts retain the accepted binding for ordinary recovery.
Message retries and resumed provider conversations do not append the historical block again.
Provider-level exactly-once delivery after an ambiguous network failure remains outside the guarantee.

## Prompt composition

Keep the historical segment structurally separate until current workflow and saved-prompt processing finishes.
Compose current instructions and the current user request through the existing trusted preparation pipeline.
Then insert the already sanitized historical block before the current user request.
A workflow template cannot replace or omit the admitted fork or current request.
The source block cannot expand `@name`, inject `<kandev-system>`, or select destination task/session IDs.

Store a server-authored fork reference on the accepted first message. Never accept this authority from arbitrary client metadata.
Use one resolver for provider dispatch and the full first-message preview.
The destination transcript shows a collapsed provenance chip with the new request, not fabricated old turns or usage events.
The source link can become unavailable after source deletion while the retained preview remains readable.

Passthrough destinations receive visible historical text only through their existing initial-prompt delivery path.
A profile with no supported initial-text delivery returns a typed unsupported error before creation.
Passthrough source transcripts without recorded message boundaries are not eligible.
Fork-on-fork tests must prove one expansion, no cycles, and no duplicate attachment inclusion.

## Interface

Add the action to `MessageActions`, with accessible text and completed-message eligibility from server data.
The source session comes from the selected message, never the task's primary-session heuristic.
Start compilation as the destination picker opens. Destination selection can proceed while compilation is pending.
Submission remains disabled until the snapshot and selected attachments are ready.

Reuse `PromptMentionChip` interaction patterns, not saved-prompt storage or expansion semantics.
The chip shows source, selected message count, estimated tokens, and a remove control.
Hover and focus show a bounded excerpt. Click opens full content with range, tool-inclusion switch, and attachment controls.
Each changed selection creates a replacement snapshot. The old ready snapshot remains usable until replacement succeeds.
An explicit Apply action selects the replacement. Stale responses cannot replace a newer selection or model estimate.
Removing the chip discards an unattached draft and restores ordinary creation behavior.

Reuse all three destination forms and their existing validation.
New-agent forms state that files are shared. New-task forms state that files are separate.
Child-task forms expose both workspace modes. No other destination exposes a sharing toggle.
Selecting a fork disables copy-prompt and summarize-session modes that would replace the context.
Users can remove the fork before selecting those modes. No automatic summarization occurs.
Keep snapshot ID, selection, model estimate generation, and submission request ID in a shared domain hook.
Do not put full text in the global task store or persistent browser storage.

Desktop uses a destination dialog, the existing creation dialog, and a navigable full-preview subview.
The nearest touch exemplar is the prompt mention Drawer branch with `useTouchDrawer`.
For phone destination selection, reuse the inset picker pattern in `components/task/mobile/mobile-picker-sheet.tsx`.
For dense preview and creation, use one full-height surface with header, scroll body, and fixed primary action.
Back returns to the creation view without losing text. Avoid stacked drawers.
Use `100dvh`, safe-area padding, and at least 44-pixel touch targets. Desktop ordinary controls remain 28 pixels.
The shared hook owns behavior across layouts. Responsive wrappers own presentation only.
All new copy uses English, Portuguese, Simplified Chinese, both Traditional Chinese catalogs, and Japanese translations.
Generate Traditional Chinese variants with the repository command and retain pseudo-locale checks.

## Authorization and failures

Draft reads require the owner and current source access. Creation also rechecks source and destination permissions.
Initial transfers stay within the source Kandev workspace and existing user access boundaries.
After successful creation, snapshot reads use destination access. A copied snapshot is destination content.
Do not leak inaccessible source names through errors or guessed IDs.
If permissions change before admission, refuse attachment without partial destination creation.

Use typed errors for expired drafts, unavailable source, unfinished cutoff, storage/transport limits, unsupported destination, and storage failure.
The UI preserves the new request and offers retry, a smaller range, deselection, or chip removal.
An unavailable selected attachment or tool payload cannot disappear silently.
No context is published through ordinary shared task summaries or public task sharing by default.

Schema initialization must remain replayable on SQLite and PostgreSQL.
Existing tasks and clients without fork fields retain their behavior.
Do not backfill old conversations or change plugin conversation reconciliation.
Structured logs contain operation, outcome, sizes, and compiler version, but no transcript, file bytes, or private paths.
Reuse existing launch diagnostics for failures after admission.

## Related contracts and delivery

- [Requirements](../requirements/conversation-forks.md)
- [Implementation plan](../../../plans/conversation-forks/plan.md)
- [Initial task brief](initial-task-brief.md)
- [Prompt attachments](prompt-attachments.md)
- [Saved-prompt delivery](saved-prompt-delivery.md)
- [Server-owned saved-prompt expansion](../../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md)

Public task guidance now describes the implemented fork flow and its workspace behavior.
The feature does not introduce a release toggle.
