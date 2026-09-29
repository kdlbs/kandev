# ADR-2026-09-29-missing-worktree-checkout-recovery: Recover missing canonical checkouts before attachment

**Status:** accepted and implemented
**Date:** 2026-09-29
**Area:** backend

## Context

Issue [#4052](https://github.com/kdlbs/kandev/issues/4052) reports a retained
Worktree environment whose checkout and Git administrative entry were removed.
Its recorded branch survives. Attach-only reuse rejects the absent checkout,
while recovery admission treats absence as no work. Repeated recovery cannot proceed.

The user selected guarded automatic recovery on 2026-09-29. This extends the
[metadata-recovery boundary](2026-09-10-worktree-metadata-recovery-boundary.md)
only for a provably absent canonical checkout. Surviving-file recovery keeps
its existing contract.

## Decision

Restore the recorded checkout in selected-environment recovery admission. Require
the existing durable exclusive claim, current ownership generation, and no live
runtime consumers. Keep attach-only preparation read-only.

Preserve the recorded slot, path, branch, and surviving branch head. Refuse
uncertain paths, owners, branch identity, or liveness. Confirmed branch loss
retains the [explicit recovery action](2026-08-31-explicit-new-branch-session-recovery.md).

Pin the managed parent and exclusive target identity through Git materialization.
Remove only the exact proven stale administrative entry; never use a Git command
that can remove a checkout which appeared after inspection. Hold a per-worktree
operating-system operation lock with the durable environment claim, including
through the external-start boundary. A completed record may settle its retained
claim only after the lock holder revalidates the exact record and checkout.
When local and tracking refs are absent, probe the exact origin branch; probe
errors are not proof of loss, and fetch must match the advertised head.

Use a narrow restoration operation. Do not inherit general recreation's path
removal, branch-refresh, or repository setup behavior. Interruption must retain
the operation identity or fail closed.

## Consequences

- A quiescent task can recover without filesystem or database access by its user.
- Busy environments remain blocked until consumers stop through their normal lifecycle.
- Deleted uncommitted content and staging choices remain unrecoverable.
- Safe path creation, stale-registration handling, and interrupted operations need regression coverage.
- Remote executors retain their own recovery contracts.

## Alternatives considered

- Recreate inside attach-only reuse: violates the shared-workspace immutability contract.
- Check sibling session state once: does not serialize startup or cover borrowed runtimes.
- Add an explicit action: adds UI and API scope without removing the need for exclusive authority.
- Keep manual repair: leaves ordinary users unable to recover an otherwise intact branch.

## Related documents

- [Requirements](../specs/tasks/requirements/worktree-metadata-recovery.md)
- [System design](../specs/tasks/system-design/worktree-metadata-recovery.md)
- [Fix package](../plans/missing-worktree-checkout-recovery/plan.md)
