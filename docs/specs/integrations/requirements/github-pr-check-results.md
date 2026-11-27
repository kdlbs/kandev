---
status: active
system: integrations
created: 2026-09-30
owners:
  - kandev
---

# GitHub Pull Request Check Results Requirements

## Overview

Users need current check results when they inspect a linked GitHub pull request.
Cancelled jobs from an earlier workflow must not appear as current test failures.
The integration system owns provider check identity, result selection, and the
status supplied to task surfaces and automation.

The existing desktop PR disclosure and phone drawer consume this contract.
Their interaction remains subject to the shared
[PR task status summary](../../ui/requirements/pr-task-status-summary.md).
[Workflow attention](github-workflow-attention.md) owns approval gates.

## Terminology

- **Current execution:** The newest observed workflow run for one workflow,
  event, source repository, source branch, and PR head.
- **Superseded execution:** An earlier run in that same identity group.
- **Retained check:** A check that remains after selection of current results.
- **Cancellation:** A completed check with the provider conclusion `cancelled`.
  It does not establish a test failure or a successful test result.

## Requirements

### REQ-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001: Current execution results

**Intent:** Present current workflow results without mixing executions or providers.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.1:** When provider evidence identifies
  a newer execution, Kandev shall exclude checks from its superseded execution.
  A later cancellation update shall not make the older execution current.
- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.2:** When a replacement execution
  exists but its dependent job does not exist yet, Kandev shall exclude the
  superseded job. Kandev shall not invent a replacement check or count.
- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.3:** Checks with equal names from
  different workflows, events, source repositories, or applications shall remain
  independent. Their results shall not overwrite or hide one another.
- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.4:** Within a retained execution,
  the newest observed check for a logical job shall replace its earlier check.
  A queued replacement shall remain visible even without a start timestamp.
  A partial rerun shall retain successful jobs that were not rerun.
- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.5:** When identity or provider
  evidence is unavailable, Kandev shall not infer supersession from a name or URL
  alone. Missing evidence shall not erase a real failure or imply successful checks.
- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.6:** After a successful refresh,
  feedback and task status shall exclude the same superseded executions for the
  observed PR head. A head change shall prevent reuse of old-head selections.
- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.7:** Explicit refresh shall bypass
  stale workflow observations. Equivalent reads shall share the existing
  credential-scoped Actions cache. Different credential scopes shall remain isolated.
- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.8:** When a selected current
  workflow runs without concrete checks yet, task status shall remain pending.
  Detailed disclosures shall not fabricate a running check count.

### REQ-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002: Cancellation and failure semantics

**Intent:** Distinguish cancellation from failure without creating a false merge signal.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.1:** Retained cancellations shall
  contribute zero passed, failed, or running checks. They shall not appear in
  the red failure group or the pass-rate denominator.
- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.2:** Retained cancellations shall
  remain inspectable as cancelled in PR details. When all retained checks are
  cancelled, the disclosure shall distinguish that result from checks that never started.
- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.3:** When at least one retained
  check has `failure` or `timed_out`, Kandev shall show a failure, even when
  cancelled, successful, or running sibling checks exist.
  Existing workflow-attention handling of `action_required` shall remain effective.
- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.4:** Hover counts, task status,
  feedback issue detection, and CI repair eligibility shall agree that
  cancellation alone is not a failure. Existing denominators can differ because
  hover progress includes running checks and task badges count completed checks.
- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.5:** Cancellation alone shall not
  queue a CI repair prompt or consume a repair round. Independent current
  failures, review feedback, and conflicts shall retain their existing eligibility.
- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.6:** A cancellation-only result shall
  not establish successful checks or automatic merge readiness. Current provider
  mergeability, review, workflow-attention, and head guards shall remain required.
- **AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.7:** Desktop hover disclosures and
  phone PR drawers shall show the same selected results and cancellation semantics.
  The repair shall preserve their existing entry points, scrolling, and dismissal.

## Compatibility and scope

This contract corrects the interpretation of GitHub check results. It does not
change the repository's required-check policy or approve provider actions.
Existing success, neutral, skipped, and stale handling remains unchanged unless
selection removes a superseded execution.

Same-run attempts use observed job replacements. An absent job in a partial
rerun does not prove that its previous successful result is obsolete.

## Out of scope

- Cancelling, rerunning, approving, or merging GitHub work.
- GitLab and plugin-provider pipeline semantics.
- New polling schedules, cache infrastructure, settings, or feature flags.
- A workflow history browser or a new cancellation section in the hover.
- Changes to the check policy enforced by GitHub branch protection.

## Implementation plans

- [Current PR check results repair](../../../plans/github-pr-check-results/plan.md)
