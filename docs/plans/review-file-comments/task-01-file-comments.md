---
id: "01-file-comments"
title: "Deliver whole-file review feedback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-REVIEW-FILE-COMMENTS-001
acceptance_criteria:
  - AC-UI-REVIEW-FILE-COMMENTS-001.1
  - AC-UI-REVIEW-FILE-COMMENTS-001.2
  - AC-UI-REVIEW-FILE-COMMENTS-001.3
  - AC-UI-REVIEW-FILE-COMMENTS-001.4
  - AC-UI-REVIEW-FILE-COMMENTS-001.5
  - AC-UI-REVIEW-FILE-COMMENTS-001.6
system_design:
  - ../../specs/ui/system-design/review-file-comments.md
---

# Task 01: Deliver whole-file review feedback

## Summary

Add whole-file comments from Review and carry them through existing local
feedback storage, overview, and submission. Preserve line-comment consumers.

## In scope

Own model/formatter/store integration, header entry points and inline editor,
review counts/overview, normal and passthrough composer integration, translations,
focused tests, and the public review how-to update.

## Out of scope

Provider writes, backend persistence, new thread semantics, and sending fixes.

## Acceptance

1. UI-01/UI-02 create, edit, remove, and restore file comments without line anchors; textless files work and repository/session isolation holds.
2. Mixed line/file counts, overview, attachments, and outgoing agent context work through Fix comments and composer routes without changing line annotations.
3. Targeted tests and localized desktop/phone rendering meet all six linked criteria; public documentation accurately describes the shipped behavior.

## ASCII UI preview

Full preview: [plan](plan.md#ascii-ui-preview).

### UI-01: Desktop file comment, editor open

```text
[v] src/parser.ts          [Comment on file] [other actions]
    File comments
    [existing feedback                           ] [Edit] [Delete]
    [Write feedback about this file...                       ]
                                           [Cancel] [Add]
    (existing diff or non-text change description)
```

### UI-02: Phone file comment

```text
[v] parser.ts                         [...]
    src/  Modified

File actions menu: [Comment on file]
After selection, menu closes:
    File comments
    [Write feedback...             ]
    [Cancel]                   [Add]
    (existing diff)
```

Header remains sticky; the existing review body owns scrolling. The inline
editor, placement before diff, and visible phone menu entry are structural.
Spacing and sample paths are illustrative. Add is disabled for empty text;
Cancel removes the editor without adding a card. A saved card replaces the
editor and offers Edit/Delete. Long paths truncate in chrome but full identity
is accessible. Shared send errors retain existing toasts and clearing rules.
These previews cover AC-UI-REVIEW-FILE-COMMENTS-001.1, .2, .4, and .6.

## Verification

Run from repository root. New test paths below are deliverables of this task.
Read `/tdd`, `/e2e`, and `/mobile-parity` before implementation. Rebuild before
browser checks and run the two projects sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/state/slices/comments hooks/domains/comments/use-pending-comments.test.ts hooks/domains/comments/use-run-comment.test.ts components/review components/task/chat/messages/review-comments-attachment.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/review lib/state/slices/comments hooks/domains/comments components/task/use-review-dialog.ts components/task/chat/messages/review-comments-attachment.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run build:e2e)
make build-backend
(cd apps/web && pnpm e2e:run --host --no-build --project chromium -- e2e/tests/review/review-file-comments.spec.ts e2e/tests/review/review-fix-comments-popover.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome -- e2e/tests/review/mobile-review-file-comments.spec.ts e2e/tests/review/mobile-review-fix-comments-popover.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Also lint any additional composer files changed during the source-union audit.
Run their changed test suites explicitly and record commands/results here.

## Files likely touched

- `apps/web/lib/state/slices/comments/{types,index,format,persistence}.ts` and tests.
- `apps/web/hooks/domains/comments/{use-pending-comments,use-run-comment}.ts` and tests; a new file-comment selector if needed.
- `apps/web/components/review/review-{diff-list,diff-header,diff-toolbar,dialog,top-bar,fix-comments-button,comments-overview}.tsx`; new `review-file-comments.tsx` and tests.
- `apps/web/components/task/use-review-dialog.ts`, `chat/messages/review-comments-attachment.tsx`, normal/passthrough composer context aggregation and removal callers found by `usePendingDiffComments`/`DiffComment` audit.
- `apps/web/components/diff/comment-form.tsx` only if reuse needs scoped touch sizing; preserve existing callers.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/review.json` and any other affected namespace; generate Traditional Chinese with `pnpm run i18n:zh-hant`.
- New desktop/mobile review E2E files, `docs/public/sessions-and-review.md`, and this package's status/results.

## Dependencies

None.

## Risks

See [plan risks](plan.md#risks). Do not widen line annotation selectors to
whole-file comments or apply legacy repository wildcard matching to new rows.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/review-file-comments.md).
- [Design](../../specs/ui/system-design/review-file-comments.md).
- `apps/web/components/review/AGENTS.md` and nearby Fix comments E2E flow.

## Results

Implemented on 2026-09-17.

- Final targeted Vitest run: 32 files / 319 tests passed. The command above also
  included `hooks/domains/comments/use-review-comments.test.ts`,
  `components/task/chat/chat-input-area.test.ts`,
  `components/task/passthrough-chat-composer.test.ts`, and
  `components/task/passthrough-toolbar.test.tsx`.
- Typecheck and ESLint of all changed/new TypeScript files passed. A small
  whole-file identity helper keeps the existing count function within its
  complexity limit while normalizing absent/empty root repository names.
- `pnpm run i18n:check` passed all locale, placeholder, pseudo, and copy checks.
- Fresh E2E frontend and backend builds succeeded. Targeted browser tests use
  `--host --no-build --retries=0` with one worker, sequential projects. Desktop:
  create/edit/cancel, opener focus, counts, reload, delete. Phone: menu entry,
  edit, close/reopen, 767px/768px touch sizing, overflow, and verified outgoing
  agent message. Existing Fix comments regressions: three desktop, one phone.
- Inspected desktop and phone screenshots: comments appear above the diff,
  with visible Edit/Delete actions and no clipped file-comment content.
- Specification catalog/lint, public-doc tests/validation, and `git diff --check`
  passed. Browser coverage limitations are recorded in the plan's E2E section.

The implementation uses the existing local comment store, preserves line-only
annotation selectors, and carries mixed feedback through Review, normal chat,
and passthrough chat. Sending/clearing reliability remains the existing contract.
Delivery includes the implementation, tests, translations, and design package in one feature commit.


### Visual alignment follow-up (2026-09-18)

Matched the line-comment card surface, spacing, monospace typography, blue icon,
and top-right edit/delete controls. Removed the redundant saved-comment section
heading and visible file path; creation still shows the target path, and saved
context remains screen-reader accessible. Markdown rendering is retained.
Desktop actions reveal on hover/focus, and phone actions remain visible at 44px.

Focused component tests, updated desktop/phone file-comment browser tests,
typecheck, ESLint, and spec lint passed. The final rebuilt isolated instance was
also checked in dark mode with both line and file comments visible, including
editing and Markdown rendering on desktop/phone. The test instance retains its
URL, task, and database; the main instance on :9998 was not changed.

Whole-file cards use the file section’s left padding on desktop and phone,
without the line-comment gutter indent (user follow-up, 2026-09-18).

### PR review corrections (2026-09-18)

Scoped Changes-panel delivery and clearing to the active session. Shared the
collision-safe repository/file grouping between the overview and sent-message
attachment, using repository IDs to combine mixed line/file feedback when
available and repository names for legacy whole-file records. Preserved named
repository context when opening a file from the passthrough comments panel.
Four new regression tests failed before their fixes; all 48 affected tests,
typecheck, and focused ESLint passed afterward.

The disposable user test instance has been shut down as requested. The main
instance on :9998 was not changed.

A follow-up review identified the same repository omission in regular composer
context chips. These now retain repositoryName and forward it through the chat
panel callback chain. Named-repository and explicit workspace-root regression
cases pass (five context-builder tests), along with typecheck.

Nested-scope review correction: whole-file grouping retains repository name/path
even when nested scopes share an ID. Legacy line rows join a named group only
when the ID-to-name mapping is unambiguous. Regression coverage also verifies
stable grouping when a repository ID becomes available later. The four new
identity cases and both aggregate component suites pass (16 tests).

Review-scope follow-up: newly created line comments retain the diff viewer's
repository name, including explicit root scope. Both Pierre and Monaco paths
carry it through creation and Run; annotation selection, aggregate grouping,
counts, composer navigation, and delivery formatting preserve it. Legacy saved
rows keep their previous fallback behavior. Added regression cases for mixed
scoped feedback, annotation isolation, file counts, grouping, and formatting.
Validation: 34 affected test files (272 tests), 26 focused count/build-file
checks after the count-helper extraction, typecheck, focused lint, and spec lint
passed. The three initial scope regressions failed before the implementation.

Completed the production line-comment creator audit: Markdown preview and both
file editors now pass explicit repository scope when creating and selecting
comments. Five new editor/preview regressions failed before the fix and pass
afterward, including named/root creation and cross-repository isolation.
The six affected editor/Markdown/selector/formatter suites pass (34 tests), as
do typecheck and focused lint. All production line-comment creators and scoped
selection callers were enumerated and checked.

CodeRabbit metadata correction: whole-file comments retain baseRef and
isSubmodule from the review file through creation, edits, and storage hydration.
The component regression failed before the fix; the existing lifecycle test now
verifies both fields survive edits and reload.
