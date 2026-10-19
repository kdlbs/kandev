---
status: current
system: ui
requirements:
  - REQ-UI-PLAN-TEXT-SEARCH-001
---

# Plan Text Search System Design

## Purpose and boundaries

UI owns rich-text match derivation and presentation. This design operates on
the live Plan document without modifying its content or persistence. It uses
the existing [editor lifetime gate](plan-editor-task-switch-stability.md) and
does not change that contract. The owning [requirement](../requirements/plan-text-search.md)
defines observable search behavior.

## Requirement mapping

| Criteria for `REQ-UI-PLAN-TEXT-SEARCH-001` | Design section |
| --- | --- |
| `.1`, `.2` | [Searchable runs](#searchable-runs) |
| `.3` | [Position and decoration contract](#position-and-decoration-contract) |
| `.4`, `.5` | [Transactions and navigation](#transactions-and-navigation) |
| `.6` | [Desktop and phone](#desktop-and-phone) |

## Components and responsibilities

- `apps/web/components/editors/tiptap/search-highlight-extension.ts` owns
  `PlanSearchExtension`, `planSearchPluginKey`, `findMatches`, and inline search
  decorations. `PlanSearchMatch` remains a document range `{ from, to }`;
  `PlanSearchState` retains `query`, `matches`, and zero-based `current`.
- `TipTapPlanEditor` in `tiptap-plan-editor.tsx` registers this extension beside
  StarterKit, Markdown, Link, Highlight, Underline, CommentMark, list and table
  extensions. Marks are formatting, not inline structural boundaries.
- `usePlanFindShortcut` dispatches existing query/step/clear commands and reads
  counters from plugin state on transactions. `TaskPlanPanel` passes those
  values to the existing `PanelSearchBar`.

The implementation boundary is the matcher; public editor props, command names,
plugin key, styling, and hook ownership remain compatible.

## Searchable runs

Visit textblocks in document order, including nested blocks. For each
textblock, inspect its direct inline children in order. Append consecutive
text children regardless of their marks. Flush the accumulated run before
every non-text child and at the end of the textblock. Process a textblock once;
do not then scan its text children again through the outer descendant walk.

Run text is the exact concatenation of those text nodes. Never concatenate the
whole document's `textContent`, insert synthetic whitespace, or strip rendered
formatting by reparsing Markdown. An inline leaf/atom or non-leaf inline node
is an opaque boundary; do not search its label, attributes, or descendants.
The shipped Plan schema includes StarterKit hard breaks, which are boundaries.
A literal newline contained in a code-block text node stays part of that run;
it differs from a hard-break node. Separate code blocks remain separate.

Keep the escaped `RegExp(query, "gi")` behavior used by `escapeForRegex` and
`findMatches`. Reset its scan for each run. Do not add regex UI, locale folding,
Unicode normalization, overlapping matches, dependencies, or indexing caches.
An empty query returns an empty array before scanning. Traversal and run data
are ephemeral for each recomputation; no new persisted state is necessary.

## Position and decoration contract

In the document descendant walk, a textblock at position `pos` has content
starting at `pos + 1`. Its direct child's offset is relative to that content.
Record the first appended text child's absolute position as the run start.
Consecutive text children have adjacent positions because marks have no
document size; structural items terminate the run instead of creating gaps.

For a regex result at `index`, produce `from = runStart + index` and
`to = from + matchedText.length`. Both JavaScript string indices and
ProseMirror text positions use UTF-16 code units. Do not convert to code-point
counts or use lowercasing that can change string length. The same translation
works inside nested lists and table cells using their actual document position,
not a paragraph-only offset assumption.

Use the existing `Decoration.inline(from, to, ...)` for each logical match.
ProseMirror may split a range into multiple DOM spans at mark boundaries. All
fragments of the current range use the existing current-highlight class.
`matches.length` is the counter authority; DOM span count is not match count.
Search decorations must not alter marks, selection, document content, or undo
history. No CSS changes are required.

## Transactions and navigation

Keep query-setting and clearing behavior in the plugin's existing `apply`
paths. A new query scans `newState.doc` and resets `current` to zero. Ordinary
document-changing transactions with an active query rescan `newState.doc` and
bound the previous ordinal with the existing clamp. This includes mark changes,
text insertion/deletion, and replacement of the document through editor APIs.
Pure selection transactions require no scan.

Next/Previous retain the current modulo wrap behavior and `scrollToMatch`
range lookup. Match positions and navigation targets come from the same plugin
result list. Zero matches keep the command's existing no-op behavior. Clearing
search removes all decorations. Do not add identity restoration for a specific
occurrence after edits; the current bounded ordinal is the compatibility rule.

## Desktop and phone

The shipped exemplar is `SessionMobileLayout` and its `MobilePlanPanel` in
`apps/web/components/task/mobile/session-mobile-layout.tsx`. The phone Plan
destination mounts the same `TaskPlanPanel`, `TipTapPlanEditor`, and search
extension as desktop. Existing Plan panel scroll ownership, bottom navigation,
safe-area offsets, formatting controls, and search entry points are retained.
No responsive branch belongs in matching.

This is data derivation within existing UI, with no change to geometry, touch,
scrolling policy, entry points, or navigation interaction. Under the
`mobile-parity` data-normalization exception, real plugin tests plus the
existing shared-editor component tests cover phone semantics; no new mobile
Playwright spec or broad viewport audit is required. Desktop browser coverage
proves actual Markdown marks and decorations through the shipped panel.

## Verification boundary

Independently author `search-highlight-extension.test.ts` around the real
extension registered with Tiptap and real ProseMirror schema, state, and
transactions. Do not mock matching, plugin state, or decoration construction.
Exercise formatted phrases, mid-word marks, literal punctuation, nested
textblocks, structural boundaries, Unicode positions, navigation, and edits.
Use a test-only inline node for the opaque non-text boundary rule when it is
not present in the shipped schema; that fixture does not widen product support.

Use one focused addition to `e2e/tests/search/plan-search.spec.ts` to verify
Markdown-to-editor formatting, exact logical counter values, highlight text,
and wrapping. Assert complete text across highlight fragments rather than one
DOM span per hit. The delivery plan owns exact test names and commands.

## Related records

- [Plan editor task-switch stability](plan-editor-task-switch-stability.md)
  remains an independent lifecycle boundary.
- [Responsive Plan formatting](responsive-plan-formatting.md) remains an
  independent presentation boundary.
- [Delivery plan](../../../plans/plan-search-across-formatting/plan.md).

No ADR is needed for this local matcher correction: the requirement/design
pair preserves the boundary policy without establishing a repo-wide rule.
