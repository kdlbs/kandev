---
id: "03-recovery-contract"
title: "Recovery contract and delivery"
status: done
wave: 3
depends_on:
  - 02-native-convergence
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.3
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.4
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.5
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
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

PR #4432 merged as bc0d3b9047 after all 55 CI checks passed. Documentation coverage passes locally after
correcting work-order metadata. Review remediation covers terminal drain ordering,
complete stream snapshot validation, exact submission stream binding and unknown
reconstruction outcomes. Focused automatic restart and repeated recovery checks pass. CI-reported
recovery fixtures pass after correcting metadata-only retained inspection.
All eight initial review findings are resolved. CI then exposed an empty outcome
on reconstructed dispatch and a canonical internal continuation incorrectly
requiring a user-message row. Both desktop and phone pruned-history failures
share the former cause. Focused regressions cover empty-outcome settlement and
existing-only continuation repair, including deleted/changed canonical controls.
The next CI run exposed initial prompts whose canonical dispatch attempt is the
original execution ID rather than the message-derived identity. Recovery now
preserves that stored identity, with initial-prompt and changed-attempt controls.
The same CI run also exposed a substring collision in the sidebar filter test;
its selection now uses the exact option name.
Final-head CI and review-thread disposition passed before merge. No local E2E ran.

[Parent plan](plan.md). [Requirements](../../specs/platform/requirements/durable-restart-reconciliation.md).
