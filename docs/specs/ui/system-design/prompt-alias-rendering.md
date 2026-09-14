---
status: draft
system: ui
requirements:
  - REQ-UI-PROMPT-ALIAS-001
  - REQ-UI-PROMPT-ALIAS-002
created: 2026-09-02
owners:
  - kandev
---

# Prompt Alias Rendering System Design

## Purpose and boundaries

The UI system owns the presentation contract for saved prompt aliases across
multiple task transcript surfaces. Prompt data and alias matching remain owned
by the existing prompt store and `lib/prompts/prompt-mention-segments` helpers.
Agent-facing prompt expansion and persistence are unchanged.

## Requirement mapping

| Requirement               | Design section                                                                                     |
| ------------------------- | -------------------------------------------------------------------------------------------------- |
| `REQ-UI-PROMPT-ALIAS-001` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow) |
| `REQ-UI-PROMPT-ALIAS-002` | [Task creation editor](#task-creation-editor) |

## Components and responsibilities

- `lib/prompts/prompt-mention-segments` remains the single matcher. It builds
  normalized prompt names and splits text into ordinary and recognized prompt
  segments.
- A shared prompt-mention presentation module under
  `apps/web/components/task/chat/messages/` owns the prompt chip, prompt lookup,
  and Markdown component factory currently embedded in `chat-message.tsx`.
  It exposes both the Markdown components used by rich transcript content and a
  text-segment renderer for single-line Prompt history rows.
- `ChatMessage` consumes the shared factory for the existing transcript user
  bubble. Its entity-reference component composition remains transcript-owned.
- `AnchoredLastPromptBar` passes the shared prompt components to
  `MemoizedMarkdown` after stripping system tags, preserving the existing
  Markdown renderer and height/overflow behavior.
- `PromptHistoryPanelContent` uses the shared text-segment renderer in both
  the collapsed and expanded row content. The row keeps its current text span,
  truncation measurement, expansion cap, navigation, and touch sizing. Content-
  bearing chips remain keyboard-focusable and intercept activation so keyboard
  and touch preview access does not navigate the row. Chips rendered inside
  Markdown links are visual-only, avoiding nested interactive semantics while
  preserving link activation.
- `MemoizedMarkdown` remains the common Markdown renderer and continues to
  normalize content through its existing cache. The change does not add raw HTML
  or alter the Markdown safety policy.

## Data and contracts

No backend, HTTP, WebSocket, or persistence contract changes. The shared UI
renderer reads `state.prompts.items`, matching the existing transcript path.
Recognized aliases are represented by the existing `custom-prompt-mention`
test ID and `data-prompt-name` attribute. Unknown aliases remain text.

The shared Markdown factory accepts optional entity-reference components so the
transcript can preserve its current entity chip behavior, while pinned content
(which has no message metadata) and history rows use the empty entity-reference
set.

## Control flow

1. Each mounted surface derives prompt names from the current prompt store.
2. The shared matcher identifies only recognized aliases using the existing
   boundary and name rules, excluding code spans and link destinations.
3. Rich Markdown surfaces pass the shared component map to `MemoizedMarkdown`,
   which injects prompt chips into supported Markdown block children.
4. Prompt history passes each plain row text through the shared segment renderer,
   preserving the row's single-line CSS measurement and expanded layout.
5. A chip looks up its current saved prompt by name. Existing non-empty prompts
   receive the hover preview; empty or missing content receives the existing
   title-only chip.
6. Store updates invalidate the derived names/components through React store
   subscriptions, so mounted views reconcile without changing message data.

## Failure and recovery

If the prompt store is not loaded or a name is absent, the alias is rendered as
ordinary text or a title-only chip according to the existing transcript behavior.
The renderer never modifies or expands persisted message content. If a surface
has no prompt text, its current empty state remains unchanged.

## Security

The change only reuses the existing prompt chip and `MemoizedMarkdown` safety
path. Prompt content shown in hover previews remains the same content already
available to the authenticated prompt store. No new HTML, URL, or navigation
handling is introduced.

## Observability

No new runtime metrics or logs are required. Unit coverage will assert shared
recognized/unknown behavior and surface-specific rendering; existing anchored
bar, ChatMessage, and Prompt history E2E coverage remains the behavioral smoke
signal.

## Task creation editor

This section is a proposed extension. Transcript behavior above remains unchanged.
UI owns this extension because alias presentation and editing serve all task types,
not only canvases. Tasks retains [saved-prompt delivery](../../tasks/system-design/saved-prompt-delivery.md).

### Integration boundary

`DialogPromptSection` currently uses `TaskFormInputs`, which owns a native textarea.
`useTaskCreatePromptMention` adapts that textarea to `useInlineMention` with
`promptInsertMode: "inline"`. That mode inserts the full saved definition today.

Add an explicit create-only editor mode through `TaskCreateDialog` and
`DialogPromptSection`. Do not infer create mode from `!isSessionMode`, because
task editing shares that branch. Other callers retain the textarea path.

The proposed `TaskPromptReferenceEditor` uses the installed Tiptap document,
paragraph, text, hard-break, history, and suggestion primitives. It does not
mount `TipTapInput` or `useTipTapEditor`: those own session history, slash commands,
entity references, and chat draft persistence. No new editor dependency is required.

### Plain-text contract and editing

Keep `TaskFormInputsHandle.getValue`, `setValue`, and `getAttachments` unchanged.
The external value remains plain text. New document conversion helpers preserve
leading/trailing whitespace, empty lines, aliases, and surrounding Markdown literally.
Do not parse the draft into rich Markdown or accept clipboard HTML as executable markup.

Use `splitMarkdownPromptMentionSegments` from `prompt-mention-components.tsx`
for recognition. If extraction is needed, move it with its tests without changing
matching behavior. Each recognized occurrence becomes an inline atom with its
exact original text and lookup name. Serialization returns that text, not a prompt
ID or definition. The atom uses `PromptMentionChip` and its existing theme tokens.

Presets, paste, and external value replacement recognize complete aliases immediately.
Typing recognizes a completed alias after a boundary or editor blur. Keep an active
`@` query editable until selection or completion, so shorter names do not capture
longer names during typing. Do not transform text during IME composition.

Backspace after an atom and Delete before an atom remove that occurrence.
A visible removal action offers the same outcome by pointer, keyboard, and touch.
Undo restores both reference text and chip presentation. Clipboard serialization
uses plain text for chips and preserves the selected surrounding text.

An internal editor adapter maps plain-text offsets to document positions for
selection and insertion. Do not reuse Tiptap document positions as string offsets.
Plugin insertion must retain synchronous insert-then-submit behavior from
`useDescriptionInput`, including two insertions within one callback.

### Suggestions, preview, and recovery

Use the existing prompt source, ranking, `MentionMenu`, and viewport placement.
Create-only selection writes an alias and a usable caret position. It must not
change the `inline` and `context` modes used by other consumers.
The suggestion handler consumes Enter and Tab before dialog submission.
Normal Enter remains a newline, and IME confirmation never submits.

Reuse `PromptMentionChip` preview content and pointer-aware disclosure behavior.
An editor-local action slot can supply removal without nesting a button inside
another button. Associate removal with the document occurrence, not the prompt name.
Closing the preview restores a stable selection beside that occurrence.

`useCustomPrompts` supplies prompt data through the existing store. Recognition
updates do not emit a changed draft, reset undo history, or move the caret.
When a name disappears, replace its atom with its original text while mapping selection.
Failed prompt loading leaves text usable and supplies no client-side fallback definition.
Task creation failure retains text, references, and attachments for retry.

### Desktop and phone composition

Desktop keeps the current task dialog and toolbar. The editable goal contains
inline chips rather than a separate attachment list. The shared chip supplies a
compact preview trigger. Ordinary actions retain the standard desktop density.

Phone keeps `TaskCreateDialog` as the full-height editing surface. It is already
the nearest creation exemplar. `PromptMentionChip` supplies the nearest preview
exemplar: `useTouchDrawer`, a fixed drawer header, and a safe-area-aware scroll body.
The drawer fits this temporary inspection task without adding a navigation step.

Chips wrap, and very long names stay within editor bounds with accessible full names.
Touch preview/removal actions meet the 44px hit-area minimum without enlarging
desktop controls. Use `useResponsiveBreakpoint` for layout and `useTouchDrawer`
for disclosure, including coarse-pointer tablets. The touch drawer exposes a
distinct localized description for assistive technology instead of repeating
its title.

The form body retains outer scrolling and its existing reachable footer.
The editor retains bounded internal scrolling for long drafts, with the caret visible.
The preview drawer owns its content scroll while open. Suggestions retain the
[visual viewport contract](../requirements/composer-suggestion-overlays.md).

### Compatibility and verification

There is no task schema, prompt CRUD, backend expansion, or permission change.
Task descriptions and launch preview retain literal references. Later launch uses
the then-current server definition under the existing workflow composition rules.
Passthrough sessions do not gain hidden expansion or a claim that chips guarantee delivery.

The [implementation plan](../../../plans/task-create-prompt-chips/plan.md) owns
serialization, editing, integration, and rendered desktop/phone checks for criteria `.1` through `.7`.
Tests must prove task payloads, not infer serialization from chip appearance.

Related decision: [Server-owned expansion](../../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md).


## Compact editable reference presentation

This draft extension implements criteria `AC-UI-PROMPT-ALIAS-002.8` through
`.10`, preserving `.3`, `.4`, and `.6`. UI owns this change because it extends
reusable alias presentation, without changing task state or prompt delivery.

### Current source and proposed composition

`TaskPromptReferenceView` in `apps/web/components/task-prompt-reference-node.tsx`
currently renders `PromptMentionChip` beside a separate remove button.
The chip owns its green border; removal has a 28px desktop box outside it.
`PromptMentionChip` also serves transcript and history consumers.

Add a narrowly scoped, opt-in editable presentation to `PromptMentionChip`.
The node view owns the shared green outer shell and its remove button.
The opt-in preview trigger inherits the shell's color and has no second border,
background, or outer padding. Default read-only chip rendering stays unchanged.
Default read-only touch preview triggers retain their established 44px target;
fine-pointer defaults retain their ordinary chip sizing.
Use sibling preview and remove controls, never nested interactive elements.
Keep prompt lookup, HoverCard, Drawer, and their handlers in the existing chip.
Apply the editable styling in the content-empty fallback as well.

At the standard 16px root font, use a 1.5rem desktop outer height and 0.75rem
label, retaining the current emerald theme tokens and rounded corners.
Account for borders when sizing children so the outer height stays 24px.
Use a small close icon, visible muted ink, hover feedback, and a focus-visible
outline. Keep removal's translated `task:removeLabeled` name and current test IDs.
Use border-box sizing, `min-w-0`, and an inner text-formatting box with a
shrinkable ellipsis label. Keep a non-shrinking remove control. The shell fits
within the editor's available content box; neither control can overlap the
other.
The label's accessible name retains the full alias even when visually truncated.

No new row wrapper, margin, or blank paragraph is inserted around a chip.
The smaller inline node reduces its own line footprint; user-authored blank
lines, editor padding, and toolbar spacing remain unchanged.

### Responsive and interaction contract

Use `useResponsiveBreakpoint` for phone sizing below 768px and combine it with
coarse-pointer detection for touch-sized controls. `useTouchDrawer` remains the
preview-mode authority, including coarse-pointer tablets and wider touch screens.
On phone or coarse-pointer devices, each actionable child is at least 44px square;
the outer shell includes those hit areas plus its border. Desktop sizing is the base.
Do not simulate a 44px target with overlapping invisible padding outside a 24px shell.

The nearest shipped exemplars are `TaskCreateDialog` for the phone creation
surface and `PromptMentionChip` for temporary preview through a touch Drawer.
Keep the inline label-first composition because references belong in editable text.
The close action removes one occurrence; the label previews it. Phone entry remains
New task through the existing FAB, in the full-height creation form.
The existing form body and bounded editor retain their scroll responsibilities.
The preview drawer retains its safe-area-aware scroll body. No navigation changes.

Preserve the node's `deleteNode` path, event propagation guards, selection,
plain-text serialization, and undo history. Preview triggers guard their pointer
press so opening and dismissing a touch preview does not leave the atom selected
for replacement by the next typed character. Preview dismissal returns to the
composer without changing the draft. Removal never triggers preview or submission.
No new store, backend, persistence, API, permissions, dependencies, or telemetry.

### Validation and delivery

The [compact-chip plan](../../../plans/compact-task-prompt-chips/plan.md) owns
focused component and desktop/phone rendered tests. Assert outer dimensions,
shared-border containment, separate targets, computed label size, long-name
truncation, focus feedback, occurrence removal, undo, and alias payloads.
Retain shared transcript preview regressions to detect default-style leakage.
