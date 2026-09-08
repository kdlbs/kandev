# ADR-2026-08-27-mixed-git-change-facets: Preserve mixed Git changes as path facets

**Status:** accepted
**Date:** 2026-08-27
**Area:** backend, agentctl, frontend, protocol

## Context

Git porcelain reports the index and working tree independently. A tracked path can therefore have a
staged edit and a later unstaged edit (`MM`), while a newly staged path can receive another unstaged
edit (`AM`). Kandev currently collapses both columns into one `FileInfo`. The parser prioritizes the
working-tree column, so the file appears only under Unstaged, and its `HEAD`-relative diff merges both
layers.

The file path is still one mutation and review identity. Duplicating the path in the status map would
distort changed-file counts, path-based stage operations, multi-repository keys, carry-forward logic,
and Review deduplication. Replacing the wire shape outright would also break stored snapshots and
older frontend consumers. The model needs to retain one path identity while exposing two independently
selectable change layers.

## Decision

`GitStatusUpdate.Files` remains a map with one entry per repository-relative path. `FileInfo` keeps
its existing flattened fields and adds two optional facet objects: `staged_change` and
`unstaged_change`. A facet carries status, additions, deletions, old path, diff content, and diff skip
reason.

Agentctl emits both facet objects when the porcelain index and working-tree columns both describe a
change. Single-layer paths retain the existing compact shape. For mixed paths, the flattened fields
remain the existing compatibility projection: worktree-priority classification plus the combined
`HEAD`-to-working-tree diff. The staged facet is computed from `HEAD` to the index, and the unstaged
facet from the index to the working tree.

New consumers use the facets when present and fall back to the flattened fields otherwise. Changes
may project two temporary rows, but raw status, Review aggregation, badges, and Git mutations remain
keyed once by `(repository, path)`. A diff target is keyed by `(repository, path, layer)` so opening
the staged and unstaged rows cannot resolve to the same content or pinned panel.

Every serialized diff representation participates in the existing total snapshot budget and retains
the existing per-representation output cap and skip reasons. Facet metadata survives even when its
diff content is skipped. Git unmerged/conflict states are not redefined by this decision.

## Consequences

- Desktop and mobile Changes can show one path in both Staged and Unstaged with accurate layer stats
  and diffs.
- Existing snapshots and clients remain decodable because the flattened fields stay present and the
  new fields are optional.
- Mixed entries can serialize more diff content than single-layer entries, but the shared snapshot
  threshold still bounds total enrichment.
- Frontend equality checks, diff identities, and focus fingerprints must include the layer/facet data
  or facet-only updates can be missed.
- Stage, unstage, discard, Review deduplication, and changed-file totals must never treat a projected
  facet row as a second path mutation.

## Alternatives Considered

### Add two status-map entries with synthetic path keys

Rejected. Synthetic keys would leak into path operations, changed-file counts, multi-repository
identity, editor synchronization, and Review deduplication.

### Replace `FileInfo` with an array of layer records

Rejected. This is a breaking wire and stored-snapshot migration for a state that can be represented
additively. Older consumers would fail to decode or render ordinary status snapshots.

### Keep one record and concatenate staged and unstaged diffs

Rejected. A separator can make both hunks visible, but it cannot give each section accurate status,
line totals, selection identity, or staging transitions.

### Fetch a layer-specific diff only after row selection

Rejected for this repair. It avoids duplicate streamed diff content but adds a new request lifecycle,
loading and failure states, and cache invalidation path to a panel that already receives bounded diff
data. The existing snapshot budget is sufficient for the additive facet contract.
