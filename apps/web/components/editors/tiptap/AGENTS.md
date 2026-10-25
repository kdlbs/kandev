# Tiptap editors

- Treat plugin, voice, and programmatic insertion as a plain-text contract unless rich text is explicitly required. Pass explicit text nodes or slices to Tiptap insertion APIs; do not pass arbitrary strings where they can be parsed as HTML.
- Reconciliation from store or prompt changes must derive ranges from the full serialized document, preserve Markdown code and link contexts, apply mapped or reverse-order edits, and preserve collapsed and range selections in the adapter's text coordinates. Keep presentation-only reconciliation out of undo history. Add focused tests for multiple paragraphs, fenced code, literal HTML-like text, Unicode, and caret or selection stability.
