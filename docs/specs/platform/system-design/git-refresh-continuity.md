---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
created: 2026-10-06
owners:
  - kandev
---

# Git Refresh Display Continuity

## Boundary and requirement mapping

Platform owns the Git observation and freshness contract, including its visible continuity.
Tasks supplies environment bindings. UI supplies the existing desktop panels and phone drawer.
This design extends [workspace status](workspace-git-status.md) without changing backend publication or wire formats.

| Criteria | Design section |
| --- | --- |
| 001.21, 001.25, 001.27, 001.47 | Display projection and scope |
| 001.24, 001.29, 001.48 | Mounted viewer and freshness |
| 001.49 | Scroll continuity |
| 001.10, 001.11, 001.33 | Scope and readiness |

## Source evidence

`captureBasicGitStatus` marks all file details pending when the observation fingerprint changes.
`publishGitStatus` already preserves ready details when that fingerprint remains identical.
`applyGitStatus` replaces the accepted authoritative snapshot without previous enrichment.
Its tests require missing counts and patches to stay missing in a newer pending snapshot.

`mapToChangedFiles` forwards those missing counts. Desktop `FileRowStats` also suppresses pending counts explicitly.
`buildReviewSources` converts missing patches to empty strings and missing counts to zero.
`ReviewFileDiffContent` replaces the viewer with `ReviewDiffStatePlaceholder` for pending or unavailable detail.
`TaskChangesPanel` uses that renderer through `ReviewDiffList`, including file-specific dockview panels.
The placeholder unmounts the viewer and shrinks its parent's scrollable content.
`FileDiffViewer` already memoizes transformed data by path, patch, and status.
`SessionRead` already retains previous cumulative data during ordinary invalidation; reuse that behavior.

## Display projection and scope

Keep authoritative `gitStatus.byEnvironmentRepo` and ordering unchanged.
Add a runtime display companion owned by the app store, below components and routes.
Use a small pure reconciliation helper beside `git-status-state.ts` and shared domain selectors.
Store at most one latest ready representation per live environment, repository, path, and change layer.
Flattened, staged, and unstaged patches are separate representations.
Reuse immutable patch strings rather than copying or parsing them into the companion.
The companion follows the live Git-state lifecycle; it is neither persisted nor included in boot hydration.
Its text remains subject to existing per-file and snapshot diff limits. No revision history accumulates.

Process only accepted authoritative snapshots. Rejected late results cannot update the display companion.
Ready files replace their representation; pending or failed details retain the eligible previous representation.
A complete accepted membership snapshot immediately prunes absent files or consumed layers.
Incomplete or failed membership cannot prove removal.
An accepted ready empty patch is a real replacement, not a signal to preserve old content.
Ready binary, skipped, or truncated representations retain their actual skip semantics.
Mixed snapshots update ready siblings while preserving eligible pending or failed siblings independently.
When a compact single-layer row omits explicit facet fields, cache its flat patch under the row's actual staged or unstaged identity.
Project compact pending rows from that matching layer only; never reuse a mixed flattened patch across a layer replacement.

Bind eligibility to the canonical environment and repository, checkout generation, branch/HEAD, and comparison identity.
Use `gitCheckoutGeneration`, `bumpSessionGitCheckoutGeneration`, and `clearGitStatus` lifecycle paths.
Reset, branch switch, changed HEAD, environment replacement, and comparison retarget retire affected representations.
Missing enrichment-only comparison metadata does not prove a retarget; use the explicit target and its invalidation path.
Do not invalidate display content solely for a tracker lifetime, timestamp, or publication revision change.
Sibling repositories and sessions sharing one environment keep their own correct scope.
A new consumer can reuse an eligible shared display snapshot; local component refs alone are insufficient.

Expose current membership/readiness separately from displayed values and their previous snapshot provenance.
Never write retained patches or counts into a newer authoritative snapshot or label them as ready for that observation.
Keep membership and whole-file mutation decisions based on current raw state.
Carry the display projection through `deriveSessionGitValues`, `git-change-facets`, Changes mapping, and review sources.
Do not turn unknown counts into confirmed zeros before selecting displayed values.

## Mounted viewer and freshness

`ReviewFileDiffContent` chooses a placeholder only when no eligible displayed representation exists.
During refresh, it keeps the same `FileDiffViewer` mounted with the last displayed patch.
Use stable keys for the target, not detail state, timestamp, revision, or patch hash.
When normalized patch, status, and relevant display metadata are unchanged, reuse the existing content reference.
Update freshness independently of the editor data, preserving the existing memoization.
Do not use a full-list loading condition to replace a populated diff panel.

Use existing localized loading/unavailable copy in a compact status outside the diff body.
The Changes toolbar remains the list's shared status owner. An independent diff panel also needs accessible freshness.
Phone status remains visible without hover in the existing full-height diff drawer.
Keep toolbar height, drawer Back/dismiss, focus return, safe areas, and existing scroll owners.
Initial loading or unavailable content without a prior representation keeps the existing placeholder.

Retained content remains readable, foldable, and copyable.
Disable patch-dependent revert, new line comments, review marking, and automatic marking while its current detail is not ready.
Carry the comment readiness guard through both providers and their submit handlers; keep an open draft visible but non-submittable until the current patch is ready.
Keep path-based whole-file actions under their existing membership and operation guards.
Do not reset review hashes to an empty pending patch or certify an old displayed patch as newly reviewed.
On ready replacement, compare review state against the actual new patch.
Existing comments can remain visible without submitting them against a new unseen patch.

## Scroll continuity

For unchanged data, keeping the viewer and scroll owner mounted preserves the reading position without restoration work.
Do not rerun selected-file navigation or walkthrough jumps because freshness alone changed.
Preserve wrap, folding, expanded context, selection, and comment drafts for unchanged content.

For changed data, capture the first visible file/line, side, its content, nearby same-side context, and its offset before the renderer commits replacement content.
`ReviewDiffList` owns its external scroll container. It restores the anchor after layout, including changes above the visible file.
Use the existing `suppressAutoMarkRef` guard during programmatic restoration.
Map the anchor to the same surviving content on that side and preserve its viewport offset, including when inserted or deleted lines shift its number.
If the anchored line was removed, use nearby surviving content on that side; use numeric proximity and clamped prior offsets only when content context is unavailable.
Monaco applies equivalent content mapping to its internal viewport and selection when restoring the saved editor state.
Expanded context can reset when the selected hunk no longer exists.
Cancel stale restoration when the target changes or the user scrolls after capture.
Restoration cannot navigate to a different file or steal focus.

Pierre uses rendered line metadata and the list's external scroll owner.
Monaco uses its editor view-state APIs for internal scroll, selection, and folding, plus the surrounding list anchor.
Apply each adapter only when its renderer is active. Do not introduce another scroll container.
Do not restore editor state across environment, repository, source, or layer replacement.

## Verification and delivery

Unit tests cover accepted-only retention, mixed ready/pending/failure snapshots, identical results, removal, and scope retirement.
Rendered tests prove viewer mount identity and patch-action guards through pending, failure, and ready transitions.
Provider tests prove unchanged view state, inserted/deleted-line mapping, removed-anchor fallback, and user-scroll cancellation for Pierre and Monaco.
Desktop and phone Playwright use disposable repositories and `createGitEnrichmentGate` after initial ready content.
Do not use the existing bridge's dropped-pending behavior to bypass the transition under test.
Hold real enrichment, change an unrelated file, and verify visible counts, readable content, and scroll position continuously.
Release enrichment for identical selected-file content, then repeat with a changed selected-file patch.
Include insertion and deletion before the visible line inside the selected file, user-scroll cancellation, initial load, removal, and comparison or checkout replacement.

The [repair plan](../../../plans/git-refresh-continuity/plan.md) owns implementation and browser proof.
The [display snapshot decision](../../../decisions/2026-10-06-git-display-snapshots.md) preserves the freshness boundary.
