---
status: current
system: tasks
created: 2026-11-03
requirements:
  - REQ-TASKS-ANSWER-ATTACHMENTS-001
  - REQ-TASKS-ANSWER-ATTACHMENTS-002
  - REQ-TASKS-ANSWER-ATTACHMENTS-003
---

# Question answer attachments system design

## Purpose and boundaries

The tasks system owns attachments as part of a clarification response.
The design reuses file-backed uploads, task-message metadata, the durable response claim, and authenticated agentctl materialization.
It adds no attachment store, table, runtime flag, or browser-to-agentctl connection.

The [requirements](../requirements/question-answer-attachments.md) define the observable outcome.
The [plan](../../../plans/question-answer-attachments/plan.md) defines the implementation sequence.
All new symbols in this document are proposed interfaces, not existing APIs.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-ANSWER-ATTACHMENTS-001` | Answer contract, Agent delivery, Composition |
| `REQ-TASKS-ANSWER-ATTACHMENTS-002` | Atomic admission, Recovery and retention, Authority and security |
| `REQ-TASKS-ANSWER-ATTACHMENTS-003` | Composition, Mobile contract, Failure presentation |

## Current boundaries

`internal/clarification/protocol.Answer` and web `ClarificationAnswer` currently contain question ID, selected options, and custom text.
`response_serialization.go` separately defines durable answer encoding.
`mcp/server.simplifyAnswer` projects only selections and text, then adds the selected label through the existing question lookup.

`Resolver.ResolveBundle` authorizes and validates before `CompleteActiveClarificationBundle` claims current-turn messages.
The repository transaction persists the response and `response_delivery_pending` before delivery.
Live waiter confirmation, detached resume acknowledgement, and guarded restoration already own response recovery.

`AttachmentService` stages private files and returns opaque IDs.
`task_message_attachments` already has owner, workspace, task, session, and message fields.
`SessionManager.materializeAttachments` reads claimed files and streams them through agentctl.
`ClarificationInputOverlay` currently auto-submits a committed single-question answer.

## Answer contract

Add optional `attachments` to each browser and durable answer.
Use the existing message descriptor vocabulary: `attachment_id`, `type`, `name`, `mime_type`, `size_bytes`, and `delivery_mode`.
The answer route accepts only file-backed references with `delivery_mode: "path"`.
It rejects inline data, caller-supplied paths, and unsupported delivery modes.
Server registry rows supply canonical metadata and byte counts.

Example browser submission:

```json
{
  "answers": [{
    "question_id": "layout",
    "selected_options": ["layout_opt1"],
    "attachments": [{
      "attachment_id": "opaque-upload-id",
      "type": "image",
      "name": "screenshot.png",
      "mime_type": "image/png",
      "size_bytes": 123,
      "delivery_mode": "path"
    }]
  }]
}
```

Preserve the existing no-file encoding by omitting the new property when empty.
Extend both `SerializeResponse` and `DeserializeResponse`, normalization, winner reconstruction, and HTTP result parsing.
All copies must retain question and attachment order.
Validate the combined file set across the bundle, not ten files per question.
Reject duplicate IDs across questions as well as within one answer.
A file-bearing answer still needs a valid option or permitted nonempty custom text.
Legacy file-free empty entries retain their existing external-client behavior.

## Atomic admission

Add an attachment-aware completion operation beside the existing no-file operation.
A proposed `ClarificationAttachmentClaim` carries authenticated ownership and per-question attachment IDs.
It belongs in task models so repositories do not import the clarification service.
The task service derives owner, workspace, task, and session from authenticated and persisted state.
It does not accept those fields from an answer descriptor.

The attachment-aware repository transaction must:

1. Use the existing queue/session, task, and turn lock order before the current-turn response claim.
2. Claim only eligible pending question rows with the existing malformed-bundle and terminal-session guards.
3. Validate every file's owner, scope, expiry, state, metadata, count, and aggregate raw size.
4. Adopt the complete file set and bind each file's `message_id` to its corresponding question row.
5. Write the canonical per-question response and its attachment manifest, then commit together.

Any invalid file rolls back all new row and attachment changes.
A losing responder makes no attachment changes and reconstructs the winner as today.
Authorization and structural validation still precede the claim, including for retries.
Rows with retained retry manifests use the attachment-aware operation even when the next answer has no files or is skipped.
Do not call `AttachmentService.Claim` in a separate transaction and then complete the answer.
Reuse its transaction-level predicates and the task-creation claim pattern instead.

Each affected question row stores its canonical manifest in `metadata.attachments`.
Its `metadata.response.attachments` is the answer projection of that same manifest.
Both derive from the same validated set in the transaction.
This keeps existing transcript-reference cleanup aware of retained files.
Do not count files from terminal siblings that this claim does not own as new uploads.
Keep mixed terminal/pending sibling behavior from the active-lifecycle contract.

## Agent delivery

Uploads for answers use path delivery for images and other files.
The question result supplies the file reference with its answer.
Agents inspect an image through their existing file-reading capability.
No base64 image data or separate ordinary user message enters the live wait.

Expose a narrow runtime operation around `SessionManager.materializeAttachments` for an existing execution.
It must not start a prompt, change prompt generation, or cancel the question.
Run it only after the durable answer claim and before releasing the live waiter.
Bind it to the source session, active execution, and claimed conversational turn.
Recheck those identities after streaming and before response delivery.

Build a separate, delivery-only projection containing generated executor paths.
Keep those paths out of browser REST results, durable serialization, and message metadata.
The internal backend response can carry this projection keyed by question and attachment ID.
The agent MCP result exposes optional `attachments` beside `selected_option`, `selected_label`, or `custom_text`.
Each result attachment includes its canonical metadata and generated executor `path`.
The output schema must allow this field, and JSON text must match structured content.

The existing native Codex bridge uses string answer arrays.
Extend its internal answer representation with trusted attachment references.
Keep the exact option-label entry or permitted custom-text entry unchanged.
Append a separate file-reference entry for each materialized attachment in that question's answer array.
Do not insert attachment text into `custom_text` or weaken `isOther` validation.
The repository's versioned native schema remains authoritative.

Detached current-turn responses pass canonical file descriptors through `DetachedClarificationResume` and the existing `promptTask` attachment path.
They must materialize after execution readiness and before dispatch acceptance.
The clarification prompt identifies each question's corresponding files.
Watchdog and restart recovery reconstruct from the exact claimed question rows, never the task-wide attachment inventory.
An exact tool retry can reconstruct delivery paths for its current execution without dispatching another answer.

## Recovery and retention

Keep the five-second pre-claim and existing post-claim delivery budgets.
Network file streaming occurs outside the database transaction.
A materialization failure follows the existing guarded restoration path and returns a retryable failure.
The failed claim's `metadata.attachments` remains on its own question rows after `metadata.response` is removed.
Those files remain claimed and reusable only by that question's retry.
The pending UI restores that manifest as retryable answer files.

A subsequent successful answer replaces only its owned retry manifest.
After commit, release removed files through the existing reference-aware cleanup path.
Skip clears an owned retry manifest and releases its unused files without agent resume.
Restoration cannot remove or replace terminal siblings' manifests.
Cancellation or a superseded turn must never release another claimant's files.

On restart, canonical IDs and the existing delivery-pending evidence support reconstruction.
Runtime paths are regenerated for the actual execution.
No stored path proves that bytes exist after execution replacement.
Preserve the existing rules for uncertain provider acceptance rather than promising exactly-once delivery across a crash.
Accepted history retains files until the owning session, task, or workspace cleanup applies.

## Composition

Reuse `processFile`, `readClipboardAttachments`, attachment upload/content/delete APIs, file previews, and localized size feedback.
Do not reuse the whole chat composer or create a second attachment registry.
A focused answer-attachment hook owns uploads by task, source session, pending ID, and question ID.
Resolve the workspace from the question's owning task, including Inbox and Coordinator hosts.
The currently selected workspace or unrelated session must not supply upload scope.

Place a paperclip action immediately to the right of the existing custom-answer textarea, within the same row.
The trigger shows a small file-count badge, uploading state, or error indicator.
Its accessible name and status description are localized.
It opens an attachment manager with Add files, per-file status, inspection, Retry, and Remove controls.
Use a desktop popover and a phone or coarse-pointer drawer through the existing responsive primitives.
Keep file rows inside that on-demand surface, with no inline attachment tray or new footer.

Retain the existing trailing Send action for single-question final submission.
It accepts the selected-option answer as well as permitted custom text.
Multi-question bundles retain their existing header Submit action.
When custom text is forbidden, place paperclip and final-submit actions in the existing header.
Do not introduce a disabled or fake text input for choice-only questions.
Keep file state separate from option/text state so changing an answer does not discard its files.
Every single-question answer now requires the explicit final Submit action.
Option selection records a draft only. Multi-question navigation can retain its existing advance behavior.
Guard header, custom-input, keyboard, and retry submission paths with the same readiness predicate.

### Clipboard images

Handle the textarea's native paste event using `event.clipboardData` and the existing `readClipboardAttachments` helper.
Send readable image files through the same question-scoped upload hook as picker files.
Use `processFile` and current file-backed limits, not the legacy image-only size constants.
The helper already avoids processing both `files` and `items` copies of the same clipboard content.
Distinct pasted images still obey the bundle-wide count and byte limits.

Image-only paste does not call the text-change or answer-commit handlers.
It preserves the selected option, custom draft, input focus, and current question.
For mixed image/text clipboard content, attach the readable images and retain native plain-text insertion.
Plain text and multiline text retain the textarea's current behavior.
Unreadable image content uses the existing localized paste-feedback helpers.
Never fetch an image URL from clipboard HTML or request background clipboard-read permission.
Pasting does not submit or navigate the question.

Extend `ClarificationDraftProvider` to preserve attachment descriptors and in-memory upload state across responsive remounts.
Hosts without this provider need equivalent source-bundle lifetime ownership.
Do not abort or delete an upload merely because its phone/desktop renderer remounts.
Fence late results by the full source identity and clean up completed, unclaimed abandoned uploads.
Do not delete ready files after an ambiguous submit until the authoritative outcome or staged-delete refusal settles ownership.
File bytes never enter browser web storage.

The existing late-answer ordinary-message fallback must receive attachments as well as formatted text.
Preserve question grouping in that text and forward the ordered descriptor set through ordinary message admission.
This fallback is distinct from resolving a superseded question.

## Mobile contract

The entry point remains the pending-question panel in task chat, Quick Chat, Threads, Inbox, or Coordinator.
The shipped exemplars are `clarification-panel-section.tsx` and `mobile/session-mobile-layout.tsx`.
They contribute the focused question surface, disclosure behavior, source state, and available-height calculations.
The shared chat attachment helpers contribute file processing, clipboard extraction, inspection, and removal behavior.

Desktop and phone retain the existing question panel and custom-answer row.
The paperclip and Send controls remain in that row without wrapping.
The textarea uses the remaining width and retains its existing text-driven auto-growth.
Attachment count and state never introduce persistent vertical content.
Contain touch hit areas within the existing row height rather than adding button height to the row's vertical padding.
Optional keyboard hints can yield width to the textarea and required actions.

An attachment manager is a temporary secondary task.
Fine-pointer desktop uses an anchored popover. Phone or coarse pointer uses an inset bottom drawer with safe-area clearance.
That open surface owns its file-list scroll region and does not resize the underlying question panel.
Closing it restores the paperclip trigger and preserves the answer draft.
Keep image-inspection Escape handling separate from question navigation and submission.
The existing question panel remains the scroll owner for question content, with reachable inline actions when the keyboard opens.
Threads retains its existing explicit footer-scroll fallback when host space is insufficient.
Fine-pointer desktop controls retain 28px sizing. Phone and coarse-pointer controls measure at least 44px.
The phone layout uses the existing 768px breakpoint independently of pointer precision.

The [plan previews](../../../plans/question-answer-attachments/plan.md#ascii-ui-preview) define control order and failure states.
Preview labels and filenames are illustrative. Product labels use existing localization conventions.

## Failure presentation

Pending and uploading files disable all final-submit paths.
The paperclip summarizes pending or failed uploads without adding a status row.
Failed or expired uploads expose Retry and Remove inside the attachment manager while preserving answer content.
File inspection cannot select an option or trigger a submit shortcut.
Stable accessible names and one translated status region announce upload and response progress.
The live status region is visually hidden and does not change panel height.
No-file behavior remains available when uploads are unavailable.
An attachment-bearing answer fails visibly if required runtime materialization is unavailable.

## Authority and security

Task authorization and uploader ownership both apply before file admission.
Only the trusted runtime creates executor paths.
Client paths, URLs, inline data, spoofed sizes, and foreign attachment IDs cannot select file content.
Parent questions retain their separate resolution channel.
The external answer MCP endpoint retains its existing input schema and file-free behavior.
External clients can observe additive attachment descriptors in an established response.
Do not infer support for new native provider protocols from the native Codex adapter.

Existing response phase logs cover failures without file contents, custom text, names, or storage paths.
No new identifier-bearing metric labels are needed.

## Related decisions

- [File-backed prompt attachments](../../../decisions/2026-08-04-file-backed-prompt-attachments.md).
- [Current-turn clarification ownership](../../../decisions/2026-08-14-current-turn-clarification-ownership.md).
- [Response reliability](clarification-response-reliability.md).
- [Readable answer labels](user-question-answer-labels.md).

These decisions already establish storage, streaming, and response authority.
The extension preserves those boundaries, so it does not need a separate ADR.
