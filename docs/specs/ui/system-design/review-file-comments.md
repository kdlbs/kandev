---
status: current
system: ui
requirements:
  - REQ-UI-REVIEW-FILE-COMMENTS-001
---

# Review file comments design

## Context and boundaries

The current `DiffComment` requires start/end lines, a diff side, and code content.
`ReviewDiffHeader` renders a toolbar but has no whole-file comment entry point.
The comments store already persists pending feedback in sessionStorage.
This change extends that local interaction; it introduces no backend endpoint,
provider write, database migration, or new persistence tier.

The request is interpreted as feedback in the local Review surface, following
its existing Fix comments workflow. This is a design assumption, not a request
to publish comments on GitHub or another provider.

## Model and identity

Add `ReviewFileComment` to `lib/state/slices/comments/types.ts` with
`source: "review-file"`, the existing comment base, `filePath`, and
`repositoryName` (including an explicit empty string for the workspace root).
Retain `repositoryId` when resolved, plus the file's explicit `base_ref` and
`is_submodule` as optional `baseRef`/`isSubmodule` comment metadata. Store
hydration and edits preserve that parent-gitlink context. Do not add line, side,
or code fields.
Add a `ReviewComment = DiffComment | ReviewFileComment` union and
`isReviewComment` guard for aggregate review consumers. Keep `isDiffComment`,
line selectors, and `commentsToAnnotations` restricted to actual line comments.
This avoids manufacturing line 0 or changing line-renderer assumptions.

Resolve identity from the current ReviewFile and review repository mapping.
Scope file comments by session plus `reviewFileKey`'s repository-name/path
identity; preserve the repository ID for consumers that use it. Do not treat
an unresolved named repository as the root or apply legacy path-only wildcard
matching to new file comments. Preserve submodule names, including when a
nested scope shares its parent's repository ID. Follow existing review identity
rules from `components/review/AGENTS.md`; do not rewrite legacy line rows.
New line comments also retain the optional repository name supplied by either
diff viewer, the Monaco/CodeMirror file editors, or Markdown preview. Grouping, annotation selection, counts, composer navigation, and
agent delivery use that explicit scope. Existing rows without a name retain
legacy matching; ID-only rows bridge aggregate groups only when their mapping
to an observed name is unambiguous. Aggregate composer groups keep unknown
legacy scope separate from explicit workspace-root scope; inferred names are
carried only in the presentation copy, without rewriting persisted rows.

Existing store add/update/remove and hydration actions own persistence and
pending IDs. No storage-key migration is needed. Old comments load unchanged.
Draft editor state is bound to the original file/session identity and resets
when that context changes; a switch cannot submit text into another file.

## Interaction and composition

Wire a Comment on file callback from `review-diff-list.tsx` through
`ReviewDiffHeader` to `FileDiffToolbar`. Desktop uses a visible header action
with an accessible name and tooltip; phone uses the existing file actions menu.
Opening the action expands the file section if needed and focuses a proposed
`ReviewFileComments` region immediately below the sticky header, above the diff
or patchless-state message. Keep this region outside diff lazy-rendering and
Markdown-preview branches. Existing file comments remain visible in an expanded
section. Collapsing hides the body but preserves saved comments and counts.

Each saved card follows the line-comment visual language: compact card surface,
blue comment icon, whole-file label, monospace body, and top-right edit/delete
icons. Whole-file cards align to the file section’s left padding rather than
the line-number gutter. Desktop actions appear on hover or keyboard focus; phone/coarse-pointer
actions stay visible with 44px targets. The file header supplies the file context;
repeat the full path only while creating a comment and keep it screen-reader
accessible for saved comments. The region also has an inline form with Add and Cancel. Reuse the existing comment form and Markdown presentation
patterns where applicable, with scoped touch sizing; do not expose a new Run
button in this slice. Saving adds to existing pending review feedback. Cancel
and Escape abandon only the current edit and return focus to the file action.
Blank or whitespace-only drafts cannot submit. Long content wraps.

Phone composition follows the existing `MobileFileActionsMenu` and
`MobileHeaderIdentity`: one file-focused flow in the review scrolling body.
The menu closes before the editor receives focus. Inline editing avoids
stacking another sheet over an already full-height review surface. The review
body remains the vertical scroll owner; retain the existing dynamic viewport
and safe-area handling. Use `useResponsiveBreakpoint` for phone composition;
new phone/coarse-pointer targets are at least 44px, ordinary desktop controls
28px. The curated mobile picker-sheet precedent supplies the focus and
single-scroll-owner discipline, without importing a task-domain picker.

## Aggregate feedback and delivery

Widen the review dialog pending filter, counts, top bar, FixCommentsButton,
ReviewCommentsOverview, and `useReviewDialog` send callback to ReviewComment.
Group whole-file comments by full repository/file identity, including nested
repository names. The overview and chat attachment render a localized File
comment label and omit line/side/code metadata for this variant.

Extend `formatReviewCommentsAsMarkdown` to accept ReviewComment and output a
whole-file location and feedback without a code fence or line suffix for file
comments. Include repository name (and ID when useful for disambiguation) so
the agent does not confuse identical paths. Preserve existing line output.

Audit normal and passthrough composer aggregation, chips, remove handlers,
`formatCommentsForMessage`, and `use-run-comment.ts`'s exhaustive source switch.
Carry the new variant into review feedback throughout; do not widen selectors
used by line annotations. Names implying diff-only data should remain narrow
or be replaced with explicitly named aggregate review selectors at consumers.
Trace current callers of `usePendingDiffComments` and `DiffComment` before edits.

## Failure, compatibility, and security

Keep the current send contracts. In particular, Fix comments currently clears
pending feedback when its fire-and-forget send starts and reports a later
failure without restoring it. Do not claim this task repairs that behavior.
Other composer routes retain their current success/error semantics. Reuse
existing safe comment rendering; never enable raw HTML. Pending text retains
sessionStorage's existing browser-local limitations. No new telemetry is needed.

## Requirement mapping

| Criteria suffix | Design coverage |
| --- | --- |
| 001.1, 001.2, 001.6 | Interaction and composition |
| 001.3 | Model and identity |
| 001.4, 001.5 | Aggregate feedback and delivery; failure and compatibility |

All suffixes refer to AC-UI-REVIEW-FILE-COMMENTS.

## Related contracts

[Review file status](../requirements/review-file-status.md) defines patchless
file visibility. [Comment Markdown](../requirements/comment-markdown.md) owns
shared rendering. [Sessions and review](../../../public/sessions-and-review.md)
documents the existing local storage and delivery limitations.
