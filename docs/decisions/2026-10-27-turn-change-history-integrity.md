# ADR-2026-10-27-turn-change-history-integrity: Separate captured evidence from turn comparison

**Status:** accepted
**Date:** 2026-10-27
**Area:** backend

## Context

Turn cards compare complete checkout trees. A rebase can therefore place upstream
changes beneath an agent reply. The stored objects and counts remain correct,
but the presentation suggests a narrower change boundary.

The investigated turn retained 320 paths across a rebase and reset. Its final
branch patch contained five documentation files. The original snapshots lack
the real HEAD and comparison-base evidence needed for reliable history adjustment.

Capture processing also outlives a turn's writing interval. An unfinished export
must not extend writer attribution or manufacture overlap with a later turn.
The user requires the presentation from PR #4831 and no additional routine UI complexity.

## Decision

Preserve fresh, immutable start and end snapshots under the existing
[interval decision](2026-10-07-immutable-turn-change-intervals.md).
For new captures, retain versioned provenance with the real HEAD, configured
comparison-target identity, target commit, and unique merge base.

Accept a separate, versioned comparison pair before storing files or content.
When a stable target's merge base advances, transplant the starting snapshot
onto the ending base through a clean three-way tree merge.
Compare that derived baseline with the captured end tree.
Never resolve a normalization conflict automatically.

Unavailable or conflicting evidence produces unavailable turn comparison.
Legacy rows keep their original files and counts under explicit repository
snapshot semantics. Do not reconstruct their missing evidence from reflogs,
ACP messages, mutable refs, or the current checkout.

Derive writer intervals from exact terminal-boundary evidence and canonical
turn completion. Keep writer attribution separate from export progress.
Preserve content in bounded, resumable batches bound to the accepted comparison.

The [history integrity design](../specs/tasks/system-design/turn-change-history-integrity.md)
owns the eligibility, compatibility, persistence, and failure contracts.
This proposal refines comparison semantics without superseding immutable capture.
The implementation builds on [PR #4831](https://github.com/kdlbs/kandev/pull/4831).
Keep its card styles and controls. Routine adjustment and export details remain
internal; exceptional results reuse existing status areas and selected-diff explanations.

## Consequences

- Upstream updates can be excluded without erasing starting dirty content.
- Legitimate reverts remain visible; matching the final upstream tree is not a
  reason to discard a file.
- Conflicting normalization is explicitly unavailable. This can reduce coverage
  compared with a broad raw checkout diff.
- Provenance, derived comparison objects, and export progress need durable storage.
- Existing history remains readable but cannot receive retroactive accuracy claims.
- Concurrent or unknown external writers remain possible. No exclusive authorship
  claim follows from a clean history adjustment.

## Alternatives Considered

- Keep whole-checkout differences as turn edits: retains the misleading rebase result.
- Use only ACP write events: misses shell commands, generators, and unsupported agents.
- Use the final branch patch: includes earlier turns and loses the starting dirty boundary.
- Remove paths whose final content matches upstream: hides legitimate reverts and
  cannot distinguish upstream and local changes within one path.
- Replace the captured start after a rebase: destroys the original evidence and
  silently changes the historical interval.
- Choose a side of merge conflicts: invents a baseline that Git could not prove.
- Increase one export timeout: does not preserve acknowledged progress or diagnose
  missing content, and increases cancellation pressure.

## Related artifacts

- [Requirements](../specs/tasks/requirements/turn-changed-files.md)
- [System design](../specs/tasks/system-design/turn-change-history-integrity.md)
- [Fix plan](../plans/turn-change-history-integrity/plan.md)
