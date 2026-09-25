---
created: 2026-09-17
status: implemented
requirements:
  - REQ-UI-REVIEW-FILE-COMMENTS-001
system_design:
  - ../../specs/ui/system-design/review-file-comments.md
legacy_specs: []
---

# Implementation plan: Review file comments

## Overview

Deliver one end-to-end work order: whole-file feedback through existing local
Review and composer flows. Start with model/formatter tests, wire the editor
and aggregate consumers, then prove desktop and phone outcomes.

## Scope

Creation, edit/delete, local persistence, repository identity, review counts,
overview, and existing feedback submission are in scope. Provider comments,
server persistence, review-state changes, and send-reliability redesign are not.

## Technical approach

Follow the [system design](../../specs/ui/system-design/review-file-comments.md).
Introduce a separate review-file comment variant, preserve line annotation
contracts, and widen aggregate review consumers. No backend behavior changes.
Update the existing how-to section in `docs/public/sessions-and-review.md` only
when implementation ships; retain its storage and sending limitations.

## ASCII UI preview

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

## Tests

Use TDD for changed logic. Extend `lib/state/slices/comments/format.test.ts`
and `persistence.test.ts` for mixed feedback, no fake anchors, round trips,
and unchanged legacy line data (AC .3-.5). Add `review-file-comments.test.tsx`
for add/cancel/blank/edit/delete and source-switch isolation (AC .1-.3).
Extend review dialog build-files/count and overview tests for session/repo
isolation, same-path repositories, root/nested scopes, and mixed totals
(AC .3-.5). Add `review-comments-attachment.test.tsx` for mixed rendering and
extend pending-comment and run-comment tests for the new source (AC .4-.5).

## E2E tests

Create `e2e/tests/review/review-file-comments.spec.ts` (chromium) and
`mobile-review-file-comments.spec.ts` (mobile-chrome). Reuse the neighboring
Fix comments fixture flow, but create file comments through real UI actions.
Desktop proves opening from a collapsed file, create/edit/cancel/delete, focus
restoration, counts, and persistence across reload. Phone proves menu-to-editor
focus, Add and Cancel, edit, Fix comments submission, 44px targets, and no
horizontal overflow. It checks 767px/768px transitions with coarse-pointer
controls. Existing Fix comments tests retain line-feedback regression coverage.
Patchless deleted files, repository/session isolation, mixed feedback, outgoing
normal/passthrough composer formatting, and attachment rendering are covered by
focused component/unit tests. The file-comment section is outside diff/Markdown
render branches, so entry does not depend on text hunks or preview mode.
Use causal request/response waits, not fixed sleeps. Compare rendered views to
UI-01/UI-02. Dedicated browser fixtures for every file status, Markdown preview,
and multiple repositories are not included in this work order's final coverage.

## Work orders

- [x] [Task 01: Deliver whole-file review feedback](task-01-file-comments.md)

One sequential work order; no delegation required or authorized.

## Verification

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

## Verification results

Design validation passed on 2026-09-17: catalog validation (988 specifications),
all specification lint checks, 36 specification-linter tests, and diff whitespace
check. Both new specifications are discoverable in the UI catalog.
Implemented and verified on 2026-09-17. The final targeted unit/component run
passed 32 files / 319 tests. RED failures were observed for whole-file formatting,
repository-scoped selection/counting, construction, mixed grouping, and the
root repository's absent-versus-empty name. Browser checks caught and verified
the Escape cancellation fix with the surrounding Review dialog mounted.

Desktop file-comment creation, edit, cancellation/focus, reload, and deletion
pass. Phone creation/edit, menu focus, close/reopen, 767px/768px coarse-pointer
controls, overflow checks, and Fix comments delivery pass. Existing Fix comments
regressions pass (three desktop and one phone). Desktop and phone screenshots
were inspected against UI-01/UI-02. See the work order for the commands and
coverage boundaries. Typecheck, changed-file ESLint, translations, and public
and specification documentation checks pass. No backend behavior changed.

## Risks

The comment union has consumers outside Review, especially normal and
passthrough composers. Missing one can silently omit feedback. Nested
repository scopes may share an ID, so path plus repository ID alone is
insufficient for new file comments. Existing Fix comments delivery can lose
pending feedback on a failed send; this task preserves that documented behavior.
