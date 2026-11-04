---
status: active
system: tasks
created: 2026-11-03
owners:
  - kandev
---

# Question answer attachments

## Overview

Users need to provide evidence with an answer to an agent question. An answer can select an offered option or contain permitted custom text.
The tasks system owns the question identity, response claim, attachment ownership, and agent delivery. Its contract includes desktop and phone interactions.

Source: [issue #5012](https://github.com/kdlbs/kandev/issues/5012).
The [system design](../system-design/question-answer-attachments.md) defines the technical contract.
The [implementation plan](../../../plans/question-answer-attachments/plan.md) records delivery work.

## Terms

- **Bundle:** The questions in one operator-facing agent request.
- **Answer attachment:** A user-provided file associated with one question's answer.
- **Ready file:** A completed upload with a valid attachment reference.
- **Submitted answer:** The answer and its ordered attachment set at final submission.

## Requirements

### REQ-TASKS-ANSWER-ATTACHMENTS-001: Answers with supporting files

**Intent:** Deliver the user's evidence with the answer that explains it.

#### Acceptance criteria

- **AC-TASKS-ANSWER-ATTACHMENTS-001.1:** Users shall attach files to an offered-option answer, including a question that disables custom text.
- **AC-TASKS-ANSWER-ATTACHMENTS-001.2:** Users shall attach files to a permitted custom-text answer. Attachments shall not enable otherwise prohibited custom text.
- **AC-TASKS-ANSWER-ATTACHMENTS-001.3:** Image and non-image files shall reach the agent with the corresponding answer. Every delivered reference shall identify readable file bytes in the answering session's execution.
- **AC-TASKS-ANSWER-ATTACHMENTS-001.4:** Files shall remain associated with their question IDs through bundle navigation and submission. Selecting an option shall leave time to attach files before explicit final submission, including single-question bundles.
- **AC-TASKS-ANSWER-ATTACHMENTS-001.5:** One bundle submission shall accept at most ten files, with 100 MiB per file and 100 MiB total. Exact byte boundaries shall succeed. Excess counts, excess bytes, and duplicate attachment IDs shall fail before answer acceptance.
- **AC-TASKS-ANSWER-ATTACHMENTS-001.6:** Answers without files shall retain their selection, label, custom-text, rejection, and question-identity behavior. Single-question answers shall use the explicit confirmation in criterion 001.4. Attachments alone shall not complete an otherwise unanswered question.

### REQ-TASKS-ANSWER-ATTACHMENTS-002: Authorized and recoverable answer delivery

**Intent:** Preserve response authority and file availability across races and failures.

#### Acceptance criteria

- **AC-TASKS-ANSWER-ATTACHMENTS-002.1:** An attachment shall require authorization for its uploader, workspace, task, and answering session. An invalid, expired, missing, or foreign file shall prevent the entire new answer from acceptance or agent delivery.
- **AC-TASKS-ANSWER-ATTACHMENTS-002.2:** Only the winning current-turn response shall adopt attachments and deliver an answer. Concurrent responders and exact retries shall observe the established outcome without duplicate agent delivery or adoption of a losing file set.
- **AC-TASKS-ANSWER-ATTACHMENTS-002.3:** An upload or file-delivery failure shall preserve the answer and usable files for retry or removal. No answer shall reach the agent with a reference to unreadable bytes. Existing bounded response outcomes shall remain in effect.
- **AC-TASKS-ANSWER-ATTACHMENTS-002.4:** Accepted answer-file associations shall survive reload and backend restart. History and authorized downloads shall retain display metadata. Browser responses and durable records shall contain neither file bytes nor backend filesystem paths.
- **AC-TASKS-ANSWER-ATTACHMENTS-002.5:** Cleanup shall retain every file referenced by an accepted answer or its retryable delivery claim. Removing an unsubmitted file shall release its staged upload on a best-effort basis. Losing or superseded drafts shall not delete winning files.
- **AC-TASKS-ANSWER-ATTACHMENTS-002.6:** A detached current-turn answer shall preserve files through the existing resume path. A superseded answer shall not revive its question. When the existing ordinary-message fallback applies, it shall retain the answer-file association.

### REQ-TASKS-ANSWER-ATTACHMENTS-003: Accessible answer composition

**Intent:** Provide the same complete answer workflow on desktop and phone.

#### Acceptance criteria

- **AC-TASKS-ANSWER-ATTACHMENTS-003.1:** Every existing editable operator-question surface shall expose file selection and explicit final submission. This includes task chat, Quick Chat, Threads, Inbox, and Coordinator. File controls shall remain available without hover.
- **AC-TASKS-ANSWER-ATTACHMENTS-003.2:** A compact attachment indicator shall show count and upload or failure status. On demand, users shall inspect each file's status and retry or remove it without losing choices, text, or ready siblings. All submission paths shall block while any selected file is unready.
- **AC-TASKS-ANSWER-ATTACHMENTS-003.3:** Bundle navigation, collapse, and desktop-to-phone remounts shall preserve question-scoped drafts. A different task, session, or bundle shall not inherit attachments or late upload results.
- **AC-TASKS-ANSWER-ATTACHMENTS-003.4:** Users shall inspect image previews and file names without answering or submitting. Closing inspection shall restore focus and the answer draft. Historical file reads shall require the existing task authorization.
- **AC-TASKS-ANSWER-ATTACHMENTS-003.5:** Application labels and status messages shall be localized and accessible. Phone controls shall have at least 44px hit targets, safe-area clearance, one panel scroll owner, and no document horizontal overflow.
- **AC-TASKS-ANSWER-ATTACHMENTS-003.6:** Unavailable upload or materialization support shall produce an actionable failure. The system shall not silently omit a selected file or report an attachment-bearing answer as successfully delivered.
- **AC-TASKS-ANSWER-ATTACHMENTS-003.7:** Pasting readable clipboard images into the custom-answer input shall attach them to the current question through the normal upload flow. Image-only paste shall preserve the selected option and existing text. Ordinary and mixed text paste shall retain their normal text behavior. Unreadable image content shall show localized feedback without erasing the draft or fetching an external image URL.
- **AC-TASKS-ANSWER-ATTACHMENTS-003.8:** The attachment action shall occupy the right side of the existing custom-answer row. File count, upload progress, readiness, and errors shall add no persistent row or increase the panel height for unchanged answer text. File details shall open on demand. Choice-only questions shall expose attachment actions within existing question chrome without introducing a custom-text input or another row.

## Related contracts

- [Prompt attachments](prompt-attachments.md) owns shared upload limits and storage behavior.
- [Active clarification lifecycle](clarification-active-lifecycle.md) owns current-turn authority and response claims.
- [Response reliability](clarification-response-reliability.md) owns deadlines and retry outcomes.
- [Answer labels](user-question-answer-labels.md) owns the selected option's readable label.
- [Question image previews](question-image-previews.md) covers agent-provided images, as requested in issue #4748.

## Out of scope

Attachment-only answers, image editing, annotations, external URLs, native image blocks in tool results, and larger file limits are excluded.
Parent-agent questions, permission requests, and new native provider question protocols are excluded.
The package does not add a separate chat message during a live question wait or change workflow completion authority.
Unsubmitted drafts retain the existing in-memory lifetime. This package does not add draft persistence across a full page reload.
