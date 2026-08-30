---
status: draft
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

Primary references:

- [VS Code 1.131 hybrid Markdown editor](https://code.visualstudio.com/updates/v1_131#_hybrid-markdown-editor-experimental)
- [VS Code 1.132 Markdown diff editing](https://code.visualstudio.com/updates/v1_132/#_markdown-diffs-in-the-hybrid-markdown-editor-experimental)
- [VS Code Markdown editor integration](https://github.com/microsoft/vscode/blob/main/extensions/markdown-language-features/markdown-editor-src/editor.ts)
- [VS Code Markdown editor provider](https://github.com/microsoft/vscode/blob/main/extensions/markdown-language-features/src/preview/markdownEditorProvider.ts)
- [Obsidian Live Preview](https://help.obsidian.md/Live%2Bpreview%2Bupdate)
- [Obsidian editor extensions](https://github.com/obsidianmd/obsidian-developer-docs/blob/main/en/Plugins/Editor/Editor%20extensions.md)
- [Obsidian editor decorations](https://docs.obsidian.md/Plugins/Editor/Decorations)

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-MARKDOWN-FILE-EDITING-001` | [Mode model](#mode-model), [Hybrid editor adapter](#hybrid-editor-adapter) |
| `REQ-UI-MARKDOWN-FILE-EDITING-002` | [Buffer and file lifecycle](#buffer-and-file-lifecycle), [Preview and integration contracts](#preview-and-integration-contracts) |
| `REQ-UI-MARKDOWN-FILE-EDITING-003` | [Responsive interaction contract](#responsive-interaction-contract), [Validation strategy](#validation-strategy) |

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

## Mode model

```text
Preview <-> Edit <-> Source
    \________________/
```

All modes read the same canonical source. Only Edit and Source can update it.
Preview is always read-only. `.md` supports all three modes. `.mdx` supports
Preview and Source because embedded JSX cannot round-trip through the hybrid
parser safely.

New `.md` tabs initialize with `markdownMode: "preview"`. New `.mdx` tabs also
start in Preview. A mode switch never triggers a save.

The mode toolbar uses visible labels on desktop and tablet. Phone uses the same
names in a compact selector or actions menu. Source remains a first-class
fallback, not a hidden recovery path.

## Data and contracts

### Open-file state

Replace the Boolean presentation flag with this UI type:

```ts
type MarkdownFileMode = "preview" | "edit" | "source"
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

### Receive an outside update

For a clean buffer, the existing update path replaces canonical source. The
hybrid adapter applies an authoritative replacement and maps the selection to
the nearest valid offset.

For a dirty buffer, the existing remote-update state remains visible. The
adapter keeps local content until the user chooses the existing reload action.

## Preview and integration contracts

Preview remains the high-fidelity reading surface. Edit mode favors immediate
editing context and exact source preservation. Visual results can differ for
constructs that the experimental engine does not support.

Unsupported blocks remain visible as editable source. Raw HTML never bypasses
Kandev sanitization. Edit mode must not use arbitrary `innerHTML` for Markdown
or Mermaid output. Links route through Kandev's existing task, file, and
external-link handlers.

Comment ranges remain source-line ranges. Preview source-line attributes and
Edit source offsets map to those same ranges. Creating or selecting a comment
does not change Markdown bytes.

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

### Phone

The Files entry point opens one focused `100dvh` editor surface. Its fixed
header shows file identity, mode, dirty state, Save, and Back. Less frequent
actions use a menu. The editor body is the only vertical scroll owner.

Controls have at least 44-pixel touch targets. The header and editing surface
respect safe-area insets. The body keeps enough keyboard clearance for the
active line and selection handles. Wide tables keep their existing local
horizontal scroller and do not widen the document.

The nearest implementation exemplar is the current `MobileFileViewerPanel`.
It already owns file identity, back navigation, preview comments, and contained
scrolling. This work extends that surface instead of compressing the desktop
dock layout.

## Failure and recovery

The adapter wraps initialization and source updates in a local error boundary.
If the package fails, Kandev keeps the canonical source, records a diagnostic,
and presents an accessible Source action. The failed hybrid instance is
disposed before retry.

An unsupported construct does not count as an editor failure. The adapter
shows source for that construct. If an upgrade changes source bytes without a
user edit, a source-preservation test fails and blocks the upgrade.

## Persistence

Open-file session state persists `markdownMode` with the existing file-tab
state. The mode is presentation state and does not enter repository content or
backend storage.

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

Desktop Playwright tests open a Markdown file, edit rendered content, save,
reload, check Preview output, use Source mode, and verify persisted mode. Mobile
Playwright tests repeat the edit and save flow with a virtual keyboard viewport.
They also verify safe controls, local table scrolling, and no page overflow.

Security tests keep the existing unsafe HTML and URL cases. Adapter tests add
unsafe link activation, unsupported HTML, initialization failure, and exact
source fallback cases.

## Related decisions

- [Use a Source-Preserving Hybrid Markdown Engine](../../../decisions/2026-08-27-source-preserving-hybrid-markdown-engine.md)
