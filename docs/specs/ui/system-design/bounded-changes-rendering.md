---
status: current
system: ui
requirements:
  - REQ-UI-BOUNDED-CHANGES-001
---

# Bounded Changes Rendering System Design

## Boundary and existing contracts

This design extends UI render isolation to the Changes timeline. It reuses
`@tanstack/react-virtual`, `PanelBody`, and current domain hooks.
No backend, schema, permission, feature flag, or persistence change is required.

The [task-surface design](task-surface-render-isolation.md) supplies positive
measurement and reveal patterns. The [row design](changes-file-row-containment.md)
retains desktop density and touch wrapping. The
[commit-navigation design](commit-file-navigation.md) retains source-aware detail
opening. This design changes inline row lifetime, not full diff viewers.

## Requirement mapping

| Criteria | Design sections |
| --- | --- |
| .1, .2, .3 | Timeline model; Virtual viewport; Complexity |
| .4, .5 | Identity and interaction |
| .6, .7 | Measurement and lifecycle |
| .8 | Mobile composition |
| .9 | Commit detail ownership; Failure states |

All criterion suffixes refer to `AC-UI-BOUNDED-CHANGES-001`.

## Timeline model

`ChangesPanelBody` owns one flattened ordered timeline and its existing
`PanelBody` scroll element. New `changes-timeline-model.ts` produces data-only
row descriptors. New `changes-timeline-viewport.tsx` renders the virtual window.
These names describe proposed files, not existing implementations.

Descriptors include section headers, repository headers, directories, working
files, commits, inline historical files, provider files, and inline status rows.
The model preserves `firstVisibleSection`, `mergeCommits`, and
`separateCommitHistories` behavior. Counts derive from complete domain inputs.
Section and repository headers remain in the sequence, including their actions.
Fixed Changes chrome and dialogs stay outside the virtual row lifetime.

Adapt `changes-panel-timeline.tsx`, `changes-panel-tree.tsx`,
`changes-panel-repo-groups.tsx`, and `changes-panel-pr-files.tsx` into model and
row adapters. Never treat a whole expanded collection as one virtual item.
A commit header and each inline file are separate items in the same viewport.
No per-repository or per-commit nested scroller is introduced.

## Virtual viewport

Use one virtualizer for all repeated timeline rows. Start with five overscan
rows on each side. Add at most one retained interaction row for focus or an open
menu. Do not retain every row that was once visible.

Before geometry exists, use conservative positive size estimates and a bounded
initial range. Never fall back to rendering the full collection. Collapsed
collections contribute headers only. Hidden panels disable unnecessary work and
must not mount all rows because their viewport reports zero height.

For a viewport no taller than 1,000 CSS pixels, the regression budget is at most
120 mounted timeline descriptors, including headers and the retained row.
Apply that budget to the entire panel, not separately to each collection.
Use a single `data-changes-timeline-row` marker for assertions. The budget is an
engineering guard, not a user setting or a cap on available entries.

Reuse existing row controls and translations. Avoid eager JSX arrays and
per-entry providers, hooks, or subscriptions outside the window. Pure metadata
can remain proportional to the available collection size.

## Identity and interaction

Encode keys as tuples, not delimiter concatenation. Include context, section,
repository, row kind, change layer, path, and complete commit/provider target as
applicable. Identical paths across repositories or historical sources must not
share a key. Keep separate occurrences in different sections distinct.

Lift section, repository, directory, and commit expansion above virtual rows.
Store selection by scoped file identity above the viewport. Adapt Changes use
of `useMultiSelect` without changing unrelated consumers. Translate identities
back to path/repository arguments at the existing operation boundary.
Range selection uses the complete eligible ordered sequence, not mounted DOM.
Preserve current selection order and collapsed-folder eligibility semantics.

Retain the row that owns focus or an open menu until dismissal or explicit
navigation. Keep only one such owner. Keyboard traversal across the window uses
logical indices: reveal the next entry, mount it, then focus its control.
Home/End and forward/backward traversal must not skip unavailable DOM entries.
Use list semantics and accessible position/set-size metadata where appropriate.
Do not add a tree role without implementing its keyboard contract.

If an update removes the owner, close its transient menu and focus the nearest
surviving logical row. Fall back to the section control when empty. Destructive
confirmation remains panel-owned and retains the captured source identity even
when the original row leaves the window. Cancellation restores focus by key.

## Measurement and lifecycle

Measure row wrappers, including gaps. Touch wrapping and commit messages require
variable heights. Only positive measurements replace cached sizes, following
`file-tree-measurement.ts`. Width, font, locale, and pointer-mode changes trigger
remeasurement. Zero-height observations during hiding retain estimates or the
last positive measurement.

Capture the first visible key and offset before a same-context model change.
Restore that anchor when it survives. Otherwise use the nearest surviving index
and clamp the scroll position. Preserve scroll on ordinary background updates.
Explicit user navigation takes precedence over anchor restoration.

On task/environment replacement, clear transient selection, detail ownership,
focus requests, measurements, and anchors before new rows appear. Use the
current task/session/environment mapping and backend/auth context. Old requests
cannot publish into a replacement context, including an A-to-B-to-A transition.
Follow existing domain invalidation and unavailable-workspace rules.

## Commit detail ownership

`CommitRow` currently owns `expanded` and `hasExpanded`, and retains a hidden
`CommitRowFiles` subtree after collapse. Move expansion and inline detail state
above virtual rows. Remove that hidden subtree from the virtual timeline.

Reuse `requestCommitDetail` and its source restrictions through a timeline-owned
controller. Keep `useCommitDetail` behavior for standalone detail panels unless
a shared extraction preserves their tests. No request starts for a collapsed,
never-opened commit or because its header enters the viewport.

On explicit expansion, request the complete target once. Normalize the response
into inline metadata and release patch bodies from this controller. Retain
expanded metadata independent of whether the header is onscreen. A collapsed
commit can reuse a recent snapshot on reopen. Bound collapsed snapshots to eight
targets and 50,000 total file descriptors, evicting least-recently-used entries.
A snapshot whose header remains mounted retains the existing cached-reopen contract.
Only unmounted, collapsed targets are eviction candidates. An evicted reopen
performs a normal request. Expanded metadata is active data,
not an inactive-cache entry. Collapse of an oversized snapshot releases it.

Fence completions by context generation and complete target. Coalesce an
outstanding request for that target. Retire owners on context change and ignore
obsolete completions. Retry is explicit after failure. Provider failure never
falls back to local Git. Scrolling adds no requests or repeated error toasts.
This local controller is not a new global server-state cache.

## Complexity

`buildChangesTree` currently searches sibling arrays for each directory.
Replace sibling scans with per-directory maps during construction, followed by
the existing directory-first sorting. Preserve chain-collapse and path identity.
Use iterative traversal where depth depends on input. Avoid spread operations
that pass tens of thousands of children as function arguments.

Memoize the data model against semantic input and expansion changes. Viewport
scrolling must not rebuild the complete tree. Construction is proportional to
path segments plus sorting, rather than repeated sibling scans. Tests use a
wide tree and a deep tree, in addition to the incident-shaped cache directory.

## Mobile composition

The existing Changes bottom-navigation action opens `MobileChangesPanel`.
This focused content surface suits frequent file browsing. The nearest exemplars
are `MobileChangesPanel` and `TouchFileRowContent`, not compressed desktop panes.

Keep the fixed header and bottom navigation. `PanelBody` remains the only
content scroll owner and retains dynamic-height and safe-area behavior. The
file identity opens the diff. The visible ellipsis opens the existing responsive
menu. Touch targets remain at least 44px; desktop inline controls retain density.
Shared models and actions serve both compositions. Phone wrapping uses actual
measurements and never overwrites saved desktop layout preferences.

## Failure states and validation

Existing loading, empty, retry, disabled-action, comparison, and workspace
recovery states remain authoritative. Status rows participate in measurement.
No new copy is expected. Any necessary copy requires all supported locales.

Pure tests cover ordering, keys, expansion, range selection, and pruning.
Component tests cover window bounds, focus, detail ownership, and lifecycle.
Browser tests use real virtualizer measurement and 50,000 metadata entries.
A smaller real-Git fixture verifies transport, staging, and source routing.
Use synthetic authorized payloads for stress, not 50,000 filesystem writes.

Record DOM counts, main-thread long tasks, and heap observations before/after
on an isolated production build. Row bounds and user outcomes gate CI. Heap
values and elapsed time remain diagnostic, because machines differ.

## Decisions and implementation

No ADR is required. This local extension reuses the existing virtualizer and
UI ownership. The design records sufficient rationale. Truncation loses access,
CSS hiding retains components, and nested scrollers weaken navigation.

- [Implementation plan](../../../plans/bounded-changes-rendering/plan.md)
