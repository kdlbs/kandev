# Orchestration review packet

This packet separates the implemented prototype, historical validation and the
remaining delivery plan. The private review branch is
[feat/workspace-orchestration](https://github.com/Corey-Fogg/kandev-orchestration/tree/feat/workspace-orchestration)
above exact v0.94.0. Start with the [root workbench guide](../../../WORKBENCH.md).

## Scope and implementation plan

- [Full scope audit](scope-audit.md): original requirement crosswalk, all implemented
  and partial behavior, contribution limits and central-view/plugin findings.
- [Prototype file inventory](scope-file-inventory.csv) and [baseline metadata](scope-baseline.json):
  every path in the 476-path original rebased prototype. These are historical
  inventory records, not the larger documentation-inclusive publication diff.
- [Remaining delivery plan](../../plans/orchestration-delivery/plan.md): eight
  publication/rollout work orders, three central-view work orders and continuation
  of the eleven assistant work orders, with dependencies, checks and rollback.
- [Plugin source review](plugin-review.md): useful patterns and incompatible host
  dependencies at a pinned revision. No plugin was installed or copied.
- [Candidate/live runbook](../../plans/orchestration-delivery/dogfood-runbook.md).

## Validation provenance

[Rebase validation](rebase.md) records checks performed on the original local
prototype. [Scope audit validation](scope-audit-validation.json) records the
earlier audit/design checkpoint; its `published: false` field is historical.
[Publication validation](publication.md) records the private import's checks,
source preservation, privacy review, hooks and remote verification.

The original local snapshot remains local because it contained installation notes
and identifying examples. This repository starts from public release ancestry
and a clean import. New documentation does not turn pending features into
completed implementation. Live data and provider-quality trials are not part of
the existing fixture evidence.

## Existing public discussion and synthetic demo

The proposal and initial desktop/mobile screenshots and silent videos are already
published in [kdlbs/kandev#3752](https://github.com/kdlbs/kandev/issues/3752).
The [maintainer feedback](https://github.com/kdlbs/kandev/issues/3752#issuecomment-5713059573)
motivates the planned workspace task overview alongside chat.

The demo uses a fictional **Garden Notes** workspace, generic requests and a
scripted provider on a fresh disposable database/browser context. It contains
five desktop and five mobile screenshots plus desktop/mobile 24-second videos in
MP4 and WebM, all silent. No production prompt/history or real model credential
was used. Follow the issue's asset links to the already-published media.

Those assets predate the release rebase and do not show the pending central task
view. View task 03 requires fresh inspected screenshots/video after implementation.
No public code PR, new issue comment or live deployment is performed by this
private publication.

## Central-view implementation checkpoint

The central workspace task view is now implemented and verified. See its
[completed work orders](../../plans/workspace-coordinator-view/plan.md),
[focused PR draft](coordinator-pr-draft.md), and
[fresh desktop/mobile screenshots and silent clip](media/coordinator-view/README.md).
These are private review artifacts from synthetic fixtures. The assistant
continuation and candidate/live delivery plan remain in progress.
