---
created: 2026-10-25
status: implemented
requirements:
  - REQ-CI-ACTIONS-SECURITY-001
  - REQ-CI-ACTIONS-SECURITY-002
system_design:
  - ../../specs/ci/system-design/privileged-workflow-inputs.md
legacy_specs: []
---

# Implementation plan: GitHub Actions security findings

## Overview

Remove PR-controlled execution inputs from automatic review. Reduce unnecessary
credentials, cache restores, and expression interpolation in publication jobs.
Preserve all existing authorization and publication behavior.

All three work orders are implemented and locally validated. PR publication, CI,
and automated-review reconciliation proceed in the primary conversation.

## Evidence and scope

Investigation used current `main`, commit
`6677802efa4fac6c2ecc909cb433477c90f7fb2d`. Zizmor 1.25.2 found 40 unsuppressed
findings: 18 informational, 3 low, 8 medium, and 11 high. The parent baseline at
`595df36ed5c` had 38. Two new CMD limitations account for the increase.
See the [complete finding ledger](findings.md) and [baseline checks](validation.md).

The fresh open-PR inventory contained 116 PRs. PR #4534 changes release and
preview workflows for SDK publication but does not fix this finding set.
PR #3693 changes PR-documentation reliability. Neither replaces this package.
Before implementation, repeat the current-main and open-PR check. Preserve any
SDK publication jobs and dependency gates if #4534 lands first.

In scope: the 40 findings, their directly associated review boundaries, targeted
tests, credential handling, and evidence for residual warnings. No suppressions
are proposed. Out of scope: provider changes, application UI, broad workflow
rewrites, existing suppressed warnings, live publication, merging, and external
repository settings. No implementation agents are authorized.

## Technical approach

1. **Review inputs:** keep workflow-SHA checkouts in Claude fork review and both
   OpenCode paths. Fetch head objects without checkout. Use bounded immutable
   context and trusted agent configuration. Preserve OpenCode producer identity.
2. **Release execution:** move reported expressions to environment data. Disable
   six checkout credential defaults in `release.yml`. Authenticate only release
   branch/tag pushes with command-local helpers and the ordinary token.
3. **Other publication:** disable persisted image checkout credentials. Remove
   docs preview dependency caching. Move the weekly date to environment data.
   Add regression coverage for the reviewed boundaries and update the ledger.

The [system design](../../specs/ci/system-design/privileged-workflow-inputs.md)
defines concrete helpers, provenance, and failure behavior. The
[proposed ADR](../../decisions/2026-10-25-automatic-review-input-isolation.md)
amends the head-checkout clause in the accepted OpenCode evidence ADR. Do not
silently retain tests that require the unsafe checkout, or remove their evidence
identity assertions. Mark the new ADR accepted and reconcile the old clause only
after the implementation passes its checks.

Existing companion context includes
[manual Claude review](../claude-manual-pr-review/plan.md) and
[walkthrough concurrency](../pr-walkthrough-eligibility-concurrency/plan.md).
Their behavior remains unchanged. This package does not reopen unrelated
OpenCode runtime/provider plans. Update the shared CI design's review-flow
references when this implementation becomes current.

## Tests

| Acceptance criteria | Required evidence |
| --- | --- |
| AC-CI-ACTIONS-SECURITY-001.1 | Raw Claude/OpenCode contracts plus a temporary Git fixture with malicious root config, skills, plugins, and symlinks. A stub agent observes only trusted execution files. |
| AC-CI-ACTIONS-SECURITY-001.2 | Head mismatch, new file, binary, oversized file, symlink/gitlink, traversal, and malformed SHA cases. Test the optional pinned-head Claude helper without changing its manual caller. |
| AC-CI-ACTIONS-SECURITY-001.3 | Existing Claude gates, OpenCode posting tests, immutable full-PR range, App identity, fork advisory behavior, and non-clean failure results. |
| AC-CI-ACTIONS-SECURITY-002.1 | Execute extracted shell blocks with fake publication commands and hostile values. Assert exact argv and no marker execution. |
| AC-CI-ACTIONS-SECURITY-002.2 | Check every named checkout. Execute branch/tag push blocks with stub Git/gh. Assert ordinary token scope, retries, no persisted auth, and unchanged admin merge. |
| AC-CI-ACTIONS-SECURITY-002.3 | Raw docs-preview contract rejects explicit and automatic caches and preserves install/publisher/comment boundaries. |
| AC-CI-ACTIONS-SECURITY-002.4 | Existing release mode/backfill tests, all publication gates, Discord and contributor contracts, image size-before-promotion ordering, and the four reviewed trigger boundaries. |

Each work order contains exact commands. All shell reproductions use synthetic
values and stubs. No credential read, model request, publication, or dispatch is
part of local verification.

## Work orders

- [x] [Task 01: Isolate automatic review inputs](task-01-review-inputs.md)
- [x] [Task 02: Harden release input and Git authentication](task-02-release-inputs.md)
- [x] [Task 03: Harden docs and image publication](task-03-publication-inputs.md)

Execute sequentially. The order puts the evidenced agent-loading path first.
The work orders share required CI registration and evidence files, so do not
delegate them concurrently.

## Verification results

[Validation evidence](validation.md) records the historical baseline and final
implementation results. All 249 Python/Node test cases and the shell contract
passed. Action pinning covers 30 workflows. The final Zizmor scan has exactly 10
reviewed residual findings: six trigger warnings, three CMD limitations, and one
action-replacement suggestion. No new suppressions were added. All specification
and diff checks passed. PR CI and automated reviews remain external delivery gates.

The adversarial review found that reserved instruction filenames in prepared
context bypassed the intended data boundary. Shared neutral blob filenames and
manifest mapping now protect both automatic reviews and walkthrough generation.
The finding, red/green regressions, and pinned-CLI evidence are recorded in
[validation.md](validation.md).

Delivery CI also exposed pre-existing E2E fixture defects. The fixup
corrects the disabled-scroll baseline, final-directory readiness, and scoped
MCP route cleanup. It also waits for drawer animation and Retry visibility
before measurement, using the existing touch-control precision allowance.
Those fixture corrections preserve product rendering and the 44px touch requirement. Detailed
causal evidence and verification are recorded in the validation file.
The review checkbox scenario also resets its disposable repository before task
creation and swipes on the persistent header instead of replaceable code rows.
Its two-file precondition and all negative auto-mark assertions remain enforced.
Approval persistence checks now await their PATCH response. Oversized-message
task switching waits for the drawer animation before tapping a task row.
Branch-refresh and filter setup wait for repository-picker focus restoration
before opening the executor picker. Their original selection and request
assertions remain unchanged.
The silent-prompt fixture releases output only after the real watchdog notice
is observed. The mobile routing fixture waits for save confirmation before
checking persisted settings. Both retain their original behavior assertions.
The Office title-edit fixture checks the optimistic render before waiting for
its PATCH response, then checks persistence after the successful save.

Current-main integration preserves both sides of the sidebar and review-fixture
changes. Later delivery CI exposed two product races: a late Azure assignment
response reopened closed detail, and remote recovery could leave a duplicate
resume stuck in `STARTING`. The fixes preserve selection ownership and restore
only a ready, previously idle session through the existing attempt-fenced state
transition. Canvas measurement uses the same settled-animation and 44px helper
as other mobile controls. Validation records the deterministic regressions.
The mobile Stats fixture keeps its simulated outage active until manual Retry,
so initial foreground recovery cannot consume the error state under test.
The next main integration also retains the upstream Azure configuration-save
response wait and automatic Stats retry coverage while preserving the scoped
mobile Retry control assertions.

## Risks

- The pinned Claude action changes sensitive configuration during preparation.
  Confirm its agent-mode behavior and trusted execution state in a stubbed test.
- OpenCode tests currently require head checkouts. Update those assertions while
  retaining exact-head posting and dedicated App identity checks.
- Removing release checkout credentials without replacement push authentication
  would break release branch and signed-tag creation.
- Release backfills combine tagged application inputs and current workflow
  helpers. Preserve both identities and all actual publication conditions.
- The timeout fixture must resolve real system sleep before installing its retry
  stub. Its regression now passes on this Nix host.
