---
status: current
system: tasks
requirements:
  - REQ-TASKS-COMMENT-FILES-001
---

# Comment attachment references

## Purpose and boundaries

The [requirement](../requirements/comment-attachment-references.md) extends task message attachment delivery. The clarification controls, claim transaction and MCP text result already implement [answer attachment delivery](question-answer-attachments.md). Keep the two submission routes independent.

## Comment composition

Extend supported variants in web `lib/state/slices/comments/types.ts` with ordered optional descriptors. Treat absent arrays as empty. Use `uploadAttachment` with path delivery. Bind each upload completion to its originating draft and scope. Discard late results after removal or replacement. Keep local descriptors with the existing session draft. On expired uploads, offer file replacement.

Use the selection comment composer, walkthrough step feedback and Review entry points. Place a labelled paperclip beside the existing action. Show count there. Open a file list with names, upload states, retry and remove actions. On phones, use the existing sheet with a reachable footer and 44 px controls. Keep keyboard focus within the open surface and return it to the trigger on close. Translate labels and status announcements.

## Task-owned plan files

Extend plan comment mutations and snapshots with attachment IDs and canonical display metadata. Store an ordered comment-to-attachment relation. On create or version-checked edit, validate uploader, workspace, task eligibility and limits in the comment transaction. Retain these files independently of staging expiry. Removing a comment or replacing its file set releases only unreferenced files. Do not bind a pending plan file to the editing session.

## Message admission and delivery

Carry each submitted session comment ID and its ordered attachment IDs alongside its existing formatted content. Expand plan IDs from authoritative rows at admission. Validate by source, resolve canonical descriptors, and enforce existing aggregate message limits across comments and ordinary composer attachments. Ignore caller-supplied paths and reject unsupported delivery modes. Persist the comment-to-file mapping and transfer retention to the message or queue item in the same transaction as plan consumption. Keep local Review drafts until transport acknowledgement.

Carry the mapping through queued dispatch and retry. Each comment-owned message descriptor has a `comment_id` that groups the files for delivery. Session comments use the first file ID so legacy comment IDs can retain file path characters. Plan comments use the saved comment ID. Each comment contains a stand-alone `<!-- kandev-comment-files:ID -->` marker. Validate that each mapped ID has exactly one marker. Leave unrelated text intact. After materialization, `v1.RenderCommentFiles` replaces each mapped marker with the quoted display names and execution paths for that comment. It removes those descriptors from the provider attachment list. ACP uses `SessionManager.materializeAttachments`. Local terminal sessions write the file into the receiving workspace. SSH terminal sessions use authenticated agentctl on the remote host. Preserve ordinary attachment delivery modes separately. An unresolved ID fails dispatch before the provider receives text. Build paths anew for the destination execution. Store only IDs and display metadata in browser drafts and durable feedback records.

## Verification

Use the [work orders](../../../plans/comment-attachment-references/plan.md) for ownership, delivery, compatibility and mobile scenarios.
