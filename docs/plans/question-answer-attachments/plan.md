---
created: 2026-11-03
status: in_progress
requirements:
  - REQ-TASKS-ANSWER-ATTACHMENTS-001
  - REQ-TASKS-ANSWER-ATTACHMENTS-002
  - REQ-TASKS-ANSWER-ATTACHMENTS-003
system_design:
  - ../../specs/tasks/system-design/question-answer-attachments.md
legacy_specs: []
---

# Implementation plan: Question answer attachments

## Overview

[Issue #5012](https://github.com/kdlbs/kandev/issues/5012) requests files alongside offered-option and custom-text answers.
The issue is assigned to `carlosflorencio`. The package was implemented and verified against the
current workspace. Production code, permanent tests, browser evidence, and public documentation are complete.

Implement atomic admission first, agent delivery second, shared composition third, and browser evidence with public documentation last.
This order establishes ownership and delivery before the interface can submit files.

## Confirmed gap and evidence

This is a missing capability, not a known regression against an active attachment-answer requirement.
The issue supplies the intended behavior. The tasks system owns the response lifecycle and its files.

- Go `internal/clarification/protocol/types.go` and web `lib/types/http-agents.ts` omit attachment fields from answers.
- `response_serialization.go` and `mcp/server.simplifyAnswer` copy only answer identity, selections, and text.
- `clarification-input-overlay.tsx` immediately submits a committed answer when the bundle has one question.
- File staging, canonical descriptors, authorized content reads, transaction-level claims, and streaming materialization already exist.

A temporary Go program extracted the actual `Answer` declaration and decoded an option answer with a PNG descriptor.
Its encoded result was `{"question_id":"q1","selected_options":["q1_opt1"]}`.
It reported `Attachments retained: false`. The temporary directory was removed. No repository test or production file changed.

The first permanent RED test is `TestAnswerAttachmentResponseRoundTrip` in `internal/clarification/answer_attachments_test.go`.
It must assert retention through real response serialization, not only through a copied model.
Browser RED must assert that selecting an option leaves the question pending until explicit submission.

## Scope

### In scope

- One task-owned [requirement](../../specs/tasks/requirements/question-answer-attachments.md) and [design](../../specs/tasks/system-design/question-answer-attachments.md).
- File-backed, question-scoped attachment references and atomic response admission.
- Readable image and resource files delivered with live MCP, native Codex, and detached current-turn answers.
- Explicit single-question submission and shared file controls across existing editable question hosts.
- Desktop and phone progress, preview, removal, retry, and source-identity isolation.
- A same-row paperclip with compact status and on-demand file details, without added persistent panel height.
- Clipboard-image paste in the custom-answer input, with ordinary text paste preserved.
- Existing late-answer ordinary-message fallback without file loss.
- Targeted tests and updates to the current user and protocol documentation.

### Out of scope

Parent questions, permission requests, new native provider protocols, attachment-only answers, image editing, and native image blocks are excluded.
The work adds no new database table, storage service, runtime flag, model routing, persistent Kandev child task, or unsolicited PR.
Unsubmitted question drafts retain their existing in-memory lifetime.

## Technical approach

### Admission and persistence

Extend the protocol, web answer model, serializer, validation, and winner reconstruction.
Use canonical registry data and the shared limits across the complete bundle.
Add an attachment-aware completion operation through task models, service, repository, and the backend adapter.
Keep the old no-file operation compatible.

Inside the current-turn transaction, adopt every file or none.
Bind its message ID to the exact question row.
Generate `metadata.attachments` and `metadata.response.attachments` from one canonical set.
Retain the top-level manifest during guarded restoration so current reference-aware cleanup protects retryable files.
Use the attachment-aware operation for a retained retry manifest even when a later answer has no files or is skipped.

### Delivery

Expose a bounded runtime materialization operation for an already waiting execution.
It reuses `SessionManager.materializeAttachments` without starting or steering a prompt.
Only the response winner can call it. Source turn and execution checks fence completion.

Live MCP and native Codex responses carry generated executor paths with their respective answers.
Browser responses and durable rows retain canonical IDs and display descriptors only.
Detached answers and the ordinary-message fallback pass descriptors through the existing attachment-capable prompt/message admission paths.

| Question route | Identity and transport | Result and verification | Unavailable support |
| --- | --- | --- | --- |
| Agent MCP question | Source session and current turn, backend channel, MCP result | Question-keyed metadata and readable paths. MCP schema, result, and byte tests. | Retryable error before successful delivery. |
| Native Codex question | Existing thread/turn/item ownership, `request_user_input` string arrays | Preserve option label or custom answer, then append file-reference entries. Native bridge tests. | Fail visibly without weakening custom-text policy. |
| Detached current-turn question | Claimed question rows and guarded resume | Existing prompt attachment materialization before acceptance. Detached delivery tests. | Restore only the owned current-turn claim. |
| Superseded ordinary-message fallback | Existing message admission identity | Formatted question grouping plus descriptor set. Fallback unit and browser tests. | Preserve draft and files. Never resolve the old question. |
| External answer clients and parent questions | Existing independent contracts | Retain file-free input behavior. Compatibility tests. | No new file submission API on these routes. |

### Composition and compatibility

Add a small question-scoped upload hook and paperclip attachment manager.
Reuse attachment APIs, `processFile`, `readClipboardAttachments`, previews, and existing feedback helpers.
Resolve the workspace from the source task, including hosts opened outside the selected workspace.
Extend responsive draft ownership without storing file bytes in web storage.

Remove single-question auto-submit after a selection.
Reuse the custom row's trailing Send action for single-question final submission, including selected-option answers.
Keep the existing multi-question header Submit. Choice-only questions use paperclip and Submit actions in the existing header.
Apply upload readiness to every handler and shortcut.
Preserve answer files when options or custom text change.
Update tests that intentionally assumed the previous immediate submission.

Place the paperclip to the right of the custom-answer textarea without wrapping or adding another row.
Its badge shows count, progress, or failure. File details appear only in the opened attachment manager.
Clipboard images use the picker upload path without changing selected options, existing text, focus, or question navigation.
Mixed image/text paste also retains native text insertion.

The mobile entry point remains the existing pending-question panel.
The nearest shipped exemplars are `clarification-panel-section.tsx`, `mobile/session-mobile-layout.tsx`, and the existing custom-input trailing controls.
Phone retains the same input/action row and existing question panel height.
Contain 44px action hit areas inside that row without adding their height to surrounding padding.
The opened attachment manager uses a desktop popover or phone/coarse-pointer drawer.
The manager owns its file-list scrolling. It does not resize the question panel.
Question content keeps its existing panel scroller, except the shipped Threads footer fallback.
The shared domain state remains above viewport-specific presentation.

## ASCII UI preview

The following labels identify required structure, not pixel geometry.
Copy and filenames are illustrative. Product copy must use localization keys.
These views cover AC-TASKS-ANSWER-ATTACHMENTS-001.1 through 001.4 and 003.1 through 003.8.
`[clip]` represents the paperclip icon, not a visible text label.

### UI-01: Current single-question option behavior

Entry: an active question with one bundle item. Source: `commitAnswer` in the overlay.

```text
Which layout works?
[ A ] [ B ]        -> a choice submits immediately
Other answer: [________________] [Send]
```

### UI-02: Compact desktop answer with ready files

Entry: the shared question panel after selecting A and adding evidence.

```text
+--------------------------------------------------------------+
| Question 1 of 2            [Submit answers] [Skip] [Collapse]  |
| Which layout works?                                          |
| (*) A   ( ) B                                                |
| [Or type a custom answer...              ] [clip 2] [Send]    |
| [Previous]                                          [Next]   |
+--------------------------------------------------------------+
```

The paperclip occupies the existing custom-input row. Its badge reports two attached files.
Opening it exposes file details through UI-06. No file tray, status row, or new footer enters the panel.
Desktop controls retain the shared compact sizes. Send records the current answer; final Submit sends the complete multi-question bundle once.
For one question, omit navigation and header Submit. The existing trailing Send action performs explicit final submission.

### UI-03: Compact phone answer with ready files

Entry: the same source bundle through Chat, Threads, Quick Chat, Inbox, or Coordinator.

```text
+------------------------------------+
| Question             [Skip] [Fold]  |
| Which layout works?                |
| (*) A                              |
| ( ) B                              |
| [Custom answer... ] [clip 2] [Send] |
+------------------------------------+
```

This view shows a single question with an explicit inline Send action.
The textarea takes remaining width. Controls do not wrap or add persistent height.
Phone and coarse-pointer targets measure at least 44px inside the existing row.
Multi-question bundles retain their existing header Submit and question navigation.
Image paste adds a file and updates the paperclip badge while focus remains in the input.

### UI-04: Compact empty, uploading, and failed states

Entry: the same input row in either viewport. Status does not add another line.

```text
Empty:     [Custom answer...           ] [clip]   [Send]
Uploading: [Custom answer...           ] [clip ~] [Send disabled]
Failed:    [Custom answer...           ] [clip !] [Send disabled]
Ready:     [Custom answer...           ] [clip 2] [Send]
```

The same readiness rule blocks mouse, touch, custom-input, keyboard, and response-retry submissions.
Upload details, Retry, and Remove appear in UI-06. A visually hidden live status announces progress and errors.

### UI-05: Choice-only question

Entry: a question with custom text disabled. Reuse the existing header action row.

```text
+--------------------------------------------------------------+
| Question                  [clip 2] [Submit] [Skip] [Collapse]  |
| Which layout works?                                          |
| (*) A   ( ) B                                                |
+--------------------------------------------------------------+
```

The attachment action does not enable custom text or create another row.

### UI-06: Attachment manager opened on demand

Entry: click or tap the paperclip. The underlying question panel retains its height and draft.

```text
+--------------------------------------------------------------+
| Answer files                                [Add files]      |
| [image] screenshot.png     Ready             [View] [Remove]  |
| [file]  diagnostic.zip     Failed           [Retry] [Remove]  |
+--------------------------------------------------------------+
```

Fine-pointer desktop uses an anchored popover. Phone or coarse pointer uses an inset drawer with a bounded file-list scroller.
File retry/removal preserves answer content and ready siblings.
Closing the manager restores its trigger. Closing image inspection restores the file control and draft state.

## Tests

Test names in this section are required new behavior tests.
Work-order commands also run the affected existing contract tests.

| Acceptance criteria | Evidence |
| --- | --- |
| 001.1, 001.2, 001.4, 001.6 | `clarification-input-overlay.test.tsx`, `clarification-custom-input.test.tsx`, `clarification-answer-attachment-submit.test.tsx`, and the desktop/phone Playwright suites for explicit submission and file-free compatibility. |
| 001.3, 002.3, 003.6 | `TestMaterializationUsesPostClaimDeliveryBudget`, `TestMaterializationFailureRestoresClaim`, `TestToolUserInputAppendsAnswerAttachmentPaths`, `TestResumeDetachedClarificationCarriesAnswerAttachments`, and the byte/hash proof in `question-answer-attachments.spec.ts`. |
| 001.5, 002.1 | `TestValidateOutcomeRejectsAnswerAttachmentShapes` and `TestCompleteActiveClarificationBundleWithAttachments*` with real registry rows, including mixed valid/foreign rollback and count/byte bounds. |
| 002.2 | `TestCompleteActiveClarificationBundleWithAttachmentsLosingCallerDoesNotAdopt` plus the existing exactly-once claim and race suites. |
| 002.4, 002.5 | `TestCompleteActiveClarificationBundleWithAttachmentsAdoptsCanonicalSet`, `TestRestoreActiveClarificationBundleAllowsRetryAfterDeliveryFailure`, and `TestCompleteActiveClarificationBundleWithAttachmentsHandlesMixedSiblingsWithoutFiles`. |
| 002.2, 002.6 | `TestCompleteActiveClarificationBundleWithAttachmentsPreservesMixedTerminalSibling`, `TestMaterializationRejectsSupersededConversationalTurn`, and the late ordinary-message Playwright case. |
| 003.1 through 003.8 | `use-clarification-answer-attachments.test.tsx`, `clarification-answer-attachments.test.tsx`, `clarification-custom-input.test.tsx`, and the desktop/phone scenarios in task 04. |
| 003.7 | Image-only, multiple-image, mixed-text, ordinary-text, and unreadable-image paste tests using the current clipboard helper. |
| 003.8 | Measured equal compact row height across empty/uploading/ready/failed states (assertion `<= 48px`), plus choice-only and narrow fine-pointer viewport coverage. |

## E2E tests

Add `e2e/tests/chat/question-answer-attachments.spec.ts` under `chromium` and `mobile-question-answer-attachments.spec.ts` under `mobile-chrome`.
Use the real attachment API and a mock agent that reads files before acknowledging the answer.
Echoed filenames alone do not prove delivery.

- Offered choice with custom text disabled, then PNG and resource upload, explicit submit, and matching agent-side bytes: 001.1, 001.3, 001.4.
- Custom answer with image, no-file choice/custom compatibility, and Skip: 001.2 and 001.6.
- Multi-question file mapping, upload failure/retry/removal, unresolved workspace, and delayed uploads after source changes: 001.4, 002.1, 003.2, 003.3, 003.6.
- Detached current-turn retry, accepted-history reload/download, and existing late ordinary-message fallback: 002.3 through 002.6 and 003.4.
- Shared host checks in Quick Chat, Threads, Inbox, and Coordinator: 003.1.
- Phone picker, image inspection, keyboard-open action reachability, 44px bounds, and zero document overflow: 003.4 and 003.5.
- Image paste preserves a selected option and input focus; mixed clipboard data retains text: 003.7.
- Paperclip remains on the input's right without wrapping. File state changes and manager open/close do not increase panel height: 003.8.

Existing suites that assume immediate single-question submission must use an explicit submit helper.
Task 04 lists the discovered suites and exact final commands.

## Work orders

- [x] [Task 01: Admit and retain answer files atomically](task-01-answer-file-admission.md)
- [x] [Task 02: Deliver files through question response transports](task-02-answer-file-delivery.md)
- [x] [Task 03: Compose answers with files on desktop and phone](task-03-answer-file-composition.md)
- [x] [Task 04: Prove browser workflows and document the feature](task-04-answer-file-e2e-and-docs.md)

Execution order: 01 -> 02 -> 03 -> 04. Every work order is sequential.
Implementation followed that order in the primary session.

## Verification results

The source-derived temporary reproduction completed successfully and showed attachment loss.
The GitHub assignment was verified against the live issue.

Package checks completed on 2026-11-03:

- `python3 scripts/list-docs.py validate`: passed, with 425 decisions and 1893 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Repository `validateCoverage` preflight: `covered`, four work orders, four accepted reference sets, no errors.
  Its input included the seven package files and a planned protocol change as the coverage trigger.
  The protocol file itself remains unchanged.
- Catalog discovery: both new task specifications appear under `question-answer-attachments`.
- Local Markdown link check: no broken targets across the seven package files.
- `git diff --check -- docs/plans/question-answer-attachments docs/specs/tasks`: passed.
- `git status --short --untracked-files=all`: seven new package files only, all unstaged and uncommitted.

The user requested the compact same-row paperclip and clipboard-image paste after the initial package checks.
The revised requirements define 20 acceptance criteria, including clipboard-image paste and unchanged
persistent panel height. The implementation kept that same-row composition and adds no persistent
vertical attachment area.
Implementation added the canonical answer-file contract, attachment-aware atomic claim and retry
manifests, live execution materialization with bounded executor paths, MCP and native Codex delivery,
detached resume delivery, the compact paperclip composition, clipboard-image paste, historical display,
and the late ordinary-message fallback.

Verification completed on 2026-11-03:

- `go build ./...` passed.
- Backend tests passed for clarification, SQLite repository, task service, MCP server and handlers,
  agentctl API, Codex app-server transport, orchestrator, backendapp, lifecycle, and mock-agent.
- Focused race tests passed for materialization, execution replacement, mixed attachment claims, and
  canonical turn authority.
- `go run ./cmd/sqlguard ./internal` passed and `storeconformance` passed.
- `scripts/lint-go-changed` reported 0 issues for the changed Go package set.
- Web typecheck, scoped ESLint, `i18n:check`, and `i18n:ratchet` passed.
- Vitest passed 178 tests across the affected clarification, attachment, draft, and late-answer suites.
- Desktop Playwright passed `question-answer-attachments.spec.ts` (6 scenarios, including
  multi-question association, custom-answer files, and a narrow fine-pointer viewport), the listed clarification, inbox, Threads,
  image-preview, and Coordinator suites, and an added Inbox source-scope attachment case. The final
  focused rerun passed all 6 attachment scenarios plus choice-only and submit-failure regressions.
  One unchanged auto-submit expectation in the existing clarification suite was fixed and its test
  then passed.
- Mobile Playwright passed `mobile-question-answer-attachments.spec.ts` (3 scenarios: choice-only,
  compact row with touch targets, and failed-upload retry) plus the listed mobile clarification,
  inbox, image-preview, and Threads suites. Runs are sequential; overlapping builds are not used.
- The mock agent read materialized files and emitted their real byte length and SHA-256 digest,
  proving delivered bytes rather than echoed filenames.
- Public documentation validation passed 63 tests and 54 pages. Documentation catalogs validated
  425 decisions and 1893 specifications. Specification lint passed 36 tests and all files.
- `git diff --check` passed.

Coverage limitations: no real PostgreSQL DSN was available, so the new attachment tests ran on
SQLite only; the repository uses the existing dialect-neutral paths and PostgreSQL conformance
suite. Remote SSH, Kubernetes, and live native-provider runs were not exercised; transport behavior
is covered through the existing materialization client seam. Detached resume descriptor mapping is
unit-tested, but a full detached prompt resume carrying files was not exercised in Playwright.
Quick Chat, Threads, and Coordinator attachment uploads were not separately scripted; they share the
same panel, upload hook, and source-task resolver as the exercised task-chat and Inbox cases.
Public documentation does not claim native image result blocks.

## Review remediation (2026-11-03)

A source review found eight issues in the first implementation. All are fixed:

1. File processing now reserves a pending placeholder synchronously, so submission is blocked before
   `processFile` settles. Every asynchronous phase captures the source store and generation, rejects
   stale completions, rechecks bundle limits after processing, and deletes staged uploads that finish
   after removal, clearing, source replacement, or host loss.
2. A source-scoped module store now owns upload state above hook instances. Remounts and simultaneous
   hosts subscribe to the same store; completion, retry, and removal update every live consumer, and
   one file has one upload promise.
3. Materialization now runs in a bounded post-claim delivery context shared with response delivery.
   It survives caller cancellation and no longer inherits the five-second pre-claim deadline.
4. The post-transfer turn check now calls a repository method built from `currentTurnAuthority`, so it
   ignores lifecycle-only turns and ranks open conversational turns exactly like the claim.
5. Attachment admission now processes only newly claimed question rows. Terminal siblings and their
   canonical files are preserved, including mixed terminal/pending bundles with and without files.
6. The lifecycle materializer captures the execution identity and revalidates it after streaming, so
   a replaced execution can never supply paths for an otherwise valid answer.
7. The custom-answer row fits 44px phone controls inside its existing height (`py-0` row with the
   textarea owning vertical padding and a non-wrapping placeholder), preserving multiline growth.
8. The attachment manager uses phone presentation from `isMobile || coarse pointer`, and Add, Retry,
   Remove, Inspect, and the trigger all provide at least 44px phone targets.

New regressions cover deferred processing with an immediate submit, source swaps before processing and
workspace resolution, overlapping additions, and abandoned uploads; remount and two-host completion;
post-claim transfer budget and restore-on-transfer-failure; canonical turn-authority ordering;
mixed terminal/pending attachment claims; execution replacement during transfer; and desktop/phone
compact geometry and manager touch targets.

## Coverage gates

The implementation is complete, but the following verification gates remain open and are not claimed
as passing evidence:

- PostgreSQL: no disposable DSN was available for the attachment-aware claim, lock-order, and
  mixed-bundle repository tests. SQLite and dialect-neutral tests pass; PostgreSQL concurrency remains
  to be exercised in CI or a dedicated environment.
- Detached resume: descriptor construction is unit-tested, but a full detached prompt resume carrying
  files was not exercised in Playwright.
- Shared hosts: Quick Chat, Threads, and Coordinator attachment uploads were not separately scripted;
  they share the panel, upload hook, and source-task resolver exercised through task chat and Inbox.
- Native providers: native Codex answer-array delivery is unit-tested; no live native-provider run was
  performed, and native image result blocks remain out of scope.

## Risks

- Single-question confirmation changes an existing interaction and requires updates to several browser tests.
- Touch controls and badges must fit within the current input row without wrapping or adding vertical padding.
- Materialization must finish inside the existing delivery budget, including for large files and remote executors.
- Retry restoration must protect its own manifest without changing terminal siblings or adopting another responder's files.
- Native Codex uses text references, not native image result blocks. Actual image inspection depends on the agent's file-reading tools.
- Remote transport tests can prove byte transfer through the existing client seam. They do not establish live coverage of every executor provider.

The package reuses existing architecture decisions. It introduces no unresolved product choice or additional approval gate.
