# ADR-2026-11-07-explicit-worktree-base-selection: Explicit worktree bases require strict resolution

**Status:** accepted
**Date:** 2026-11-07
**Area:** backend, persistence, workflow

## Context

[Issue #5142](https://github.com/kdlbs/kandev/issues/5142) reports preparation
from the repository default after an explicitly selected branch cannot resolve.
A temporary native Git reproduction confirms that the current manager returns
success and persists the wrong-base worktree.

The same fallback intentionally recovers inherited stale parent bases.
The current branch string and repository default do not distinguish those cases.
The local-first refresh decision permits an older copy of the selected local
branch. It does not authorize another branch as the source.

## Decision

Make strict resolution the zero-value missing-base policy. A supplied base,
including a supplied repository default, resolves exactly or stops preparation.
Automatic substitution requires separately recorded inherited/default intent.
Keep that intent in existing attachment metadata, bound to the effective branch.
Pass a typed policy through preparation instead of inferring intent from names.

Missing, invalid, or stale provenance on a non-empty base grants no substitution
permission. Do not guess legacy intent from hierarchy or current defaults.
An explicit edit removes permissive intent even when its branch text is unchanged.

Preserve same-ref local refresh fallback, valid worktree reuse, qualified PR
strictness, and authenticated empty-remote bootstrap. Add no automatic fetch
when refresh is disabled. Use existing failure and deliberate retry surfaces.

## Consequences

- Preparation cannot silently replace an explicit source with another branch.
- Known inherited/default selections retain their documented recovery behavior.
- Ambiguous older attachments can require repository repair and deliberate retry.
- New selection producers and runtime adapters must retain intent per attachment.
- Existing JSON metadata storage avoids a schema migration and speculative API.
- Provider retargeting and prepared-start overwrite behavior remain separate work.

## Alternatives considered

### Remove all missing-base recovery

This removes the existing inherited-parent and automatic-default recovery paths.
Separately recorded intent preserves those paths without authorizing explicit substitution.

### Infer intent from branch names or ancestry

The caller can explicitly select the default or the parent's branch.
Names and current hierarchy cannot recover the original choice reliably.

### Mark only new explicit choices and allow every unmarked fallback

Older explicit tasks remain vulnerable. Strict handling of ambiguous records
provides the required compatibility boundary.

### Clear only the configured fallback string

The manager also discovers the live remote default and has earlier refresh
fallbacks. Every substitution path needs the policy gate.

## References

- [Requirements](../specs/workspaces/requirements/worktree-base-refresh.md)
- [Selection design](../specs/workspaces/system-design/worktree-base-selection.md)
- [Fix package](../plans/explicit-worktree-base-selection/plan.md)
