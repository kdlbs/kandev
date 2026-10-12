---
status: pending
---

# Task 03: Recovery contract and delivery

## Outcome and scope

Independent reachable-state and persistence-boundary regressions spanning
shutdown, repeated native recovery/adoption, SQL commit and new prompt admission.
Reconcile docs and public recovery guidance. Commit, push, open PR, fix CI/review
and merge in this session. Depends on tasks 01 and 02.

## Contract

REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002, criteria .3 through .5.
[Design](../../specs/platform/system-design/session-recovery-convergence.md).

## Acceptance

- Finite supported state/identity/terminal matrix has an explicit safe disposition.
- Failures between durable writes converge on retry without old prompt replay, false receipt, independent owner takeover or resolved-history resurrection.
- Normal launch/Resume/direct/queue admission agree; actual final-head required CI and review gates pass.

## Check

Focused Go race checks from tasks 01/02 and affected production entrypoint tests.
`python3 scripts/list-docs.py validate`
`python3 scripts/lint-spec-files.py --all`
`git diff --check`
Desktop and phone E2E are CI-only; never run locally.

## Results

Pending. PR #4425 is merged; overall recovery remains in progress.

[Parent plan](plan.md). [Requirements](../../specs/platform/requirements/durable-restart-reconciliation.md).
