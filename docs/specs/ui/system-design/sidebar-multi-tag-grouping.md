---
status: draft
system: ui
requirements:
  - REQ-UI-SIDEBAR-MULTI-TAG-001
---

# Multi-tag Combination Classification

## Boundary and existing contracts

This is an additive mode of the [saved-sidebar design](sidebar-plugin-grouping.md),
not a new host grouping engine. The dedicated yattdev/kandev-plugin-tags repository
owns its implementation. The host continues to consume generic complete snapshots.
No host/SDK production change is planned. ADR [0041](../../../decisions/0041-backend-owned-portable-user-settings.md)
retains preference ownership; [0023](../../../decisions/0023-active-workspace-cookie.md)
retains boot workspace resolution. No new persistence or architecture ADR is needed.

## Requirement mapping

| Criterion | Design section |
| --- | --- |
| 001.1 | Discovery and compatibility |
| 001.2 | Canonical combination construction |
| 001.3 | Identity and revision |
| 001.4 | Existing host projection |
| 001.5 | Bounded complete response |

All suffixes refer to AC-UI-SIDEBAR-MULTI-TAG-001.

## Discovery and compatibility

Keep manifest sidebar_groupings entry tags / Tag / sidebar-tags unchanged.
Add entry multi-tags / Multi-tag / sidebar-multi-tags with authenticated,
workspace-scoped action and max_body_bytes 16, matching the existing action.
Canonical reference: plugin:kandev-plugin-tags:multi-tags. Use the corrected
unreleased host floor 0.103.0 and a pinned compatible standalone SDK: no wire change.
Bump the local plugin manifest/Makefile package version from 0.19.0 to 0.20.0
for installable iteration; that is not release/publication authorization.
Provider copy follows the existing manifest-owned Tag label convention, not a
Tags-specific host translation or conditional. Native selectors discover the
second entry from catalog/boot metadata, including phone task-switcher surfaces.

## Canonical combination construction

HandleAction routes the new key to a distinct private snapshot builder using
readSidebarTagDoc and the existing strict v1/v2 validation, raw-before-dedup
application bound and read-without-migration-writes guarantees. Keep the old
sidebarTagSnapshot output unchanged. Share narrow text/color/revision helpers,
not an intermediate copy of the complete singleton snapshot.

For each task, union active human/agent applications, omit deleted tag IDs,
deduplicate and sort member IDs bytewise. Enumerate every nonempty subset.
Only observed subsets become values; unused catalog tags do not create compound
values. Empty tag sets emit no membership; host projection supplies Unassigned.
Every subset produces exactly one task/value pair, independent of input order.
Only the host decides which records are accessible roots and repeats descendants;
child-only tags do not reparent or independently reclassify an inherited tree.

All combination IDs, including singletons, use set- plus the SHA-256 hex digest
of the JSON-encoded sorted member-ID array. This is independent of mutable display
names and safely distinguishes IDs containing punctuation. Multi-tag and Tag
references keep their group disclosure namespaces separate.

## Identity and revision

A value label joins JSON-quoted original tag names with ' + ' in canonical ID
order, for example "Backend" + "Frontend". Escape each raw name once, including
controls and literal backslashes; never quote a pre-escaped Tag label.
Quoting makes a literal plus sign inside one tag unambiguous. Singleton values retain their normalized tag color;
compound values omit color rather than imply a single tag owns the combination.
No generic host chip renderer or multi-color DTO is introduced.
Sort groups/memberships deterministically and reuse sidebarTagRevision's
classification-content hash. Name changes update labels/revision, singleton
color changes update color/revision; timestamps, application ownership and notes
remain excluded. Compound-only color changes need not alter that value's revision.
Combination identity does not change on rename/recolor. Existing Host-state
notifications invalidate both declared groupings. On tag deletion, its subsets
vanish and surviving subset identities persist.

## Bounded complete response

Reuse SDK constants MaxSidebarGroupingValues, MaxSidebarGroupingMemberships and
MaxSidebarGroupingResponseBytes, and the decoder's 256-byte label/ID contract.
Preflight subset counts by bounded addition/doubling, not an unchecked bit shift
or allocation of the full power set. A single set of ten distinct effective tags
already needs 1,023 values and must fail before enumeration. For smaller sets,
check cumulative per-task expansion and the global distinct-value union while
building, before accepting the next over-limit element. Raw input validation
still precedes inactive/deleted filtering and normalization. Check cancellation
during bounded batches and before response serialization.

Marshal once, verify actual encoded bytes and validate the final complete snapshot.
An overlong quoted label, value/member overflow or response above 1 MiB returns
an error and no successful snapshot. Do not shorten labels, cap tags per task,
truncate subsets, increase limits or return complete:false as a healthy response.
Only emitted observed subsets consume the encoded-label budget; an unused valid
catalog definition does not fail Multi-tag merely because its Tag label expands.
Existing unavailable/retained-stale/retry behavior owns presentation and recovery.
Tag's original action remains independent of combination-only overflow.

## Existing host projection and UI

Use the existing authorized query and SQL occurrence windows unchanged: complete
classification before slicing, label ordering/value-ID tie-break, Unassigned last,
canonical hydration/task actions, per-occurrence continuations and group-scoped
collapse. Do not enumerate combinations in the browser or query plugin state from
host code. Saved Multi-tag views use ordinary workspace journals and copy/reload
behavior. Provider generation, revision and context fences remain authoritative.

Desktop adds one ordinary provider menu entry; phone adds one ordinary internally
scrolling 44px choice in its native grouping sheet. Existing keyboard/Escape/focus
and safe-area behavior applies. Composite labels wrap or truncate using current
heading rules; a complete label remains available through existing accessible
text. No desktop crop or new scroll owner is introduced.

## Verification

Permanent plugin tests cover powerset membership, stable identity, mutation
revision, duplicate/inactive/deleted inputs, corruption, zero writes, exact group/
membership/byte/label bounds and cancellation. Keep existing Tag regressions green.
Extend the generic host native fixture scenario with overlapping subset values,
100+ roots, repeated descendants, pagination/collapse and a foreign workspace.
Do not add a production Tags-specific fixture/allowlist or an external-repository
artifact requirement to the default CI suite.

Real 0.20.0 package smoke is mandatory in a NEW isolated owned runtime: both
selectors, saved view reuse/reload, duplicate identity, live mutation, overflow/
recovery and desktop/phone behavior. The currently running user demo on port52121
must stay untouched until the user explicitly requests its upgrade.
No implementation, tests, package build, commit, push or deployment is authorized
by this design turn; no prior review streak certifies the future extension.
