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
| .6, .7 | Measurement and lifecycle; Section and child geometry |
| .8 | Mobile composition; Section and child geometry |
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

`ChangesTimelineViewport` refreshes presentation measurements through
`observeChangesTimelinePresentationChanges`. Invalidating the virtualizer's
cache does not cause an unchanged mounted element to emit another
`ResizeObserver` entry. A refresh must therefore repopulate mounted row sizes,
including rows whose dimensions did not change, before restoring its anchor.
Conservative estimates remain valid for unmounted rows only.

Capture the anchor and collect the mounted wrappers' sizes before clearing the
cache. Batch layout reads before publishing sizes. Accept positive current
heights and retain each mounted row's previous positive keyed size when hidden
geometry is unavailable. Rebuild the virtualizer's measurements, apply those
sizes, then publish the pending anchor for restoration. This ordering prevents
an intermediate estimate-only layout from moving the visible entry.

Keep this explicit measurement pass limited to the mounted window on
presentation invalidation. Initial mounting and ordinary scrolling retain the
asynchronous `file-tree-measurement.ts` path. Do not measure the full collection,
fix all row heights, or enlarge overscan to mask missing measurements.

Capture the first visible key and offset before a same-context model change.
Restore that anchor when it survives. Otherwise use the nearest surviving index
and clamp the scroll position. Preserve scroll on ordinary background updates.
Explicit user navigation takes precedence over anchor restoration.

On task/environment replacement, clear transient selection, detail ownership,
focus requests, measurements, and anchors before new rows appear. Use the
current task/session/environment mapping and backend/auth context. Old requests
cannot publish into a replacement context, including an A-to-B-to-A transition.
Follow existing domain invalidation and unavailable-workspace rules.

## Section and child geometry

Flattening a section must preserve the spacing previously supplied by its
normal-flow container. `TimelineSection` places `pb-3` after the complete
section, while its header uses `mb-1` and `-mt-0.5`. Moving that footer padding
onto a standalone header creates a large header-to-content gap and removes the
separation before the next section. Singleton `space-y-0.5` lists also lose the
spacing that previously separated their sibling rows.

[PR #4145](https://github.com/kdlbs/kandev/pull/4145), inspected at head
`489f509f0446`, owns header padding removal and disclosure geometry. Preserve
its 28px desktop / at least 44px phone or coarse-pointer controls and direct
first-descendant adjacency. Do not restore the legacy negative header margin
or 4px header gap. Collapsed headers must remain compact.

Integrate that repair before measuring any remaining geometry defect. The
original 10px expanded-section separation, 2px sibling gap, and 16px content
gutter are reference targets for defects still present. Any required spacing
belongs inside measured history-row shells, after the final visible child for
section separation and between siblings for list gaps. Do not add an extra
trailing sibling gap. Preserve tree depth indentation and existing file-control
negative margins. Do not duplicate the colleague's header implementation.

Controlled expanded commits require the same treatment at their inner boundary.
Where a defect remains, separately rendered files must retain `CommitRow` content padding
and `CommitRowFiles` sibling spacing. Allocate the commit's trailing padding
after its final visible child, preserving the collapsed header's padding.
Repository and directory headers keep their existing indentation and controls.

Derive the section, list, and expanded-commit boundaries from semantic row inputs
when the model changes. Record any required leading/trailing spacing on the
descriptors or an equivalent local history-row shell projection. Recompute the
final visible row on collapse, expansion, and accepted data updates. Apply the
same spacing to the row's initial size estimate and its measured wrapper.
This work must account for history rows split before and after the working tree.
An empty working tree must not drop the separation between PR and Commits.

Virtual wrappers remain contiguous. Inner padding represents deliberate blank
space, so it participates in actual measurement and anchor restoration. An
absolute-positioned group wrapper cannot supply that measured space. Do not add
a global virtualizer gap, nested scroller, or fixed height to reproduce it.
Keep the existing presentation invalidation and mounted-row refresh algorithm.
Phone headers and rows retain their measured wrapping and 44px action targets.

Browser regressions measure inner header/content and section gaps separately
from wrapper contiguity. Cover PR plus commits, flat/tree files, expanded and
collapsed commits, empty sections, repository groups, and refresh/resize/reopen.
The [toolbar and section repair plan](../../../plans/changes-loading-feedback/plan.md)
records the comparison against the earlier normal-flow rendering.

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
- [Measurement refresh repair](../../../plans/changes-timeline-measurement-refresh/plan.md)
- [Toolbar feedback and section spacing repair](../../../plans/changes-loading-feedback/plan.md)
