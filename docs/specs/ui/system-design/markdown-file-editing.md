---
status: current
system: ui
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-002
  - REQ-UI-MARKDOWN-FILE-EDITING-003
---

# Markdown File Editing System Design

## Purpose and boundaries

The UI system owns the Markdown file modes, responsive controls, and editor
composition. The existing file buffer remains authoritative. Existing APIs
continue to own file reads, saves, dirty state, deletes, remote updates, and
version-control data.

This design applies to Markdown files opened from the Files surface. Review
diff previews keep their separate requirement. Shared preview behaviors keep
the contracts in [Resizable Markdown Table Columns](../requirements/resizable-markdown-tables.md),
[Comment Markdown Rendering](../requirements/comment-markdown.md), and
[Review Markdown Preview](../requirements/review-markdown-preview.md).

## Research findings

VS Code 1.131 introduced an experimental hybrid Markdown editor that renders
inactive blocks and reveals source around the active range. Its implementation
uses a canonical source string, a transient editor model, and host-owned file
operations. VS Code 1.132 added editable rendered diffs and gutter markers.
The editor runs in a webview and does not use Monaco as its document engine.

The published `@vscode/markdown-editor` package exposes the same browser editor
model, view, controller, history, comment, baseline, and gutter contracts. Its
pre-1.0 releases change often, and active VS Code issues still report editing
and rendering gaps.

Obsidian uses CodeMirror 6 extensions, decorations, and widgets for Live
Preview. It also keeps Markdown source canonical and reveals syntax near the
cursor. Obsidian's implementation is closed source, so its public developer
contracts describe the extension model but not the complete editor.

Both products support a source-preserving model. A rich document model such as
Kandev's Tiptap plan editor can normalize unsupported Markdown during a round
trip, so it is not suitable for arbitrary repository files.

Confluence exposes table insertion controls in gutters above columns and beside
rows, keeping those actions out of editable cells. It also exposes drag-based
width controls. Kandev adopts those interaction locations for the subset that
plain Markdown can represent, while retaining source-preservation and mobile
accessibility constraints.

Primary references:

- [VS Code 1.131 hybrid Markdown editor](https://code.visualstudio.com/updates/v1_131#_hybrid-markdown-editor-experimental)
- [VS Code 1.132 Markdown diff editing](https://code.visualstudio.com/updates/v1_132/#_markdown-diffs-in-the-hybrid-markdown-editor-experimental)
- [VS Code Markdown editor integration](https://github.com/microsoft/vscode/blob/main/extensions/markdown-language-features/markdown-editor-src/editor.ts)
- [VS Code Markdown editor provider](https://github.com/microsoft/vscode/blob/main/extensions/markdown-language-features/src/preview/markdownEditorProvider.ts)
- [Obsidian Live Preview](https://help.obsidian.md/Live%2Bpreview%2Bupdate)
- [Obsidian editor extensions](https://github.com/obsidianmd/obsidian-developer-docs/blob/main/en/Plugins/Editor/Editor%20extensions.md)
- [Obsidian editor decorations](https://docs.obsidian.md/Plugins/Editor/Decorations)
- [Confluence table editing](https://www.atlassian.com/software/confluence/resources/guides/best-practices/make-tables#make-quick-work-of-tables)

## Requirement mapping

| Requirement                        | Design section                                                                                                                   |
| ---------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-MARKDOWN-FILE-EDITING-001` | [Mode model](#mode-model), [Hybrid editor adapter](#hybrid-editor-adapter)                                                       |
| `REQ-UI-MARKDOWN-FILE-EDITING-002` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Preview and integration contracts](#preview-and-integration-contracts) |
| `REQ-UI-MARKDOWN-FILE-EDITING-003` | [Responsive interaction contract](#responsive-interaction-contract), [Validation strategy](#validation-strategy)                 |

## Components and responsibilities

### Markdown file coordinator

A new `MarkdownFileEditor` component selects Preview, Edit, or Source from one
canonical `content` value. It receives the current file identity, dirty state,
original content, update state, comment callbacks, and file actions from
`FileEditorPanel`.

The coordinator owns mode transitions and selection handoff. It does not save
files or create another durable document model.

### Hybrid editor adapter

`HybridMarkdownEditor` is Kandev's only import boundary for the experimental
VS Code package. The adapter creates and disposes the upstream model, view, and
controller. It translates upstream source edits into Kandev `onChange` calls.
It also owns link activation, task checkbox changes, source selection mapping,
history, baseline markers, and error reporting.

The package version must use an exact pin. A dependency upgrade requires the
adapter contract tests to pass before production code changes. No other Kandev
component imports the package.

### Preview renderer

`MarkdownPreviewContent` remains the Preview mode implementation and the
rendering authority. It keeps `react-markdown`, the shared remark plug-ins,
`rehype-raw`, and the immediately following `rehype-sanitize` step. Existing
task links, comments, Mermaid diagrams, source-line attributes, external links,
and resizable tables remain in this component.

### Source editors

Desktop Source mode uses Kandev's selected Monaco or CodeMirror provider.
Phone Source mode uses the existing CodeMirror integration with editing and
save enabled. Source mode supports every `.md` and `.mdx` file.

### Responsive surfaces

`FileEditorPanel` hosts the desktop and tablet coordinator. The mobile Files
route upgrades `MobileFileViewerPanel` from a read-only viewer to a focused
file editor for text files. Both surfaces use the same mode type, file buffer,
save contract, and hybrid adapter.

### Responsive draft ownership

`TaskLayout` retains the draft/session controller above `ResponsiveTaskLayout`.
The proposed registry uses existing `FileEditorState` records, keyed by
session, repository name, and path, with an opaque instance identifier.
`dockview-file-state.ts` remains the mutation boundary. Its actions update the
canonical registry and the active-session `openFiles` projection atomically.
The projection references the same records. It is not a second editable copy.

Mobile retains a selected draft key and reads that record. The coarse-pointer
tablet `TaskCenterPanel` also reads canonical records through its existing tab
controller. Its tab list owns presentation metadata only. Desktop keeps its
existing projection consumers. `useFileEditorEffects` activates the session
projection and reuses retained records before restoring persisted tab metadata
or fetching file content. Restoration can clear a projection but cannot retire
a dirty draft. Explicit discard, close, or confirmed task departure retires
the relevant records. A responsive unmount does not.

Each asynchronous read, notification, and save captures session identity,
owner epoch, draft instance, and operation generation. Retire or replace
invalidates those tokens. A save acknowledgement can update its live canonical
record after the viewport changes. Existing Dockview visit and API guards still
protect panel effects and toasts. Those presentation guards cannot discard a
valid acknowledgement merely because the editor component unmounted.

A successful save advances the baseline to the saved snapshot while preserving
newer typing as dirty. An accepted external write publishes content, baseline,
hash, and cleanliness together, even when content bytes are unchanged.
Remove the phone's independently authoritative baseline and draft copies.

The retained controller registers the existing routing navigation blocker and
native `beforeunload` warning while a draft is dirty. Existing mobile switcher
actions use the same discard decision. Cancel superseded navigation intents.
Do not automatically save on navigation or overwrite a draft from restoration.
This extends the existing canonical-buffer contract without introducing draft
disk persistence or a second workspace save API.

## Mode model

```text
Preview <-> Edit <-> Source
    \________________/
```

All modes read the same canonical source. Only Edit and Source can update it.
Preview is always read-only. `.md` supports all three modes across current
Chromium, Firefox/Zen, and Safari browsers. `.mdx` supports
Preview and Source because embedded JSX cannot round-trip through the hybrid
parser safely.

New `.md` tabs initialize with `markdownMode: "preview"`. New `.mdx` tabs also
start in Preview. A mode switch never triggers a save.

Desktop and tablet retain the existing toolbar and its overflow menu for narrow
Source panels. Mode controls use visible labels. Phone uses the same names in
a compact selector. Source remains available for normal editing and recovery.

Input transport selection belongs at the adapter boundary. Use native
EditContext when available, and the scoped native-textarea transport otherwise.
A missing or unusable EditContext constructor does not disable Edit. Do not
identify browsers by name or change a stored mode preference. Source recovery
is reserved for actual editor initialization or runtime failures.

### Browser input transport

Retain the pinned engine's model, controller, rendering, history, and source
selection. Add an input-transport factory through a maintained pnpm patch to
`@vscode/markdown-editor@0.0.2-84`. Patch its runtime and declarations together.
Keep the patch limited to transport creation, attachment, focus, suspension,
geometry, and disposal. Do not fork its Markdown parser or table rendering.

`EditorView` currently constructs one EditContext and assigns it at two sites.
Its controller consumes text updates and synchronizes source text and selection.
The transport exposes that context contract plus `attach`, `focus`,
`setSuspended`, `syncState`, and `dispose`. Its factory receives `getCaretRect`
and `onSelection`. Selection callbacks carry canonical anchor/active offsets
and instance/source revision. Native EditContext retains upstream behavior. The textarea implementation is owned below
`components/editors/markdown/input/`, not installed on browser globals.

Mount one stable native textarea in a shadow root attached to the editor host.
Native keyboard and clipboard events retarget to the host, which preserves the
controller's existing root-target checks. The engine retains geometric pointer
selection and source-offset mapping. Do not synthesize a second DOM selection
model or patch Selection, document focus, execCommand, or other editors.

The transport context retains exact canonical source. A textarea normalizes
line endings, so keep a native-input projection and a bidirectional offset map.
Translate each native replacement into a source-range edit. Preserve untouched
CRLF, LF, mixed endings, BOM, whitespace, and non-BMP text. New line endings use
the edited line's style, then the document's prevailing style, then LF. Never
publish the complete textarea value as canonical source. Existing engine history
owns inverse edits and exact undo/redo bytes.

Native selection changes from touch handles or assistive technology use the
selection callback, not `textupdate`. The pinned controller treats textupdate
as insertion and ignores its selection endpoints. Apply accepted callbacks
directly to `model.selection` with `selectionSource: "user"`. Preserve direction
and read-only selection. Do not emit a source edit, dirty callback, or history
entry. Reject stale instance/revision notifications and suppress selection
echoes. For native text input, accept the source edit before applying its mapped
selection. Defer or settle selection changes that conflict with composition.

Synchronize textarea selection after engine pointer and keyboard selection, before
composition starts. During composition, retain the input node and native value,
selection, and replacement span. A matching model echo must not reset native
composition. Apply each native delta once, including cancelled composition and
sequential compositions. Settle composition through native focus/input events
before hiding or suspending the editor. A mode transition or outside replacement
must wait for settlement rather than replacing composing text. The adapter's
`runWhenSettled` queues the newest callback per owner, independently for source
and read-only updates. Keep existing canonical draft and stale-instance guards.

Keyboard navigation remains controller-owned. Typing and composition update the
transport context. Clipboard and history commands have one owner: preserve the
engine's synchronous clipboard strategy and history. Prevent native defaults
only after the owning command handles them. Native cut/paste, beforeinput history,
and keyboard shortcuts must not produce duplicate edits or a separate textarea
undo stack. Clipboard permissions and secure-context-only APIs are not required.

Map the public engine caret rectangle into client coordinates after selection,
layout, and scroll changes. Position the input near the caret so mobile keyboards
and IME candidates follow the active text. Keep the input stable while table
controls, Find, or comment controls temporarily suspend editor input. Resume only
when the same editor instance still owns focus. Disposal removes its input and
listeners once after controller teardown.

Provide a named native editing control for the textarea path. Expose current
text and selection to assistive technology. Avoid duplicate editor announcements
by excluding only the decorative rendering subtree, not the native input or
interactive controls. Do not apply an aria-hidden ancestor to the input. Validate
native EditContext accessibility separately. Host textbox semantics alone do not
establish accessible native selection or screen-reader editing.

This is an owned compatibility layer, not a claim that an unmodified polyfill
provides parity. The published EditContext polyfill installs global selection,
focus, and legacy command changes and documents composition gaps. It can inform
input translation, but it is not installed wholesale. Any reused source requires
an exact revision, retained license, and tests through this engine. See the
[browser-input decision](../../../decisions/2026-10-21-scoped-markdown-browser-input.md).


## Data and contracts

### Open-file state

Replace the Boolean presentation flag with this UI type:

```ts
type MarkdownFileMode = "preview" | "edit" | "source";
```

Open-file records and restored file tabs store `markdownMode` explicitly. The
restoration boundary accepts the legacy `markdownPreview` value during one-way
migration:

- `true` becomes `preview`.
- `false` becomes `source`.
- A restored legacy record without the field becomes `source`.
- A newly opened Markdown file becomes `preview`.

Writers stop storing `markdownPreview` after migration. Existing Review-local
flags keep their current names because they represent a different interaction.

### Hybrid adapter contract

The adapter accepts source text, read-only state, language services, comments,
optional comparison baseline content, and host callbacks. It emits minimal
source changes where the upstream API supplies them. The parent still receives
a complete current string for compatibility with the file buffer.

The adapter keeps its transient model alive while Preview hides the editing
view. This preserves local hybrid undo history. A Source edit replaces the
adapter's authoritative text and clears stale redo entries. The next Edit view
starts from the exact Source result.

The adapter maps line and column positions to source offsets. It does not store
rendered DOM positions in file or comment state.

The adapter supplies the upstream editor with Kandev's existing Monaco Monarch
runtime and a bounded set of bundled language grammars. The upstream
incremental highlighter owns tokenization for supported fenced code blocks;
Kandev's scoped theme maps those tokens to application colors. Unknown
languages remain readable plain text.

The upstream engine does not provide table-structure commands. Kandev therefore
owns positional row and column insertion at the adapter boundary. Each edge
action locates the active table AST, identifies the semantic row or column,
constructs one source edit, records it in the adapter's local history, and
places the selection in the inserted cell. Inserting below the visible header
places the new row after the required delimiter row. The source helper parses
escaped pipes, preserves line endings, leading and trailing pipe style, and
all existing cell bytes. It never converts the table into another document
model.

Kandev also owns transient column sizing. A per-file-tab table presentation
map keys widths by the active table's source start and column index. Drag or
keyboard resizing updates a table-local `colgroup` and the edge-control
geometry without emitting a source edit. Mode switches retain the map because
the hybrid lifecycle remains mounted; closing the file tab disposes it. A
structural edit retains widths for existing column indices and gives an
inserted column the default width.

## Control flow

### Open and edit

1. The Files surface loads file content through the existing file hook.
2. The open-file state resolves the persisted Markdown mode.
3. `MarkdownFileEditor` mounts the selected presentation.
4. Edit or Source emits a new canonical source through `onChange`.
5. The existing file hook marks the buffer dirty.
6. The existing Save action or keyboard shortcut persists the buffer.

### Switch presentation

1. The active editor records a source selection and nearest source line.
2. The coordinator changes only `markdownMode`.
3. The target mode receives the same canonical source.
4. The target restores the source selection or nearest rendered source line.
5. Preview or Edit restores the nearest useful scroll position.

Use a transient source-position value with optional anchor and active offsets
plus a one-based nearest source line. Preserve selection direction. The new
editor-local `SourcePositionHandle` capture/restore port connects Monaco,
CodeMirror, the hybrid adapter, and Preview through the same coordinator hook.
These are proposed local interfaces, not a plugin API or persisted file format.

Capture synchronously before the explicit transition. Restore once the target
is ready and visible, scoped to the current draft instance and transition.
Clamp offsets after authoritative replacement. Preview uses existing
source-line attributes and scrolls only its own container. The hybrid adapter
uses its model selection and public range-reveal API. Until deferred reveal
runs, capture retains the requested position. User scrolling, source changes,
exit, or failure cancels pending intent/reveal. Source providers use native
selection APIs. Source retains Monaco. Its exact-version pnpm patch normalizes
native backward range bounds/direction and input-delta endpoints while keeping
model direction. Restoring Source uses immediate scrolling so a rapid handoff
captures its restored region. Real package tests and browser replacement guard upgrades.

Do not restore on each keystroke, reveal a hidden editor, or route selection
through the global pending-cursor registry. That registry handles point
navigation rather than cross-mode selection and reading position.

### Receive an outside update

For a clean buffer, the existing update path replaces canonical source. The
hybrid adapter applies an authoritative replacement and maps the selection to
the nearest valid offset.

For a dirty buffer, the existing remote-update state remains visible. The
adapter keeps local content until the user chooses the existing reload action.

## Preview and integration contracts

Walkthrough range geometry and navigation follow the
[Markdown walkthrough design](markdown-walkthrough-ranges.md).

Preview remains the high-fidelity reading surface. Edit mode favors immediate
editing context and exact source preservation. Visual results can differ for
constructs that the experimental engine does not support.

The Kandev hybrid-editor theme hides the upstream return-arrow decorations for
block-gap and block-break newlines. Their source spans remain in layout so
source-offset mapping, selection geometry, and canonical line-ending bytes do
not change.

The same scoped theme gives active blocks a two-pixel radius. It restores a
one-pixel themed border on every table cell in both active and inactive table
states because the upstream active-table theme otherwise makes cell borders
transparent. Active tables retain themed cell surfaces and a muted header
instead of inheriting the upstream source-outline presentation or a block-wide
highlight. Preview continues to use the shared bordered Markdown table renderer.

The scoped Edit theme overrides the upstream active-table delimiter rule so
`.md-table-delimiter-row` remains hidden. This is presentation-only: the AST,
source offsets, history, and canonical delimiter bytes remain unchanged. The
active header keeps normal cell padding so hiding the compact delimiter row
does not leave a false editing gap. Active table-cell glue spans also remain in
the source mapping but are visually hidden, so Markdown pipe delimiters do not
appear inside bordered cells.

Unsupported blocks remain visible as editable source. Raw HTML never bypasses
Kandev sanitization. Edit mode must not use arbitrary `innerHTML` for Markdown
or Mermaid output. Links route through Kandev's existing task, file, and
external-link handlers.

Comment ranges remain source-line ranges. Preview source-line attributes and
Edit source offsets map to those same ranges. Creating or selecting a comment
does not change Markdown bytes.

`MarkdownFileEditor` and the phone surface pass `repo` through
`useMarkdownEditorCommentState`. The bridge passes that repository name to
`useDiffFileComments` and `buildDiffComment`, using the existing Preview and
Source convention. The primary repository uses the existing empty-name
convention. Retain legacy unqualified-comment behavior from the selector.
Do not infer an additional repository's identity from the primary repository ID.

When Files opens a workspace-prefixed path, resolve its checkout qualifier from
the known workspace and worktree roots before publishing the tab identity.
Keep the initial workspace read request unchanged. Preserve explicit qualifiers
and unqualified paths that do not match a known checkout root.

The pinned engine exposes its comment input only in read-only mode. Editable
hybrid selection therefore uses Kandev's existing `EditorCommentPopover`.
Observe the engine selection and caret position, and capture the source range
before opening the popover. Keep its native textarea outside the editor input
host. Submit through the same repository-qualified bridge without changing
read-only state or source content. Dispose selection subscriptions with the
editor instance.

Invalidate a captured selection when source, file identity, instance, visibility,
or read-only state changes. Recheck that ownership before submission. Clamp the
comment trigger to the visible viewport. Return focus after an accepted comment
only while the same editor remains active.

An explicit comparison surface can supply baseline content to the adapter. The
ordinary Files editor does not treat its saved `originalContent` buffer as a
comparison baseline because that would stack the old and edited blocks during
normal input. Added and changed lines in an explicit comparison receive gutter
markers. Kandev's existing version-control data remains authoritative.

## Responsive interaction contract

### Desktop and tablet

The existing file tab remains the focal surface. A single mode control replaces
the eye/code toggle. Preview is the initial mode, Edit is the prominent action,
and Source remains available beside it. The existing toolbar continues to own
Save, Delete, Reload, Comments, and external file actions.

When a table is active, a table-local edge layer reserves a narrow top gutter
above columns and a left gutter beside rows. Fine-pointer users see compact
dots close to the table. Hover or keyboard focus turns a dot into a blue plus
and draws a blue guide across the affected row or column edge. Each row action
inserts below its row; each column action inserts to the right of its column.
The layer also places a compact draggable separator at every internal column
boundary. Its resize guide appears only while that separator is hovered,
focused, or dragged. Resize separators support Left and Right arrow keys after
focus. The layer follows the table's local horizontal scroller, never creates a
second file toolbar, and never overlaps editable cells.

### Phone

The Files entry point opens one focused `100dvh` editor surface. Its fixed
header shows file identity, mode, dirty state, Save, and Back. Less frequent
actions use a menu. The editor body is the only vertical scroll owner.

Keep Save in the fixed header. Use a compact, labelled current-mode trigger
and a visible actions trigger instead of placing every mode and Reload beside
Save. Mode choices and Reload use the existing responsive menu primitives.
The remote-update cue remains visible while Reload is in the menu.
This composition follows the persistent-toolbar pattern in the mobile UI
language guide. It does not require a second full-screen choice surface.

Controls have at least 44-pixel touch targets. The header and editing surface
respect safe-area insets. The body keeps enough keyboard clearance for the
active line and selection handles. Wide tables keep their existing local
horizontal scroller and do not widen the document.

Table edge actions use the same labels as desktop, but remain visibly
discoverable without hover and expand to 44-pixel hit targets. Column resize
handles accept touch drag and expose the same keyboard alternative. The edge
layer stays inside the table-local horizontal wrapper so the file surface
retains one vertical scroll owner and document-level horizontal overflow stays
zero.

The nearest implementation exemplar is the current `MobileFileViewerPanel`.
It already owns file identity, back navigation, preview comments, and contained
scrolling. This work extends that surface instead of compressing the desktop
dock layout.

## Failure and recovery

The adapter wraps initialization and source updates in a local error boundary.
If the package fails, Kandev keeps the canonical source, records a diagnostic,
and presents an accessible Source action. The failed hybrid instance is
disposed before retry.

Desktop and phone coordinators clear the failed instance's retention flag and
select Source. A later explicit Edit action mounts a fresh instance with the
latest canonical source. Healthy instances remain mounted across mode changes
to retain undo history and transient widths. Teardown has one owner and is
idempotent across initialization, update failure, and React cleanup.

Each failure produces one localized recovery message and one content-free
diagnostic. A missing EditContext uses native textarea input instead of this failure path.
Neither recovery nor transport selection saves or rewrites content.

An unsupported construct does not count as an editor failure. The adapter
shows source for that construct. If an upgrade changes source bytes without a
user edit, a source-preservation test fails and blocks the upgrade.

## Persistence

Open-file session state persists `markdownMode` with the existing file-tab
state. The mode is presentation state and does not enter repository content or
backend storage.

Transient Edit-mode column widths live only with the mounted file-tab hybrid
lifecycle. They survive Preview, Edit, and Source switches while that tab is
open, but do not enter Markdown, local storage, or backend state.

The canonical source and dirty state keep their current lifetime. Closing an
unpersisted tab follows the existing unsaved-change flow. Disposing a file tab
also disposes its hybrid model and local history.

## Security

Preview keeps the required `rehype-raw` then `rehype-sanitize` sequence. Edit
mode does not execute embedded HTML, scripts, event attributes, or unsafe URLs.
It treats unsupported HTML as source text.

The host validates link destinations through existing navigation helpers.
Images, task links, file links, and Mermaid diagrams use Kandev-owned renderers
or remain source until an audited renderer is available.

## Observability

Adapter failures emit one structured browser diagnostic with the file type,
active mode, operation, and upstream package version. Diagnostics exclude file
content and repository secrets.

No product metric is required for the first release. Test failures and browser
diagnostics are sufficient for the local editor boundary.

## Validation strategy

Unit and component tests use a source-preservation corpus with headings,
emphasis, nested lists, task lists, tables, code fences, frontmatter, footnotes,
math, raw HTML, links, images, comments, and unsupported syntax. Tests prove
that focus, blur, mode changes, and external updates do not change untouched
bytes.

Focused adapter tests also prove supported-code tokenization is connected,
positional row and column edits preserve existing source bytes and line
endings, the delimiter row remains hidden, table controls and resize handles
are keyboard accessible, structural edits participate in local undo history,
and resizing emits no source edit.

Desktop Playwright tests open a Markdown file, edit rendered content, save,
reload, check Preview output, use Source mode, and verify persisted mode. Mobile
Playwright tests repeat the edit and save flow with a virtual keyboard viewport.
They also verify safe edge controls, touch resizing, local table scrolling, and
no page overflow.

Security tests keep the existing unsafe HTML and URL cases. Adapter tests add
unsafe link activation, unsupported HTML, initialization failure, and exact
source fallback cases.

Mocked lifecycle tests prove callback wiring and teardown only. Source
preservation evidence must mount the pinned real engine through the production
adapter. The corpus covers CRLF and LF, frontmatter, nested lists, escaped table
pipes, raw HTML, and unsupported constructs. Apply edits through the rendered
editor, then compare the canonical bytes before and after undo, mode changes,
and authoritative replacement. Remove the unused `applyMarkdownSourceEdit`
helper if it remains outside the production path.

Browser tests run rendered paragraph and table-cell editing through real
Chromium, Firefox, and WebKit. Missing EditContext must select native text input
and keep Edit available. Retain runtime failure and explicit retry tests.
Chromium with the API removed tests transport selection only. It does not prove
Firefox or Safari compatibility.

Each engine must cover selection replacement, deletion, Enter/Tab, clipboard,
undo/redo, comments, mode handoff, save/reload, and exact-byte source preservation.
Include composition after pointer and keyboard selection, sequential composition,
blur, mode changes, and outside replacement. Check two hybrid instances and a
Monaco/CodeMirror editor together to prove event and focus containment.

Record real Zen/macOS, Safari/macOS, Android keyboard, and iOS Safari keyboard
results separately from Playwright automation. WebKit emulation does not prove
OS candidate windows, autocorrect, selection handles, or virtual keyboards.
NVDA/Firefox and VoiceOver/Safari checks must prove reading, selection, editing,
and focus return. Missing device or assistive-technology evidence remains an
open acceptance item, not an automatic pass.

## Implementation plans

- [Original Markdown editing package](../../../plans/markdown-file-editing/plan.md).
- [Review remediation package](../../../plans/markdown-editing-remediation/plan.md).
- [Browser parity correction](../../../plans/markdown-browser-parity/plan.md).

## Related decisions

- [Use a Source-Preserving Hybrid Markdown Engine](../../../decisions/2026-08-27-source-preserving-hybrid-markdown-engine.md)
