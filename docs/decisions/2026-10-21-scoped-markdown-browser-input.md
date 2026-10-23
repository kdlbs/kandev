# ADR-2026-10-21-scoped-markdown-browser-input: Scope Markdown input to its editor

**Status:** accepted
**Date:** 2026-10-21
**Area:** frontend

## Context

The user requires rendered Markdown Edit across current browsers. Zen on
Firefox 158 reports no EditContext. The pinned VS Code Markdown engine creates
EditContext unconditionally, so the existing capability fallback disables Edit.
Chromium works at the same HTTP Tailscale URL. This is an input dependency gap,
not evidence that HTTPS can repair Firefox support.

The existing engine owns source-offset editing, rendered table cells, undo,
comments, and caret geometry. Inspection of `0.0.2-84` found one context
construction, two attachment assignments, and no runtime EditContext instance
checks. Its controller already consumes text updates through a narrow context.

## Decision

Retain the source-preserving engine behind `HybridMarkdownEditor`. Add a
scoped input-transport seam through an exact-version pnpm patch. Keep native
EditContext behavior where available. Use a stable native textarea transport
when it is absent or cannot initialize. Missing EditContext must not disable
Edit or silently select Source.

The textarea lives in a shadow root on the engine host. Native event retargeting
preserves the controller's root-target keyboard and clipboard checks. Keep the
engine's pointer selection, source offsets, canonical bytes, and history.
Do not patch browser globals, document selection, or focus. Do not redirect
other editors through this transport.

Kandev owns composition synchronization, native-to-source offset mapping,
clipboard command ownership, caret positioning, accessible input, and disposal.
Native selection changes have a separate callback into model selection. They
must not enter the text-edit, dirty-state, or history path.
The patch includes matching runtime and declarations and remains reproducible
under a frozen install. It must fail visibly on incompatible package upgrades.
It must not grow into a parser or renderer fork. An upstream seam can replace
the patch only after equivalent contract tests pass.

Source continues to use Monaco. Native Firefox regression testing found that
pristine `monaco-editor@0.55.1` passes reversed offsets to the native textarea
and treats backward selection as a composition replacement. Maintain a second,
exact-version pnpm patch for native range/direction handling and input-delta
calculation. Preserve external selection direction. Cover the actual package
runtime and exact browser replacement before any upgrade. This patch does not
change Source draft ownership or route Source through the hybrid transport.

The UI system owns this reusable editing contract through
`REQ-UI-MARKDOWN-FILE-EDITING-001` through `-003` and their
[system design](../specs/ui/system-design/markdown-file-editing.md).
The [initial engine decision](2026-08-27-source-preserving-hybrid-markdown-engine.md)
continues to govern source preservation and host ownership. This decision
replaces the later assumption that Source fallback establishes browser support.

## Consequences

Users retain the existing rendered editing model on Chromium, Firefox/Zen,
and Safari. Kandev owns a bounded native-input subsystem and two package patches.
Native Firefox/WebKit, real mobile keyboards, IME, and assistive technology
require separate evidence. A constructor probe or simulated events cannot
establish those results.

The textarea's normalized line endings never become canonical source.
Range mapping must preserve mixed line endings, BOM, and exact undo bytes.
An actual editor failure still preserves the draft in Source and supports retry.

## Alternatives Considered

- **Install a global EditContext polyfill.** The available
  [installer](https://github.com/neftaly/editcontext-polyfill/blob/c3b3b1571542780f3b02edac9ab7e90cee69b821/src/install.ts)
  changes global selection, focus, and legacy commands. Those layers overlap
  this engine's source selection. Its [documented limits](https://github.com/neftaly/editcontext-polyfill/blob/c3b3b1571542780f3b02edac9ab7e90cee69b821/README.md)
  include misplaced IME input. Audited source can inform the transport, with
  its license and revision retained. The unmodified installer is not selected.
- **Replace the engine with CodeMirror Live Preview.** CodeMirror provides
  compatible native input. It would also require a new rendered table-cell
  editor, selection bridge, and exact-byte history bridge. A local probe showed
  that its document representation normalizes mixed line endings. That work
  is larger than isolating the existing input seam.
- **Offer only Preview and Source without EditContext.** This preserves the
  draft but fails the user's browser-parity requirement.
- **Convert repository Markdown into a rich document model.** This retains
  the normalization risk rejected by the initial engine decision.
