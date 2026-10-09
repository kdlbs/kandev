# ADR-2026-08-27-source-preserving-hybrid-markdown-engine: Use a Source-Preserving Hybrid Markdown Engine

**Status:** proposed
**Date:** 2026-08-27
**Area:** frontend

## Context

Kandev uses Monaco or CodeMirror for file source and `react-markdown` for a
separate sanitized preview. Users must switch the entire file surface to check
formatting. Kandev's Tiptap plan editor provides rich editing for a controlled
document type, but arbitrary repository Markdown can contain syntax that a
ProseMirror schema cannot round-trip without normalization.

[VS Code 1.131](https://code.visualstudio.com/updates/v1_131#_hybrid-markdown-editor-experimental)
introduced an experimental browser editor that keeps Markdown source canonical,
renders inactive blocks, and reveals source near the cursor. Its
[extension integration](https://github.com/microsoft/vscode/blob/main/extensions/markdown-language-features/markdown-editor-src/editor.ts)
keeps file operations in the host. The published `@vscode/markdown-editor`
package exposes the model, view, controller, history, baseline, and comment
contracts.

[Obsidian Live Preview](https://help.obsidian.md/Live%2Bpreview%2Bupdate) uses
the same product model on CodeMirror 6. Its public
[editor extension documentation](https://github.com/obsidianmd/obsidian-developer-docs/blob/main/en/Plugins/Editor/Editor%20extensions.md)
describes decorations and widgets, but Obsidian's complete implementation is
closed source.

The VS Code package is still pre-1.0 and experimental. Active issues report
gaps in [list editing](https://github.com/microsoft/vscode/issues/329464),
[raw HTML rendering](https://github.com/microsoft/vscode/issues/329412), and
[link editing](https://github.com/microsoft/vscode/issues/328937). Kandev needs
an integration boundary that can absorb upstream changes or support a future
engine replacement.

## Decision

Kandev will use a source-preserving hybrid engine for `.md` Edit mode. The
initial implementation will integrate an exactly pinned
`@vscode/markdown-editor` version through one Kandev-owned adapter.

The plain Markdown buffer remains authoritative. The upstream model is a
transient presentation and editing model. It does not own file persistence,
dirty state, comments, links, or remote-update policy.

Kandev will keep its sanitized `react-markdown` renderer for Preview mode. It
will keep Monaco and CodeMirror for Source mode. MDX will use Preview and Source
only. Unsupported Markdown blocks will remain editable source.

No production component outside the adapter may import the experimental
package. Exact source-preservation, lifecycle, link, comment, and failure tests
guard the adapter. Package upgrades are deliberate dependency changes, not
automatic compatible-range updates.

## Consequences

Users get the interaction model demonstrated by VS Code and Obsidian without
turning repository Markdown into a lossy rich-document format. Preview remains
safe and visually complete. Source mode remains available for unsupported
syntax, advanced edits, and recovery.

Kandev must maintain visual alignment and state handoff across three
presentations. The hybrid package can change quickly or remain incomplete.
The adapter, exact pin, source corpus, and Source fallback contain that risk.

The first implementation adds a frontend dependency and an editor lifecycle.
It does not replace Kandev's general editor provider or backend file APIs.

## Alternatives Considered

- Use Tiptap for repository files. This would reuse Kandev code, but unsupported
  Markdown can be normalized or lost during document serialization.
- Build Monaco decorations for rendered blocks. Monaco supports inline and
  block decorations, but it does not provide the required editable block layout
  or source-range replacement model without substantial custom work.
- Rebuild Obsidian Live Preview on CodeMirror 6. This preserves source, but it
  replaces the current editor-provider boundary and requires Kandev to own the
  Markdown parser, decorations, widgets, selections, and block editing engine.
- Keep only separate source and preview surfaces. This preserves fidelity, but
  it does not provide rendered context while the user edits.
