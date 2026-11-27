---
created: 2026-09-26
status: implemented
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
legacy_specs: []
---

# Implementation Plan: Composer attachment scope

## Overview

Repair task-session image uploads when the task is absent from Kanban collections.
One sequential work order covers scope resolution, feedback, and browser proof.
Source: [issue #3980](https://github.com/kdlbs/kandev/issues/3980).
Investigation baseline: `dfce4dac05809c0fcec156166f5479f14b0cdb76`.

## Evidence and root cause

A read-only execution of `resolveComposerWorkspaceId` supplied an Office task
with workspace `office-workspace`, empty snapshots, and an unrelated active board.
The result was `null`. The resolver does not consume Office task identity.
Both shared-composer entry points use this resolver.

`use-chat-input-state.ts` skips uploads without `workspaceId`. Its submission
and pending-upload guards also depend on that value. A selected file can thus
escape the upload gate as inline data, or as an empty descriptor for large files.
This violates the existing file-backed web-client contract.

The issue's surface inventory does not match this revision:

| Surface                   | Current source evidence                             | Package treatment                         |
| ------------------------- | --------------------------------------------------- | ----------------------------------------- |
| Office advanced task Chat | Dockview renders shared TaskChatPanel               | Fix scope and prove cold-route upload     |
| Quick Chat                | ChatInputArea and shared submit handler             | Preserve behavior and add real-file proof |
| General run transcript    | ChatInputArea and shared submit handler             | Preserve behavior and add real-file proof |
| Passthrough composer      | ChatInputContainer and attachment-aware request     | Share corrected scope and prove delivery  |
| Office agent run detail   | AdvancedChatPanel with hideInput                    | Preserve read-only behavior               |
| Office per-agent tabs     | AdvancedChatPanel with hideInput                    | Preserve read-only behavior               |
| Simple Office comments    | task-chat.tsx is imported by chat-activity-tabs.tsx | Preserve separate comment contract        |

The simple comment composer reads images into Markdown data URLs and submits
`createComment({body})`. It is not dead code. Do not delete it or silently route
comments through `message.add`. Its file-backed migration is not covered here.
`clipboard-attachments.ts` already warns for image-only HTML without readable
files. Existing tests cover that fallback; the report's claim is outdated.

This is source and pure-function evidence. No live browser or macOS clipboard
reproduction occurred. It establishes the current scope bug, not every reported
v0.96.0 symptom. No diagnostic files or processes require cleanup.

## Scope

In scope: exact task workspace resolution, cold routes, unresolved-scope
feedback, descriptor-only submission, stale upload isolation, and real-image
coverage for editable task-session composers.

Out of scope: enabling read-only views, file-backed Office comments, deleting
legacy composers, drag-and-drop expansion, upload limits, backend APIs, and new
runtime flags. This package does not justify closing every claim in #3980.

## Technical approach

Follow [Composer workspace resolution](../../specs/tasks/system-design/prompt-attachments.md#composer-workspace-resolution).
Add a shared scope hook around the existing resolver. Include exact Office task
identity and an authoritative task fetch for cold or conflicting state. Keep
that request authoritative while pending and after it resolves or fails, even
if cache state changes. Connect both shared and passthrough consumers. Correct
the upload and submit gates in `use-chat-input-state.ts`, including localized
feedback, restored unready drafts, full draft-identity cleanup, and
plan-implementation guards.

| Transport                   | Identity                    | Expected result                               | Evidence/fallback                                  |
| --------------------------- | --------------------------- | --------------------------------------------- | -------------------------------------------------- |
| ACP task message            | Exact task/session          | Uploaded ID reaches existing submit route     | Real-file E2E; unresolved scope blocks files       |
| Quick Chat                  | Persisted session workspace | Existing staging behavior remains             | Real-file E2E                                      |
| Passthrough                 | Exact task/session          | Descriptor reaches existing passthrough route | Mock terminal E2E; preserve provider delivery mode |
| Office read-only transcript | Run session                 | No composer appears                           | Read-only regression assertion                     |
| Office comment HTTP         | Task and comment body       | Existing comment behavior                     | No file-backed support claimed                     |

No provider capability expansion is implied. Existing native-image versus path
delivery choices remain authoritative. The backend still authorizes each upload.

## ASCII UI preview

UI-01: Editable task-session composer after image paste, desktop and phone.

```text
Before, missing scope             After, shared composition
[image.png: pending]               [image.png: uploading] [Remove]
[Message text           ]          [Message text                 ]
[Attach]          [Send]           [Attach]       [Send: disabled]
                                  Ready: [image.png] [Remove] [Send]
                                  Error: [Scope unavailable] [Retry]
                                  [Implement: disabled while upload is pending]
```

Phone: chips wrap above the full-width prompt. Attach, retry, remove, and send
have 44px touch targets. The prompt stays in the existing Chat surface, without
a new modal. The transcript remains the scroll owner. Preserve safe-area and
keyboard clearance. Desktop controls keep normal density. Order, reachability,
and disabled behavior are required; spacing and sample copy are illustrative.
All real copy uses translations. The same layout covers attachment-only input.
Read-only views have no input or attachment controls.
Maps to AC-TASKS-PROMPT-ATTACHMENTS-001.12 through .17.

## Implemented unit coverage

`composer-workspace.test.ts` proves exact Office scope, conflict handling,
unrelated-task isolation, snapshot matching, and preserved Quick Chat/workflow
resolution. `use-composer-workspace.test.ts` covers cold reads, retries,
deduplication, authoritative precedence over cache changes, and stale task
responses. `use-chat-input-state.test.ts` covers unresolved-scope blocking,
restored unready drafts, task changes that reuse a session ID, draft retention,
upload retry, text-only submission, and stale upload cleanup. Plan-action tests
prove incomplete attachments block message dispatch and workflow advance;
desktop and phone toolbar tests assert the disabled state. Existing clipboard
tests retain readable-file, unreadable-image, image-only HTML, and ordinary-text
behavior.

The new Office resolver and submission regressions were first run against the
old implementation and failed with a missing workspace and a sendable pending
file. They passed after the fix.

## E2E tests

The new `chromium` test uses real PNG bytes in a File/DataTransfer paste on a
cold Office advanced route while an unrelated workspace is selected. It checks
the upload workspace, submitted descriptor, stored message, and authorized
image content after reload. The `mobile-chrome` test checks failed upload and
retry, removal, file-picker fallback, touch targets, transcript display, reload,
and viewport overflow. The existing unreadable-image paste-warning test also
passes. Quick Chat and workflow scope preservation are covered by resolver and
shared-state tests; these new browser scenarios focus on the Office-only cold
route that reproduced the defect. Passthrough is wired to the same shared hook
and container.

Synthetic clipboard events verify application handling, not native OS clipboard
permissions. No browser test in this package claims native clipboard coverage.

## Work orders

- [x] [Task 01: Resolve attachment scope and verify delivery](task-01-resolve-attachment-scope.md)

## Verification results

- Initial package composer-focused Vitest suite: 131 tests passed across 11 files.
- Final PR-review focused Vitest suite: 153 tests passed across 7 files. Deferred
  hook cases cover
  cached scope disappearance/conflict and task A-to-null-to-A resolution,
  rejection, and retry. Deferred file-processing cases cover both scope/decode
  completion orders, draft switching, and explicit retry without auto-retry.
- `pnpm run typecheck`: passed after review fixes.
- Targeted ESLint completed without warnings; Prettier checks passed.
- `pnpm run i18n:check`: passed with 8,758 keys and all six locales complete.
- Desktop smoke unit tests: 17 passed. `pnpm --filter @kandev/desktop e2e`
  passed startup and two-window conflict recovery.
- `CAPTURE_PR_ASSETS=1 pnpm e2e:run --no-build --project chromium tests/chat/composer-attachment-scope.spec.ts`: 1 passed; captured the desktop ready-attachment state.
- `CAPTURE_PR_ASSETS=1 pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-composer-attachment-scope.spec.ts`: 1 passed; captured the mobile ready-attachment state.
- Final managed Chromium E2E run rebuilt the application and passed the Office
  cold-route attachment and unreadable-image paste-warning cases (2 tests).
- Final managed mobile-chrome E2E run rebuilt the application and passed the
  Office real-file upload, retry, and send case (1 test).
- Chromium unreadable-image paste-warning E2E: 1 passed.
- `pnpm run build:vite`: passed. Vite reported existing deprecation,
  ineffective dynamic-import, and chunk-size warnings.
- Public docs validation tests: 62 passed; validator accepted all 47 pages.
- `python3 scripts/list-docs.py validate`: passed, 311 decisions and 1,187 specs.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Post-PR review follow-up: authoritative task lookup remains the source of
  scope while in flight and after success or failure. A matching cache record
  cannot enable upload with an unverified workspace.
- Post-PR review follow-up: restored inline draft bytes are reconstructed into
  a pending `File` and uploaded before use. New incomplete browser files are not
  persisted as bytes. Unready attachments cannot submit inline payloads.
- Post-PR review follow-up: switching task identity while reusing the same
  session clears the old attachment draft and best-effort deletes ready staged
  files. Plan implementation remains disabled and guarded until all attachments
  have descriptors.
- Desktop smoke follow-up: the fake runtime now serializes atomic instance
  record replacements, so concurrent health, readiness, and root requests cannot
  expose truncated JSON to the test reader.
- Issue assignment to `carlosflorencio` was completed and verified during triage.
- Browser coverage uses synthetic paste and Office task chat. It does not prove
  native OS clipboard permission behavior or add real-file browser scenarios to
  Quick Chat, general run transcripts, and passthrough. Those surfaces continue
  to use the shared composer path; Quick Chat and workflow scope behavior have
  focused unit coverage.

## Risks

- Office comments remain a distinct attachment gap and require a separate design.
- Cold routes must not depend on a populated Office list or ambient workspace.
- Late uploaded files are deleted on a best-effort basis when their captured
  task/session/workspace owner no longer matches the active draft.
- Restored attachments without descriptors remain blocked. Recoverable old
  inline bytes are uploaded, while unrecoverable entries can be removed and
  reattached.
- A green synthetic paste test cannot establish native macOS browser behavior.

## Related packages and public documentation

Preparation attachment previews, session launch admission, and steer attachment
materialization are completed companion packages. Their backend scope and recorded
results remain unchanged. This package adds composer admission coverage only.
The attachment section in `docs/public/tasks-and-workflows.md` now describes
scope recovery and distinguishes task-session messages, comments, and
read-only transcripts.
