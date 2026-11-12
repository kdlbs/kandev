---
created: 2026-11-11
status: complete
requirements:
  - REQ-UI-SELECTION-COMMENT-ACTIONS-001
  - REQ-UI-AGENT-MESSAGE-COMMENTS-001
system_design:
  - ../../specs/ui/system-design/selection-comment-actions.md
  - ../../specs/ui/system-design/agent-message-selection.md
legacy_specs: []
---

# Implementation plan: Touch selection comments

## Overview

Resolve [issue #5270](https://github.com/kdlbs/kandev/issues/5270) by placing
selection Comment actions in an owner-bounded dock on touch devices.
Three sequential work orders deliver chat and Office, Markdown preview, then
both code editors. The first order provides the shared dock for later consumers.
Each order includes its own regression and browser evidence.

The three sequential work orders delivered the shared dock and migrated all
covered surfaces. Review remediation is complete for viewport-clipped
clearance, Office transcript reservation, editor input ownership, and delayed
Markdown selection capture. Focused regression and browser evidence appears
below.

## Diagnosis and evidence

Investigation used checkout `fa0c48b88e774936fd28d81123e16dc96e9a5be3` on
2026-11-11. The canonical issue was open, without comments or image attachments.
Its assignment to authenticated user `carlosflorencio` succeeded and was verified.

Confirmed source paths:

- `SelectionCommentTrigger` in `message-comment-controls.tsx` always calls
  `placeFloatingRect` with `selection.rect`. Touch mode changes its size from
  28px to 44px, but does not change the position model.
- `CommentExtractSurface` reuses `MessageCommentSurface`, so Office shares
  the chat trigger.
- `MarkdownPreviewCommentOverlays` uses `currentSelection.position + 8px`.
- `CodeMirrorOverlays` uses mouse-derived `floatingButtonPos + 8px`.
- `MonacoFloatingCommentButton` uses the selected end position plus offsets.
- CodeMirror and Monaco reveal these triggers through mouse-release handlers.
  A touch dock must also admit eligible editor selections without those events.
- The shipped Plan toolbar already uses owner bounds and visual-viewport geometry.

A temporary `issue-5270-repro.test.tsx` rendered the real
`MessageCommentControls` with `useDrawer=true`. Moving only the selection
rectangle from y=180 to y=400 moved the trigger top from `120px` to `340px`.
The selection-independent assertion failed for that reason. The same change
correctly moved the fine-pointer control.

The diagnostic command ran from `apps/web`:

```bash
pnpm exec vitest run \
  components/task/chat/messages/issue-5270-repro.test.tsx \
  components/task/chat/messages/message-comment-surface.test.tsx \
  components/editors/tiptap/plan-bubble-menu.test.tsx \
  hooks/use-visual-viewport-offset.test.ts
```

Result: 36 existing tests and the fine-pointer control passed. The new touch
assertion failed. The temporary file was removed immediately afterward.
No production code or permanent tests changed during diagnosis.

The issue's Android report and the prior Plan correction explain the native
menu collision. The component reproduction proves Kandev's incorrect anchoring.
It does not reproduce or inspect operating-system chrome.

## Intent and specification classification

The issue explicitly requests docking across all listed selection comment
surfaces. The reusable placement behavior belongs to UI, independent of any
comment's data or delivery lifecycle. The current agent-message requirement
defines its editor and eligibility, but does not define this cross-surface dock.

This is a missing presentation requirement, covered by the new draft
`REQ-UI-SELECTION-COMMENT-ACTIONS-001`. Existing agent-message acceptance
criteria remain unchanged. The selection design links the proposed shared
presentation without changing message eligibility or delivery ownership.
No material product question blocks the package. No new ADR is necessary.

## Scope

### In scope

- Task Chat and Quick Chat settled agent replies.
- Office thread comment extracts, including their existing no-Run flow.
- Markdown file previews and both CodeMirror and Monaco file editors.
- Phone widths below 768px, plus coarse primary pointers at every width.
- Owner and modal bounds, composer clearance, safe areas, keyboard movement,
  touch-only selection tracking, and selection-preserving activation.
- Focused regression tests and updated public comment instructions during implementation.

### Out of scope

- Backend, database, API, queue, delivery, or comment-format changes.
- New comment eligibility, whole-file comments, diff gutter actions, or Browser annotations.
- Plan toolbar migration or comment-editor redesign.
- Native menu suppression, platform detection, or increased stacking as a workaround.
- Public plugin props, runtime flags, new dependencies, and unrelated responsive cleanup.

## Technical approach

### Shared dock and chat hosts

Implement the internal provider and dock in
`apps/web/components/shared/selection-comment-dock.tsx`, with a pure geometry
helper in `selection-comment-dock-position.ts`. These names identify proposed
files, not existing exports. Use the design's owner, composer, visible-viewport,
and portal coordinate contract.

`NativeMessageList` supplies its existing viewport to Task Chat and Quick Chat.
`TaskChat` supplies `scrollParent` and a local composer boundary for Office.
It must not add another vertical scroll region to the Office page.
`MessageCommentControls` chooses the dock for `isMobile || !isFinePointer`.
Keep its desktop trigger and existing drawer/popover editor unchanged.

Extend the owned touch-selection path in `use-message-comment-dom.ts` and
`MessageCommentSurface`. Preserve the tested mouse start-inside rule and
outside-dismissal behavior. Do not let delayed selection events replace an
open comment editor's quote.

### Markdown and editor consumers

Markdown uses its existing `scrollRef`. Its touch capture follows native range
changes, while viewport updates reposition rather than dismiss the dock.
CodeMirror and Monaco use their existing `editorAreaRef` and validated editor
selection state. Neither touch path requires `floatingButtonPos` or a mouse event.
Preserve desktop positioning and each current comment editor.

### Mobile design contract

- Entry point: native selection in the current reply, extract, preview, or editor.
- Exemplar: Plan's `MobileFormattingToolbar` contributes owner-bounded docking,
  compact geometry, and visual-viewport tracking.
- Hierarchy: current content, one localized Comment action, then the existing editor.
- Presentation: a short dock fits this frequent, single action. It adds no menu.
- Scrolling: the existing owner keeps vertical scrolling and reserves dock space.
- Geometry: the dock clears the composer, navigation, keyboard, and safe areas.
- State: each feature retains its quote, anchors, eligibility, and action handlers.
- Input: phone composition applies even with a fine pointer. Wide coarse-pointer
  screens also dock. Wider fine-pointer screens retain current placement.
- Accessibility: semantic activation, localized labels, and actual 44px touch targets.

### Documentation impact

`docs/public/developer-tools.md`, under **Comment on an agent reply**, currently
says that the button appears beside the selection. Task 01 updates that how-to
for the dock on phones and touch devices. Other public docs and the screenshot
catalog contain no position-specific instructions for the affected triggers.
Do not publish future behavior during this design-only turn.

## ASCII UI preview

The following repository previews describe structure, not pixel spacing.
Labels use existing localization keys. The work orders repeat their relevant views.

### UI-01: Selection Comment entry point, current and proposed

Entry: select eligible text. Current source positions every trigger near that text.

```text
Current touch                       Proposed wider fine pointer
+-----------------------------+     +-----------------------------+
| Reply or file text           |     | Reply or file text           |
| [selected text] [Comment]    |     | [selected text] [Comment]    |
|    native selection menu    |     |                             |
|    can cover Comment        |     |                             |
+-----------------------------+     +-----------------------------+

Proposed phone / coarse pointer
+-----------------------------+
| Reply, extract, or file      |
| [selected text]             |  Existing content scroll owner
| native selection menu       |  Native actions remain available
|                             |
|-----------------------------|
| [ Comment ]                 |  Dock inside owner's visible bottom
+-----------------------------+
| Chat composer, when present |  Separate from dock
| Navigation / safe area      |  Dock clears these regions
+-----------------------------+
```

The wide coarse-pointer layout uses the same dock inside its own pane.
The dock does not span adjacent panes. The closed-keyboard state clears the
composer and navigation. The open-keyboard state follows the visible owner
above the keyboard. A valid selection remains the same quote during viewport changes.

### UI-02: Comment editor after activation

```text
Chat / Office                  File surfaces
+---------------------------+  +---------------------------+
| Existing drawer/popover   |  | Existing popover or       |
| Selected quote            |  | Monaco inline view zone   |
| Feedback input            |  | Selected quote + feedback |
| [Add] [Run, if available]  |  | Existing Add / Run flow   |
+---------------------------+  +---------------------------+
No selection dock while the comment editor is open.
```

Only dock placement, scope, action visibility, and editor handoff are structural
requirements. Existing editor controls remain as shipped. Empty, invalid, or
ineligible selections show no dock. These views map to acceptance criteria
`.1` through `.8` of `REQ-UI-SELECTION-COMMENT-ACTIONS-001`.

## Tests

All paths in this section are relative to `apps/web`.

| Criteria | Permanent regression evidence |
| --- | --- |
| `.1`, `.2`, `.3` | New `components/shared/selection-comment-dock-position.test.ts` and `selection-comment-dock.test.tsx`: owner and viewport geometry, origin conversion, pointer modes, composer clearance, and listener cleanup |
| `.1`, `.4`, `.5`, `.8` | `components/task/chat/messages/message-comment-surface.test.tsx`: `keeps the touch Comment action independent of the selection rectangle`, touch-only range changes, single activation, stale target and open-editor isolation |
| `.1`, `.3`, `.4`, `.5`, `.8` | New `components/task/markdown-preview-content.comments.test.tsx` and existing `hooks/domains/comments/use-markdown-preview-comments.test.ts`: touch quote capture, owner position, line mapping, and dismissal |
| `.1`, `.3`, `.4`, `.5`, `.8` | New `components/editors/codemirror/codemirror-code-editor.comments.test.tsx` and `components/editors/monaco/monaco-code-editor.comments.test.tsx`: engine selections without mouse events, current lines, editor handoff, identity cleanup |
| `.6`, `.7` | Shared and consumer component tests: selection-preserving button events, native content behavior, accessible localized labels, and keyboard activation |
| Existing agent-message `.1`, `.2`, `.3`, `.4` | Existing message tests and chat browser flows retain eligibility, comment editor, selected quote, context, and storage behavior |

## E2E tests

Each work order names exact commands. Use the managed runner with fresh builds
and one worker per shard. Run desktop and mobile projects separately.

| Surface and scenario | File and project | Criteria |
| --- | --- | --- |
| Task Chat and Quick Chat: dock bounds, keyboard movement, touch activation, Add/Run outcome, modal hit testing | Existing `chat/mobile-agent-message-comments.spec.ts`, `mobile-chrome` | `.1`, `.3` through `.8` |
| Office extracts: dock within the outer scroll viewport, visible composer clearance, exact extract, no Run action | Existing `office/mobile-thread-extract-comments.spec.ts`, `mobile-chrome` | `.1`, `.3` through `.8` |
| Chat and Office: current floating action and comment flow | Existing `chat/agent-message-comments.spec.ts` and `office/thread-extract-comments.spec.ts`, `chromium` | `.2`, `.5`, `.8` |
| Markdown preview: dock, moving viewport, comment with correct quote and source lines | New `task/mobile-markdown-selection-comments.spec.ts`, `mobile-chrome`; new `task/markdown-selection-comments.spec.ts`, `chromium` | `.1` through `.8` |
| Both code editors: touch-only selection, dock, current quote/lines, actual pending comment | New `task/mobile-file-selection-comments.spec.ts`, `mobile-chrome`; new `task/file-selection-comments.spec.ts`, `chromium` | `.1` through `.8` |
| Input boundaries: 767px fine pointer docks, 768px fine pointer floats, and 1280px coarse pointer docks inside its pane | Named cases in the chat mobile/desktop files, using real browser pointer capabilities | `.1`, `.2`, `.3`, `.7` |
| Plan compatibility after new shared UI work | Existing `task/mobile-plan-formatting-toolbar.spec.ts`, `mobile-chrome`, plus existing Plan/viewport unit tests | Existing Plan formatting contract |

Select real DOM or editor text, then tap Comment and assert the quote and saved
context. Assert one trigger, actual button bounds, center-point hit testing,
owner and composer bounds, final-content reachability, and no document overflow.
Move the visual viewport while the selection is active, then assert unchanged
quote data and updated dock geometry. Do not count a missing test ID as behavioral RED.

Use browser contexts with appropriate pointer capabilities for boundary cases.
Keep the configured Pixel 5 for ordinary mobile tests. Viewport width alone
cannot convert a fine pointer into a coarse pointer.

Capture the phone rendering during these focused scenarios and compare it
with UI-01 and UI-02. Physical Android Chrome and iOS Safari remain separate
acceptance evidence when devices are available. Record availability explicitly.

## Work orders

- [x] [Task 01: Dock chat and Office Comment actions](task-01-chat-comment-dock.md)
- [x] [Task 02: Dock Markdown preview Comment actions](task-02-markdown-comment-dock.md)
- [x] [Task 03: Dock CodeMirror and Monaco Comment actions](task-03-code-editor-comment-dock.md)

Executed 01, then 02, then 03 in the primary session. All orders are `done`.
Tasks 02 and 03 each depend technically on Task 01. Their scheduled order keeps
execution sequential. Dependency waves do not authorize subagents.

## Documentation validation

From the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/touch-selection-comments
git status --short -- docs/specs docs/plans/touch-selection-comments
```

The following offline preflight validates both the actual diff and projected
production coverage. It includes untracked work orders and reads real contract bytes.
Run it again with the final actual diff before delivery.

```bash
node <<'JS'
const fs = require('node:fs');
const cp = require('node:child_process');
const assert = require('node:assert/strict');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/touch-selection-comments';
const docs = [
  ...fs.readdirSync(dir).filter(p => p.endsWith('.md')).map(p => `${dir}/${p}`),
  'docs/specs/ui/requirements/selection-comment-actions.md',
  'docs/specs/ui/system-design/selection-comment-actions.md',
  'docs/specs/ui/requirements/agent-message-comments.md',
  'docs/specs/ui/system-design/agent-message-selection.md',
];
const tracked = cp.execFileSync('git', ['diff', 'HEAD', '--name-only', '-z'])
  .toString().split('\0').filter(Boolean);
const untracked = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'])
  .toString().split('\0').filter(Boolean);
const actual = [...new Set([...tracked, ...untracked])];
const projected = [...new Set([...actual, ...docs,
  'apps/web/components/task/chat/messages/message-comment-controls.tsx',
  'apps/web/components/task/markdown-preview-content.tsx',
  'apps/web/components/editors/codemirror/codemirror-code-editor.tsx',
  'apps/web/components/editors/monaco/monaco-code-editor.tsx',
])];
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
for (const [mode, changedFiles] of [['actual', actual], ['projected', projected]]) {
  const result = validateCoverage({ changedFiles, fileContents });
  console.log(JSON.stringify({ mode, ok: result.ok, status: result.status,
    workOrders: result.workOrders, errors: result.errors }));
  assert.equal(result.ok, true);
  assert.deepEqual(result.errors, []);
  if (mode === 'projected') assert.equal(result.status, 'covered');
}
JS
```

## Verification results

Design validation completed on 2026-11-11:

- `python3 scripts/list-docs.py validate`: passed, with 449 decisions and
  1986 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: all 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Offline PR documentation preflight: the actual documentation-only diff is
  exempt. Projected production coverage passed with all three work orders and
  zero reference errors.
- Relative document links resolved. The prose received the simple-English
  sentence-length and vocabulary checks.
- Diff whitespace and workspace inventory checks passed. All seven package
  documents remain unstaged and uncommitted. The temporary reproduction is absent.

Implementation and permanent regression tests are complete. Final evidence:

- Focused Vitest: 14 files and 71 tests passed.
- `pnpm run typecheck` and Oxlint with `--deny-warnings` on all changed
  TypeScript files passed.
- Mobile E2E: 8 tests passed. Desktop E2E: 9 tests passed. Both used the managed
  production build and a single worker.
- `pnpm run i18n:check`, `pnpm run i18n:ratchet`, `python3
  scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`,
  `python3 scripts/lint-spec-files.py --all`, public-doc validators, the offline
  PR documentation preflight, and `git diff --check` passed.
- Physical Android Chrome and iOS Safari were unavailable. The configured
  Pixel 5 Playwright project passed; its emulation does not prove native OS
  selection-menu behavior.

Review remediation results:

- The four P2 findings were fixed. Shared docks reserve viewport-relative
  clearance; Monaco applies the same measured amount as editor padding; Office
  reserves space in the transcript before its composer; CodeMirror hands input
  ownership back to mouse/pen; Markdown delayed selection is canceled and
  generation-fenced across dismissal, editor handoff, and identity changes.
- Focused Vitest: 6 files and 27 tests passed.
- Focused mobile E2E: 5 tests passed across CodeMirror, Monaco, Markdown preview,
  and Office. The cases kept `window.innerHeight` and owner layout bounds fixed
  while shrinking and panning `visualViewport`, then verified final text above
  the dock. Office also verified final transcript text with the composer visible.
- Managed backend and Vite builds, `pnpm run typecheck`, targeted Oxlint with
  `--deny-warnings`, and `git diff --check` passed.

The diagnostic RED and existing test results appear under
[Diagnosis and evidence](#diagnosis-and-evidence).

## Risks

- Quick Chat modal portals can be visually present but outside the interactive
  scope. Hit testing and successful Add prove actual reachability.
- Office's composer shares the page scroller. Its dock needs a separate lower
  boundary without restructuring that scroll owner.
- Native handles update selections after touch release. A mouse-only dock can
  look correct in synthetic tests while remaining absent on a phone.
- A viewport resize currently dismisses Markdown overlays. The touch branch
  must retain the quote while geometry changes.
- Pointer mode and layout width differ. Tests must include wide touch screens
  and narrow fine-pointer screens.
- Desktop emulation cannot prove native menu coexistence on physical devices.
