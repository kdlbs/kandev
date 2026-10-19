---
created: 2026-10-18
status: done
requirements:
  - REQ-UI-EDIT-FOCUS-001
system_design:
  - ../../specs/ui/system-design/inline-edit-focus-continuity.md
legacy_specs:
  - ../../specs/office-task-content-editing/spec.md
---

# Preserve inline edit drafts

## Overview

Preserve unfinished names when users leave the browser tab and return.
Implement task-title continuity first. Apply the same focus classification to
the related tab/file editors second. Both work orders are complete and their checks passed.

Component regressions reproduced unconditional blur completion and repeated
select-all handlers. Native browser tests also found dialog focus-trap
selection on return. The correction preserves that selection after Radix
restores focus, without forcing focus from a window listener.

## Scope and assumptions

The user requested task rename repair and an investigation of similar losses.
External focus means another browser tab, window, application, or browser
chrome. It does not mean navigating to a different task inside Kandev.

Preserve exact drafts, edit ownership, selection, and explicit completion.
Retain each consumer's existing internal click-away behavior. Exclude reload
persistence, save-error redesign, layout changes, and settings autosave.
No material product question blocks this package.

## Technical approach

Use the [focus design](../../specs/ui/system-design/inline-edit-focus-continuity.md)
for `useInlineEditFocus`. Keep drafts and persistence in their consumers.
Correct title selection at edit entry without replacing the existing clamp
hook. Scope every deferred action to one target and edit generation.

The following source paths require correction:

| Surface | Failure mechanism | Correction |
| --- | --- | --- |
| Task top-bar title | Blur cancels, focus selects all | Classify blur, select once, bind task identity |
| Office title | Blur cancels and releases field guard | Preserve draft and guard on external focus |
| Task Rename dialog | Focus handler and focus trap select all | Select at edit entry; restore external-focus selection |
| Session/terminal tabs | Blur commits | Preserve edit on external focus |
| Quick Chat tabs | Outside-container blur commits | Preserve container semantics and external draft |
| File-tree rename/create | Blur commits or cancels | Preserve external draft and avoid duplicate completion |

## Related-surface audit

- Shared task create/edit: `useFormResetEffects` gates resets on opening and
  context changes. No matching unconditional focus reset was found.
- Office description: no blur dismissal was found.
- Canvas rename: selects on opening. No repeated-focus selection was found.
- Saved-query dialogs in integration/GitHub/GitLab surfaces select on every
  focus. Record these as candidates, not reproduced draft-reset bugs.
- General settings blur handlers were inventoried but are outside this repair.

## Interaction preview

UI-01, desktop inline task title: the user edits `Release notes`, places the
caret after `Release`, leaves the browser tab, and returns. The input remains
open. Typing ` draft` produces `Release draft notes`. Enter saves, Escape
cancels, and an internal outside click retains the existing cancel behavior.

UI-02, phone task Rename: open the task-switcher action menu, choose Rename,
and edit the input above the existing Cancel and Save actions. Leaving and
returning keeps the draft and selection. Save remains the primary action.

UI-03, tab/file rename: leaving Kandev keeps the inline input open without a
write. Returning and explicitly completing the edit writes once.

These previews specify interaction state. Existing composition, touch targets,
scrolling, and geometry remain unchanged. The phone exemplar is the shipped
task-switcher Rename dialog and Quick Chat touch action menu. No new visual
styling or animation is proposed.

## Tests

Work order 01 covers AC-001.1 through AC-001.6 for task title consumers.
Work order 02 applies the same criteria to related inline editors.
Each work order names the exact component and browser suites. Include external
blur, internal focusable/nonfocusable targets, returning outside click, empty
draft, whitespace, same-target updates, target replacement, and stale cleanup.
Keep existing title clamp and IME checks.

## E2E tests

Add `task/inline-edit-focus.spec.ts` for task title, Office title, and Rename
dialog flows. Add `task/mobile-inline-edit-focus.spec.ts` for the real phone
Rename entry point. Add desktop and mobile `inline-name-focus` suites for the
related surfaces in work order 02. Use native foreground transitions, causal
waits, and persisted-state assertions. A synthetic focus event is unit evidence
only. Actual mobile OS app suspension remains a device-check limitation.

## Work orders

1. [Task 01: Preserve task title editing](task-01-task-title-continuity.md).
2. [Task 02: Preserve related name editing](task-02-related-name-continuity.md).

Execution is sequential. No delegation is authorized.

## Verification results

Implementation checks on 2026-10-19:

- Targeted Vitest: 10 files and 116 tests passed after the final source edit.
  This includes native-input selection ordering, repeat restoration, external
  draft retention, explicit completion, stale cleanup, and existing clamp tests.
- Rename dialog native focus regression: three consecutive runs passed. The
  trace found two input-focus restorations on return; the saved range now
  survives both until the next user action.
- ESLint: every changed and new TypeScript file passed with zero warnings.
- Typecheck: passed with `NODE_OPTIONS=--max-old-space-size=8192`.
- Phone browser suite: six tests passed, including native focus retention
  through the visible task Rename and Quick Chat menus, existing title-cap
  regressions, and touch-menu/viewport coverage.
- Desktop browser suite: 18 tests passed, including native external focus
  in top-bar, Office, Rename dialog, Quick Chat, session tab, and file-tree
  rename/create flows. Existing title-clamp and completion tests also passed.
- Terminal rename browser check: one test passed.
- Documentation catalog and specification lint: passed. Work-order coverage
  preflight accepted both work orders with no reference errors.
- `git diff --check`: passed. No public copy, API, or layout changed.

The exact task-level test commands are in the two work orders. Unit suites
were batched into one run. Native tests use full Chromium with driver focus
emulation disabled, not synthetic focus events. This NixOS host needed a
temporary library environment for Chromium and
`KANDEV_SERVER_HOST=127.0.0.1` for the isolated backend. An initial Xvfb probe
was stopped after headless full Chromium proved native transitions.

Browser logs are under `/tmp/kandev-focus-*.log`; they are local evidence,
not committed artifacts. Device-level mobile OS suspension was not tested.

Design checks passed on 2026-10-18:

- `python3 scripts/list-docs.py validate`: 384 decisions and 1618 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- PR documentation `validateCoverage` preflight: both work orders covered,
  no reference errors. Used the planned top-bar source path as an in-memory
  triggering change. No source file was modified for this check.
- `git diff --check`: passed. Package files are present and uncommitted.

The design turn made no production changes. Implementation was authorized
on 2026-10-19 and runs in isolated browser/backend fixtures.

## PR review remediation

Review of PR #4588 found that document listeners could finish a new file editor
before its delayed initial focus. Two failing hook regressions reproduced
outside pointer and focus events during that interval. The listener guard now
arms on initial focus and remains armed across external focus loss.

Claude's suggestions clarified why an explicit focus event on the document body
is an internal move and removed redundant `aria-disabled="false"` from an
interactive task title. The title test first failed on the old attribute.

Focused verification uses the same 10-file Vitest command (116 tests), plus
`pnpm exec eslint --max-warnings 0` on the four changed TypeScript files.
`pnpm run typecheck` and `pnpm run build:vite` passed. The managed Chromium
command for `tests/task/file-tree-create.spec.ts` and
`tests/task/file-tree-rename.spec.ts` passed all nine cases with
`E2E_PORT_OFFSET=27 KANDEV_SERVER_HOST=127.0.0.1` and the temporary Nix browser
library environment. CI and review completion remain externally pending.

## Risks

- Browser blur ordering differs across headless, headed, and mobile contexts.
- Suppressing every null-target blur would break nonfocusable outside clicks.
- Releasing Office's guard on external blur would apply deferred server state.
- Retaining an edit without target identity could rename the next task.

## Documentation

Public documentation needs no change. This repair preserves existing labels,
public APIs, and user workflow steps.
