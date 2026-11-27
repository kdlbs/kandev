---
created: 2026-09-30
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001
  - REQ-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002
system_design:
  - ../../specs/integrations/system-design/github-pr-check-results.md
legacy_specs: []
---

# Implementation Plan: Current GitHub PR Check Results

## Overview

First preserve provider identity and select current executions. Then correct
cancellation semantics across status, hover presentation, and automation.
These work orders run sequentially because presentation and automation need the
selected-check contract from Task 01.

The integration owns this package because it defines current provider evidence.
The accepted requirements and current system design are implemented through
the two sequential work orders below.

## Confirmed defect

The investigation concerns task `b5afe795-a59a-4f5d-a6ac-6df178425845` and
[PR #4068](https://github.com/kdlbs/kandev/pull/4068), at head
`0add931cbf985c0fb800e23b5b6e762fcf24e2e0`.

| Provider observation | Earlier execution | Replacement execution |
| --- | --- | --- |
| Preview run | [36729911126](https://github.com/kdlbs/kandev/actions/runs/36729911126) | [36729911232](https://github.com/kdlbs/kandev/actions/runs/36729911232) |
| Workflow ID | `266562986` | `266562986` |
| Suite ID | `99475252942` | `99475253346` |
| Event | `pull_request_target` | `pull_request_target` |
| Created at | `2026-09-30T14:32:41Z` | `2026-09-30T14:32:41Z` |
| Conclusion | Cancelled at `14:32:49Z` | Success at `14:39:15Z` |

The runs have equal creation times. The run-ID tie breaker identifies the replacement.
Their source repository is `abhishekbiyala/kandev`, and their branch is
`fix/plugin-remote-agentctl-instance`. The earlier run has no explicit PR associations.
The check selector therefore needs the verified `pull_request_target` fallback.

While replacement packaging ran, its deploy and description jobs did not exist.
`mergeChecks` kept the cancelled older jobs by name. `bucketCheck` classified
them as failed. Both the deployed `a2c8a79e0a3` and the investigation checkout
contain these rules.

A timestamp reconstruction at 14:35 UTC reproduced the screenshot.
It contained 47 successful check runs, 2 successful status contexts, 2 running
jobs, and 2 cancelled jobs. The hover reported **49/53 (92%)** and two failures.
This was a reconstruction, not a capture of the original browser response.
The diagnostic archive was partial because of its byte limit.

At approximately 14:41 UTC, live Kandev feedback contained 52 successes,
16 skipped checks, one running walkthrough, and zero failures.
The two preview jobs then pointed to the successful replacement.

The same cancellation classification exists in backend counts and
`ciAutomationCheckConclusionNeedsFix`. The GraphQL batch uses GitHub's native
rollup, so detailed reads can also produce a different summary state.

## Scope

### In scope

- Preserve check/application/suite identity in both real provider clients and mocks.
- Select matched current suites before per-job reduction or feedback publication.
- Retain independent producers and safe legacy/third-party fallbacks.
- Exclude cancellation from failure counts and CI repair evidence.
- Preserve inspectable cancellation details and non-success cancellation-only state.
- Prove desktop and phone behavior through the real mock-backed provider path.

### Out of scope

- Workflow mutations, branch protection, merge policy changes, or live task repairs.
- GitLab and plugin-provider pipeline interpretation.
- New polling, caches, settings, feature flags, or database migrations.
- A history viewer, new cancellation group, or redesigned PR disclosure.
- Global replacement of existing neutral/skipped/stale policies.

## Assumption check

Confirmed intent: create a fix package for the investigated false failures.
Verified facts: cancellation conclusions, absent replacement jobs, suite identity,
equal run timestamps, existing failure predicates, and shared phone presentation.

The repair keeps genuine failures actionable. It does not promote cancellation
to success. Existing provider merge guards remain required.
These choices follow the reported discrepancy and the existing strict automation
contract. No material question blocks the package.

## Technical approach

The [paired design](../../specs/integrations/system-design/github-pr-check-results.md)
owns selection, identity fields, cache reuse, and conclusion policy.

Task 01 adds a pure `check_selection.go` boundary, expands shared transport
shapes, and wires selection into status/feedback before persistence.
It reuses workflow ordering and cache primitives. Matching unassociated target
events is local to check selection; approval classification keeps its current rule.

Task 02 changes the existing reducers and repair predicate. It updates the
web buckets and workflow grouping, removes ignored-only groups, and uses existing
localized non-success copy for a cancellation-only result.
The task adds unit, service, automation, and rendered regression evidence.

### Compatibility matrix

| Provider/transport | Identity | Behavior | Evidence | Unsupported shape |
| --- | --- | --- | --- | --- |
| GitHub / GH CLI | Check/app/suite IDs and matched Actions run | Suppress older suites before reduction | CLI pagination and selector tests | Preserve raw checks; no guessed supersession |
| GitHub / PAT or App-backed PAT | Same shared REST fields | Same selection and conclusion policy | PAT pagination and service tests | Same conservative fallback |
| GitHub / batched GraphQL | Native `statusCheckRollup` | Retain native state and populated-field rules | REST/batch/feedback convergence test | Do not invent detailed counts |
| Third-party GitHub check application | App/check identity; no Actions lineage | Latest job within application; preserve real failures | Independent application test | Preserve unknown-producer partition |
| Commit status context | Existing source/name/time | Keep existing logical-check merge behavior | Status-context merge test | Do not shadow known distinct producers |
| Mock GitHub | Raw identities and workflow snapshots | Exercise real selection in service | Service and desktop/phone E2E | Legacy fixtures without IDs retain fallback |
| GitLab / plugin providers | Existing provider contract | No change | Existing shared-anatomy tests | No GitHub cancellation policy applied |

## ASCII UI preview

### UI-01: Running replacement execution

Entry: desktop PR hover or phone PR-chip tap.
This small fixture has two passing checks, one running check, and two cancelled
checks from the superseded suite. Counts illustrate the regression contract.

```text
BEFORE: desktop hover          AFTER: desktop hover
+-------------------------+    +-------------------------+
| #42 Current PR           |    | #42 Current PR           |
| Pass rate 2/5 (40%)      |    | Pass rate 2/3 (67%)      |
| Passed              2   |    | Passed              2   |
| In progress         1   |    | In progress         1   |
|   package-preview       |    |   Preview Environment   |
| Failed              2   |    |     1 running           |
|   deploy-fork       [+] |    |                         |
|   update-description[+] |    | Review and automation   |
| Review and automation   |    +-------------------------+
+-------------------------+

AFTER: phone, existing inset drawer
+-----------------------------+
| #42 Current PR       [Close] | fixed existing header
| Pass rate 2/3 (67%)          |
| Passed                  2   |
| In progress             1   |
|   Preview Environment       |
|     1 running               |
| Review and automation       | existing internal scroll
+-----------------------------+
          safe-area clearance
```

Required structure: omit the false failure group and its repair controls.
Preserve the current check's link and the existing review/automation sections.
The phone uses the shipped drawer, not hover or a smaller desktop popover.
ASCII spacing, workflow labels, and fixture numbers are illustrative.

### UI-02: Cancellation-only retained result

```text
Desktop hover / phone drawer body
+--------------------------------+
| #42 Current PR                 |
| Checks not successful          | existing localized copy
| Review and automation          |
+--------------------------------+
```

No red failure group, green pass rate, fake running check, or new cancellation
section appears. The existing details action still exposes cancelled job data.
An unrelated genuine failure instead retains the current red failure group.

Both views map to `AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002.1`, `.2`, `.3`,
and `.7`. The closest mobile exemplar is `PRStatusChipDrawer` and
`ChangeRequestStatusDrawerContent`. Keep their scroll ownership, dismissal,
safe-area behavior, and touch targets. No responsive preference is written.

## Tests

All method names below are planned regression names unless they already exist.

| Criteria | Test location and proof |
| --- | --- |
| 001.1, 001.2 | `check_selection_test.go`: `TestSelectCurrentPRChecksSupersededSuiteWithoutReplacementJobs` fails before suite selection |
| 001.1 | `TestSelectCurrentPRChecksEqualCreationTimeAndLateCancellation`: higher run ID wins despite late updates |
| 001.3, 001.5 | `TestSelectCurrentPRChecksIndependentProducers`, `TestSelectCurrentPRChecksMissingEvidence`, and `TestGetPRFeedbackPreservesUnmatchedActionsSuitesWhenWorkflowEvidenceUnavailable` |
| 001.4 | `TestSelectCurrentPRChecksQueuedJobAndPartialRerun`: newer ID without start time; preserve jobs not rerun |
| 001.8 | `TestSelectCurrentPRChecksActiveWorkflowWithoutJobs`: pending snapshot with zero invented checks |
| 001.6, 001.7 | `service_pr_feedback_sync_test.go` and `workflow_attention_cache_test.go`: shared snapshot, head/scope changes, explicit invalidation |
| 001.1, 001.3 | `gh_client_test.go`, `gh_client_reads_test.go`, `pat_client_test.go`: identity survives paginated transport |
| 002.1, 002.3, 002.4 | `client_helpers_test.go`, `service_test.go`: `TestCancelledCheckPolicy`, including mixed cancellation/failure/running results |
| 002.5, 002.6 | Orchestrator `event_handlers_github_ci_automation_test.go`: `TestCIAutomationCancelledChecksDoNotConsumeRound` and `TestCIAutomationCancellationOnlyNotReadyToMerge` |
| 002.1, 002.2, 002.7 | Web `check-buckets.test.ts`, `pr-ci-popover.test.ts`, new `pr-ci-popover.checks.test.tsx`: ignored-only groups and cancellation-only copy |

All abbreviated criteria refer to the two requirements in this plan's frontmatter.
Keep existing workflow-attention and mixed genuine-failure tests.

## E2E tests

Extend `apps/web/e2e/tests/pr/pr-topbar-popover.spec.ts` (project `chromium`)
and `apps/web/e2e/tests/pr/mobile-pr-ci-chip.spec.ts` (project `mobile-chrome`).
Prefix new scenario titles with `current PR checks:` for focused execution.

| Scenario | Criteria | User-visible proof |
| --- | --- | --- |
| Superseded cancelled suite, active replacement, absent dependent jobs | 001.1, 001.2, 002.1, 002.7 | Open disclosure; current progress remains, false failure rows/actions disappear |
| Replacement completes on same SHA | 001.6, 001.7 | Refresh/reopen and reload show replacement success and current links |
| Cancellation-only retained checks | 002.1, 002.2, 002.6, 002.7 | Non-success copy; no fake running or passing result; details remain accessible |
| Current active workflow without concrete jobs | 001.8 | Pending summary with zero fabricated running jobs in loaded feedback |
| Cancellation plus independent true failure | 001.3, 002.3, 002.7 | Red failure and actionable context control refer only to the real failure |

Seed raw checks plus workflows using `mockGitHubAssociateTaskPR`. Do not replace
the browser's feedback response with already-normalized data.
Preserve phone drawer containment and existing scroll/dismiss assertions.
No broad E2E suite is required.

## Work orders

- [x] [Task 01: Select current provider check executions](task-01-select-current-check-executions.md)
- [x] [Task 02: Correct cancellation semantics across consumers](task-02-correct-cancellation-semantics.md)

## Implementation results

Task 01 completed. The initial service regression failed with the two cancelled
older-suite jobs still present alongside the replacement packaging check. After
the fix, the provider identity, selector, cache-sharing, jobless-active-workflow,
and transport pagination regressions passed.

Follow-up review found that same-name GitHub Actions checks from different
suites could collapse to the newest check ID when no workflow run was available
to join. A service-level regression failed before the correction for App ID and
slug-only identities under both absent and failed workflow reads. The selector
now preserves unmatched Actions suite/producer partitions. The regression
confirms that both checks remain, the overall state is failure, and
`has_issues` stays true.

- `(cd apps/backend && go test ./internal/github -count=1)`: passed.
- `(cd apps/backend && go test -race ./internal/github -run '^TestSelectCurrentPRChecks' -count=1)`: passed.
- `(cd apps/backend && go test ./internal/github -run '^(TestGetPRFeedbackPreservesUnmatchedActionsSuitesWhenWorkflowEvidenceUnavailable|TestSelectCurrentPRChecksSupersededSuiteWithoutReplacementJobs|TestSelectCurrentPRChecksQueuedReplacementAndPartialRerun)$' -count=1)`: passed after the review remediation.
- `(cd apps/backend && go test ./internal/orchestrator -run 'Test.*(CIAutomation|CIFix|CIMerge)' -count=1)`: passed after the review remediation.
- `(cd apps/web && pnpm run typecheck)`: passed.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

Task 02 completed. Cancellation is excluded from failure and repair evidence;
loaded empty feedback does not inherit stale aggregate counts; provider workflow
identity keeps equal display names independent. Desktop and phone regressions
exercise raw provider snapshots, same-head replacement, cancellation details,
jobless active workflows, and an independent real failure. Existing strict
merge-readiness checks remain in place.

The initial cancellation tests failed before the reducer changes: backend status
reported failure, CI automation kept cancellation as repair evidence, the browser
bucket counted cancellation as failed, and the rendered hover lacked its required
non-success empty state. Those assertions pass with the implementation.

## Verification results

The managed desktop and phone E2E runners each rebuilt the Go backend and Vite
assets before exercising the rendered flows.

- (cd apps/backend && go test ./internal/github -count=1): passed.
- (cd apps/backend && go test ./internal/orchestrator -run 'Test.*(CIAutomation|CIFix|CIMerge)' -count=1): passed.
- (cd apps/web && pnpm exec vitest run lib/github/check-buckets.test.ts components/github/pr-ci-popover.test.ts components/github/pr-ci-popover.checks.test.tsx components/github/pr-ci-popover.automation.test.tsx): passed, 44 tests.
- (cd apps/web && pnpm run typecheck): passed.
- (cd apps/web && pnpm exec eslint lib/github/check-buckets.ts components/github/pr-ci-popover.tsx components/github/pr-ci-popover.checks.test.tsx lib/types/github.ts lib/types/github-checks.ts e2e/helpers/pr-checks.ts e2e/tests/pr/pr-topbar-popover.spec.ts e2e/tests/pr/mobile-pr-ci-chip.spec.ts): passed.
- (cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet): passed; no new product copy.
- (cd apps/web && pnpm e2e:run --project chromium tests/pr/pr-topbar-popover.spec.ts -- --grep 'current PR checks:'): passed, 1 test.
- (cd apps/web && pnpm e2e:run --project mobile-chrome tests/pr/mobile-pr-ci-chip.spec.ts -- --grep 'current PR checks:'): passed, 1 test.
- python3 scripts/list-docs.py validate: passed.
- python3 scripts/lint-spec-files.py --all: passed.
- Local work-order documentation coverage preflight: passed for both work orders.
- git diff --check: passed.

Package validation on 2026-09-30:

- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local `validateCoverage` preflight: both work orders covered, zero errors.
  This used a prospective runtime path to exercise reference validation despite
  the current documentation-only diff. It did not evaluate a live PR.
- Local Markdown link and whitespace scan: passed for all five package files.
- `git diff --check` and scoped `git status --short`: passed; all five new
  files are unstaged and uncommitted.

To repeat the structural preflight from the repository root:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/github-pr-check-results';
const paths = fs.readdirSync(dir).filter(p => p.endsWith('.md')).map(p => `${dir}/${p}`);
paths.push('docs/specs/integrations/requirements/github-pr-check-results.md');
paths.push('docs/specs/integrations/system-design/github-pr-check-results.md');
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.map(filename => ({ filename, status: 'added' }));
changedFiles.push({ filename: 'apps/backend/internal/github/client_helpers.go', status: 'modified' });
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, errors: result.errors, workOrders: result.workOrders }, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

## Risks

- Equal workflow creation times require run-ID ordering. Update time alone is incorrect.
- `pull_request_target` can lack PR associations. Require positive source and head
  identity; never broaden the approval classifier as a side effect.
- Partial reruns reuse suites and can omit successful jobs. Only observed job
  replacements justify per-job reduction within that suite.
- Removing identity before selection can lose unrelated true failures permanently.
- Cached workflow evidence can lag same-SHA reruns. Preserve explicit invalidation
  and bounded existing TTLs; do not claim instant background convergence.
- Current frontend neutral handling and backend completed denominators differ.
  This package aligns cancellation semantics without expanding that policy change.
- Existing historical automation checkpoints can contain cancelled checks.
  Prune them through normal prompt-free refresh, without rewriting old conversations.

## Documentation impact

No public-documentation change was needed. The implementation-time search found
no repair instructions that classify cancellation as a repairable failure; the
existing CI repair descriptions continue to refer to failed checks. The changed
desktop and phone content uses existing localized copy, so no locale entries or
screenshots changed.

## Delivery boundary

Implementation is complete and delivered in PR #4092.
