---
id: "04-answer-file-e2e-and-docs"
title: "Prove browser workflows and document the feature"
status: in_progress
wave: 4
depends_on:
  - "03-answer-file-composition"
plan: "plan.md"
requirements:
  - REQ-TASKS-ANSWER-ATTACHMENTS-001
  - REQ-TASKS-ANSWER-ATTACHMENTS-002
  - REQ-TASKS-ANSWER-ATTACHMENTS-003
acceptance_criteria:
  - AC-TASKS-ANSWER-ATTACHMENTS-001.1
  - AC-TASKS-ANSWER-ATTACHMENTS-001.2
  - AC-TASKS-ANSWER-ATTACHMENTS-001.3
  - AC-TASKS-ANSWER-ATTACHMENTS-001.4
  - AC-TASKS-ANSWER-ATTACHMENTS-001.5
  - AC-TASKS-ANSWER-ATTACHMENTS-001.6
  - AC-TASKS-ANSWER-ATTACHMENTS-002.1
  - AC-TASKS-ANSWER-ATTACHMENTS-002.2
  - AC-TASKS-ANSWER-ATTACHMENTS-002.3
  - AC-TASKS-ANSWER-ATTACHMENTS-002.4
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

# Task 04: Prove browser workflows and document the feature

## Summary

Prove the answer-and-file workflow through real browser uploads and an agent fixture that checks actual file bytes.
Update existing single-question expectations and publish the user/protocol documentation with the implementation.

## In scope

- Add a deterministic clarification-with-attachments mock scenario and focused tests for its file-read acknowledgements.
- Add desktop and phone Playwright suites for offered choices, custom text, PNG plus resource evidence, and no-file compatibility.
- Cover multi-question association, failed upload retry/removal, invalid references, retained history, and detached or ordinary-message fallback.
- Cover source-task upload scope from task chat, Quick Chat, Threads, Inbox, and Coordinator.
- Cover desktop -> phone -> desktop draft retention without cross-bundle uploads.
- Paste image clipboard content into the custom input with an option already selected. Prove the image reaches the agent with that option.
- Cover mixed text/images, ordinary multiline text, unreadable image feedback, and multiple pasted images through the shared upload limits.
- Open file details from the paperclip through the desktop popover and phone drawer. Retry/remove there without changing the underlying panel height.
- Measure the paperclip's right-of-input position and unchanged row/panel height across empty, uploading, ready, and failed states.
- Cover phone widths below 768px, a narrow fine pointer, and breakpoint neighbors. Include choice-only questions with no extra input row.
- Measure phone hit targets, keyboard-open action reachability, panel containment, inspection focus return, and zero document overflow.
- Update all discovered immediate-submit test callers to make final submission explicit.
- Update public user and protocol documentation after behavior exists.
- Record every task-defined check and reconcile requirement/design lifecycle only after successful implementation.

## Out of scope

A broad local QA/review gate, a full E2E suite, feature videos, container-provider certification, commits, pushes, and PR creation are excluded.

## Acceptance

1. Desktop and phone users submit picker or pasted images with either answer form. Agent-side byte checks establish question association before acknowledgement.
2. Targeted scenarios prove recovery, source isolation, normal text paste, and compact same-row geometry across all file states and affected hosts.
3. The changed existing suites pass. Public documentation and the durable package accurately describe verified behavior and remaining coverage limits.

## TDD and verification

The new browser scenario's RED boundary is either retained pending state after selection or the missing file in the agent's returned answer.
Missing selectors alone do not establish behavioral RED.
Task 03's component RED proves the changed submission interaction before production changes.
Use the real upload endpoint, and compare fixture file hashes or bytes rather than echoed filenames.
Use repository causal waits and worker-scoped cleanup.

Add these suites:

- `e2e/tests/chat/question-answer-attachments.spec.ts`, project `chromium`.
- `e2e/tests/chat/mobile-question-answer-attachments.spec.ts`, project `mobile-chrome`.

The [plan's E2E matrix](plan.md#e2e-tests) defines their scenarios and acceptance mapping.
Native Codex transport evidence remains in task 02's real protocol tests.
The ACP mock browser scenario does not establish live native Codex execution.

Run each managed command sequentially from the repository root:

```bash
(cd apps/backend && go test -trimpath ./cmd/mock-agent -run 'Clarification|AnswerAttachment' -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/question-answer-attachments.spec.ts tests/chat/clarification.spec.ts tests/chat/clarification-choice-only.spec.ts tests/chat/clarification-submit-failure.spec.ts tests/chat/clarification-resize.spec.ts tests/chat/clarification-context-hosts.spec.ts tests/chat/clarification-lifecycle-turn-shadow.spec.ts tests/chat/question-image-previews.spec.ts tests/chat/needs-you-inbox-answer.spec.ts tests/task/threads-composer-disclosure.spec.ts tests/coordinator/answer-in-place.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-question-answer-attachments.spec.ts tests/chat/mobile-clarification.spec.ts tests/chat/mobile-needs-you-inbox-answer.spec.ts tests/chat/mobile-question-image-previews.spec.ts tests/task/mobile-threads-composer-disclosure.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed runner rebuilds the web, backend, and mock artifacts.
Keep its default resource controls and one worker per shard. Do not overlap desktop and phone runs.
Confirm the intended suites were discovered in their matching projects.
Add any further changed test file to the exact final command before marking Results complete.

## Files likely touched

- `apps/backend/cmd/mock-agent/scenarios_clarification.go`
- `apps/backend/cmd/mock-agent/scenarios.go`
- Adjacent mock-agent attachment scenario tests (new)
- `apps/web/e2e/tests/chat/question-answer-attachments.spec.ts` (new)
- `apps/web/e2e/tests/chat/mobile-question-answer-attachments.spec.ts` (new)
- `apps/web/e2e/helpers/clarification.ts`
- `apps/web/e2e/pages/session-page.ts`, if a shared explicit-submit helper belongs there
- The existing E2E files named in the commands
- `docs/public/sessions-and-review.md`, how-to section for answering questions and file recovery
- `docs/public/agent-communication.md`, reference for question-associated attachment paths and native question behavior
- `docs/public/automation-and-mcp.md`, only its question-result description if it duplicates the affected contract
- `docs/specs/tasks/requirements/question-answer-attachments.md`
- `docs/specs/tasks/system-design/question-answer-attachments.md`
- This plan and its work-order Results sections

Public documentation must explain the paperclip, clipboard-image paste, on-demand file details, file limits, explicit submission, failures, and path delivery.
It must not claim native image result blocks.
Search the root README and screenshot catalog for affected terminology. Change them only when their content becomes inaccurate.

## Dependencies

Tasks 01 through 03 provide the complete behavior. Install web dependencies once if this worktree lacks them.

## Risks

An acknowledgement that contains only filenames can pass while bytes remain unavailable.
Mock ACP evidence does not prove native provider support or every remote executor.
Existing clarification tests outside the new suites can still assume immediate single-question submission.
Clipboard support depends on browser-provided readable image files. Test unavailable image content without fetching an external URL.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/question-answer-attachments.md).
- [System design](../../specs/tasks/system-design/question-answer-attachments.md).
- [Plan scenario matrix and UI previews](plan.md).
- `/e2e`, `/mobile-parity`, `/docs-maintainer`, and `apps/backend/cmd/mock-agent/AGENTS.md`.
- Existing clarification and attachment browser fixtures.

## Results

Added the `clarification-attachments` and `clarification-attachments-choice-only` mock scenarios,
which read delivered files and emit their real byte length and SHA-256 digest. Added desktop and phone
Playwright suites covering choice-only files, custom-answer files, pasted images, failed-upload retry,
explicit submit, 44px touch targets, right-of-input placement, and zero document overflow. Updated
existing clarification suites to explicit submission and documented the feature.

Verification passed: the mock-agent Go test `TestClarificationAttachmentProof*`; the six new desktop
Playwright scenarios; the listed desktop regressions; the added Inbox source-scope attachment case;
the three new phone attachment scenarios, including failed-upload retry; and the listed mobile
regressions. Phone coverage asserts equal compact row height `<= 48px` across empty/uploading/ready/
failed states and Add/Retry/Remove/Inspect targets at or above 44px; it does not assert an exact pixel
value. Public documentation validation passed 63 tests and 54 pages. `node --test
scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs`, `python3
scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`, `python3
scripts/lint-spec-files.py --all`, and `git diff --check` passed.
Coverage limits: no live SSH or Kubernetes executor runs, no native image result blocks, no PostgreSQL
DSN, no full detached resume with files, and no separately scripted Quick Chat, Threads, or Coordinator
attachment uploads.
