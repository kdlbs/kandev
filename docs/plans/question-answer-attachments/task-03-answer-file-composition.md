---
id: "03-answer-file-composition"
title: "Compose answers with files on desktop and phone"
status: done
wave: 3
depends_on:
  - "02-answer-file-delivery"
plan: "plan.md"
requirements:
  - REQ-TASKS-ANSWER-ATTACHMENTS-001
  - REQ-TASKS-ANSWER-ATTACHMENTS-002
  - REQ-TASKS-ANSWER-ATTACHMENTS-003
acceptance_criteria:
  - AC-TASKS-ANSWER-ATTACHMENTS-001.1
  - AC-TASKS-ANSWER-ATTACHMENTS-001.2
  - AC-TASKS-ANSWER-ATTACHMENTS-001.4
  - AC-TASKS-ANSWER-ATTACHMENTS-001.6
  - AC-TASKS-ANSWER-ATTACHMENTS-002.3
  - AC-TASKS-ANSWER-ATTACHMENTS-002.5
  - AC-TASKS-ANSWER-ATTACHMENTS-002.6
  - AC-TASKS-ANSWER-ATTACHMENTS-003.1
  - AC-TASKS-ANSWER-ATTACHMENTS-003.2
  - AC-TASKS-ANSWER-ATTACHMENTS-003.3
  - AC-TASKS-ANSWER-ATTACHMENTS-003.4
  - AC-TASKS-ANSWER-ATTACHMENTS-003.5
  - AC-TASKS-ANSWER-ATTACHMENTS-003.6
  - AC-TASKS-ANSWER-ATTACHMENTS-003.7
  - AC-TASKS-ANSWER-ATTACHMENTS-003.8
system_design:
  - ../../specs/tasks/system-design/question-answer-attachments.md
---

# Task 03: Compose answers with files on desktop and phone

## Summary

Expose a paperclip on the right of the existing custom-answer row without increasing the question panel's persistent height.
Support clipboard-image paste through that textarea and keep file details in an on-demand attachment manager.
Require explicit final submission so users can select an option before they attach evidence.

## In scope

- Extend web answer types and authoritative response parsing with canonical file descriptors.
- Add a focused upload hook and same-row paperclip using current attachment APIs, processing, previews, and localized feedback.
- Show count, uploading, and failure states on the paperclip. Keep file rows, Add files, inspection, Retry, and Remove in its popover/drawer.
- Handle textarea paste with `readClipboardAttachments`. Upload readable images through the same hook as picker files.
- Preserve selected options, existing text, question position, and focus on image-only paste. Retain native text insertion for ordinary and mixed paste.
- Report unreadable image content through existing localized feedback. Do not fetch clipboard HTML image URLs or request clipboard-read permission.
- Resolve upload scope from the question's source task/session, including hosts outside the selected workspace.
- Keep attachments independent from selected option and custom text.
- Preserve source-scoped drafts across navigation, collapse, and responsive remounts.
- Fence and clean up abandoned upload results without deleting accepted or ambiguous-submit files.
- Replace single-question auto-submit with the custom row's existing trailing Send action for both selected and custom answers.
- Keep multi-question header Submit. For choice-only questions, place paperclip and final-submit actions in the existing header without a new input row.
- Apply the same readiness predicate to every final-submit handler.
- Preserve phone Enter-as-newline behavior. A deliberate final keyboard shortcut remains available only when the full answer is ready.
- Show pending, uploading, ready, expired, and failed file states with Retry/Remove.
- Keep the textarea and actions on one line at phone and desktop widths. File states must not add an inline tray, status row, or footer.
- Preserve existing text-driven textarea growth. Fit 44px phone/coarse-pointer hit areas inside the existing row without cumulative vertical padding.
- Restore server-retained files after retryable materialization failure.
- Render accepted files in historical answer context, using authorized content URLs rather than durable executor paths.
- Extend late-answer snapshots and ordinary-message admission to retain all files and their question grouping.
- Cover task chat, Quick Chat, Threads, Inbox, and Coordinator through the shared panel.
- Add or reuse translation keys in English, Portuguese, Simplified Chinese, both Traditional Chinese catalogs, Japanese, and Korean.

## Out of scope

New upload endpoints, a second composer implementation, persisted unsent drafts across reload, and provider transport changes are excluded.

## Acceptance

1. The paperclip stays right of the textarea without adding persistent panel height across file states. Choice-only questions reuse existing header space.
2. Picker and image paste use one upload path and preserve answer identity, normal text behavior, focus, and ready siblings through failures and remounts.
3. Every shared host exposes localized, readiness-guarded submission on desktop and phone. History and late-message fallback retain the correct files.

## ASCII UI preview

Use [UI-02 through UI-06 in the plan](plan.md#ascii-ui-preview).
They map to AC-TASKS-ANSWER-ATTACHMENTS-001.1, 001.2, 001.4, and 003.1 through 003.8.
This excerpt preserves those view labels and the required action order.

```text
UI-02: Compact desktop, ready
Which layout works?    (*) A   ( ) B
[Or type a custom answer...             ] [clip 2] [Send]

UI-03: Compact phone, ready
+------------------------------------+
| Which layout works?                |
| (*) A                              |
| ( ) B                              |
| [Custom answer... ] [clip 2] [Send] |
+------------------------------------+

UI-04: States in the same row, either viewport
[Custom answer... ] [clip]   [Send]
[Custom answer... ] [clip ~] [Send disabled]
[Custom answer... ] [clip !] [Send disabled]

UI-05: Choice-only header, no custom-input row
Question               [clip 2] [Submit] [Skip] [Collapse]

UI-06: File details opened on demand
Answer files                                [Add files]
screenshot.png   Ready                   [View] [Remove]
diagnostic.zip   Failed                 [Retry] [Remove]
```

`[clip]` represents the icon. Its count and status occupy the same action space.
Required structure: same-row paperclip, on-demand file details, separate selection/inspection, and explicit submission.
Spacing and labels are illustrative.
Keep the existing bounded panel body as the scroll owner, with the documented Threads footer fallback.
The desktop popover or phone/coarse-pointer drawer owns its own file-list scroller only while open.
It must not resize the underlying question panel. Closing it restores its trigger and answer state.
Phone/coarse-pointer hit targets measure at least 44px. Fine-pointer desktop controls retain normal compact sizing.
The source-bundle state must survive a desktop -> phone -> desktop visit.

## TDD and verification

First assert that choosing an offered option does not call the submit transport.
The current auto-submit path must fail that assertion before production changes.
Add `use-clarification-answer-attachments.test.tsx` beside the new hook.
Cover every submit entry point while one file remains pending or failed.
Use delayed upload promises to exercise question, session, task, and viewport transitions.
Test a valid ready sibling with a failed upload, not only all-ready or all-failed sets.
Add image-only paste with an option already selected, mixed text/image paste, multiple images, ordinary text, and unreadable image content.
Assert that paste never submits or navigates. Only actual inserted text can activate the custom answer.
Measure unchanged row and panel height across empty, uploading, ready, and failed attachment states for the same answer text.
Cover just below/above 768px and a narrow fine-pointer viewport. File-state indicators must never wrap onto a second row.

If this worktree lacks dependencies, run once from the repository root:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Run from the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/use-clarification-answer-attachments.test.tsx components/task/chat/clarification-answer-attachments.test.tsx components/task/chat/clipboard-attachments.test.ts components/task/chat/clarification-input-overlay.test.tsx components/task/chat/clarification-custom-input.test.tsx components/task/chat/clarification-draft-context.test.tsx components/task/chat/clarification-panel-section.test.tsx components/task/chat/messages/clarification-request-message.test.tsx hooks/domains/session/use-clarification-group.test.ts hooks/domains/session/use-clarification-group.regressions.test.ts hooks/domains/session/use-clarification-group.timeout.test.ts lib/clarification/late-clarification-message.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

Run scoped ESLint against the final changed TS/TSX file list.
When implementation changes test locations, update this command before marking the work order done.
Task 04 supplies the rendered desktop and phone evidence.

## Files likely touched

- `apps/web/lib/types/http-agents.ts`
- `apps/web/hooks/domains/session/use-clarification-group.ts` and its existing test files
- `apps/web/components/task/chat/use-clarification-answer-attachments.ts` (new) and adjacent tests
- `apps/web/components/task/chat/clarification-answer-attachments.tsx` (new) and adjacent interaction tests
- `apps/web/components/task/chat/clipboard-attachments.ts` and tests, only if the shared helper needs an independently covered change
- `apps/web/components/task/chat/clarification-input-overlay.tsx` and tests
- `apps/web/components/task/chat/clarification-overlay-parts.tsx`
- `apps/web/components/task/chat/clarification-overlay-header.tsx`
- `apps/web/components/task/chat/clarification-panel-section.tsx` and tests
- `apps/web/components/task/chat/clarification-draft-context.tsx` and tests
- `apps/web/components/task/chat/messages/clarification-request-message.tsx` and tests
- `apps/web/hooks/use-late-clarification-message.ts`
- `apps/web/lib/clarification/late-clarification-message.ts` and tests
- `apps/web/app/coordinator/components/question-answer.tsx`, only if source-scope wiring requires a prop
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/` affected catalogs

Keep shared attachment helpers unchanged unless reuse requires a small, covered extraction.
The new domain hook must not depend on the main chat composer's local-storage behavior.

## Dependencies

Task 02 supplies attachment-bearing answer delivery and safe failure outcomes.

## Risks

Option/text handlers currently replace whole answer objects. They must preserve the question's file set.
A component unmount is not proof that its source-bundle draft was abandoned.
Existing tests and hosts assume single-question automatic submission.
Wrapping controls, count badges, and touch-target padding can increase the row height despite fixed-size icons.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/question-answer-attachments.md).
- [Composition, Mobile contract, and Failure presentation](../../specs/tasks/system-design/question-answer-attachments.md).
- [Combined previews](plan.md#ascii-ui-preview).
- Existing `use-chat-input-state.ts`, `file-attachment.ts`, `clipboard-attachments.ts`, file previews, and draft-remount tests.
- `/mobile-parity`, `/tdd`, and scoped web/chat guidance.

## Results

Implemented the question-scoped upload hook and module-level in-memory draft cache, compact same-row
paperclip with count/progress/failure badge, desktop popover and coarse-pointer drawer manager,
picker and clipboard-image upload, explicit single-question Send, choice-only header actions,
historical file links, late-answer attachments, and localized catalogs in all seven locales.

Verification passed: web typecheck, scoped ESLint, `i18n:check`, `i18n:ratchet`, and 178 Vitest tests
across overlay, custom input, draft context, panel, history, hook, clipboard, and late-answer suites.
Review remediation replaced the per-hook cache with a source-scoped reactive store: processing is
reserved synchronously, stale completions are fenced by source and generation, removals delete
abandoned staged uploads, eviction releases staged uploads, and remounts/two hosts share completion. Phone controls use
`isMobile || coarse pointer`, all manager actions meet 44px, and the custom row stays at approximately 46px with
unchanged text. No file bytes enter web storage.
