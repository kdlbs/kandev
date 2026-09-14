# ADR-2026-09-10-worktree-metadata-recovery-boundary: Bound Automatic Worktree Metadata Recovery

**Status:** accepted (implementation verification is tracked in the linked plan)
**Date:** 2026-09-10
**Area:** backend

## Context

PR #3137 can recover checkout files after linked Git metadata disappears. It
creates a sibling branch and worktree, then changes the canonical repository slot.
The source branch and file contents can survive this metadata failure.

[Explicit new-branch recovery](2026-08-31-explicit-new-branch-session-recovery.md)
requires user action after confirmed branch loss. Its opening decision uses broader
replacement-branch wording. This proposal defines a narrow exception, not a silent
change to that accepted decision.

## Decision

Automatic metadata recovery applies only to a selected host Worktree environment
with surviving files and a resolvable recorded branch. It retains the original
checkout and snapshot. It does not claim to restore staging choices or missing
Git objects.

Recovery requires durable exclusive authority over the environment and its current
owner generation. Runtime startup, workspace restoration, cleanup, and ownership
transfer cannot overlap that authority. A busy or ambiguous environment refuses
replacement.

Confirmed branch loss retains the existing explicit action. Recovery must not
substitute the base branch when the recorded branch is absent.

The task environment remains the physical owner. Remote executors retain their
provider-owned workspace contracts. An origin URL does not establish execution
location.

## Consequences

- The contributor's file-preservation approach remains intact.
- The automatic path has a narrower eligibility contract than general branch recovery.
- A durable claim adds persistence and concurrency work beyond a path filter.
- Recovery can block until another runtime stops through its normal lifecycle.
- Retained snapshots consume disk space and can contain ignored secrets.
- The accepted branch-loss decision remains unchanged. Automatic metadata repair
  does not authorize replacement after confirmed branch loss.

## Alternatives considered

- **Inspect all task paths before selection.** This can inspect an unrelated or
  remote environment with host filesystem operations.
- **Use only a process-local mutex.** This does not protect separate backend
  connections or durable ownership transitions.
- **Repair a live shared environment.** Existing runtimes can continue to use
  the old directory while new sessions use its replacement.
- **Require explicit action for every metadata failure.** This avoids automatic
  replacement but changes the contributor's intended recovery flow.
- **Rebuild from the task base branch.** This can conceal lost branch history
  and belongs to the separate explicit recovery flow.

## Related specifications

- [Requirements](../specs/tasks/requirements/worktree-metadata-recovery.md)
- [System design](../specs/tasks/system-design/worktree-metadata-recovery.md)
- [Implementation plan](../plans/worktree-metadata-recovery/plan.md)
