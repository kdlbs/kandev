---
created: 2026-10-13
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-02.md
legacy_specs: []
---

# Implementation Plan: GitLab optional manual CI summaries

## Overview

Correct optional manual-job acceptance consistently in the backend and
frontend, with independent regressions proving API and first-party display
agreement. Exactly one sequential work order owns this vertical result.
ROOT completed full-file review and explicitly released implementation in the
same primary. The correction and regressions pass task-defined local checks,
including canonical changed-package Go lint and the staged i18n ratchet.
Hosted CI, review and merge remain delivery gates; CI owns full-backend lint.

Integrations owns this outcome because the defect is provider-specific CI
classification used by status, cached associations, feedback and existing
desktop/phone disclosures. The adjacent MR status-chip specification owns
disclosure mechanics and is unchanged. No independent UI repair specification
or new ADR is required.

## Scope

### In scope

- Existing requirement AC .16-.19 and its split design clarification.
- Only raw `manual` plus `allow_failure=true` joins the existing pass bucket.
- Real provider-API status/feedback coverage, classifier/summary controls and
  real first-party live/fallback component outcomes.
- Existing stage-priority and link-selection agreement for mixed job states.

### Out of scope

- Job or pipeline status rewriting, upstream job execution, CI configuration,
  credentials, network mutations, merge/automation gate changes.
- Layout, touch behavior, copy/locales, new dependencies, new persistence or
  cache/refresh policy, request races, other providers, companion features.
- Installs, production/permanent-test edits, heavy tests/lint/builds, commits,
  pushes, PRs, delegation, new tasks or sessions during this design turn.
- Protected discovery source read/copy/replay/modification/cleanup, foreign
  worktrees/deps/caches/media references or protected resources.

## Evidence and admission

Initial checkout was clean at
`1de225c145afd4cab09cf2770b0a370616fe0a54`, the qualified ROOT head.
Both current bucket functions still default every manual job to in progress.
ROOT's independent qualification receipt is
`/tmp/kandev-root-gitlab-manual-discovery-20261013-_gues960/qualified-proof.json`.
Its actual Go run failed two causal assertions: `GetMRStatus` and
`GetMRFeedback` returned 1/2 rather than 2/2 for a success plus optional manual
job (session 70002, join 49d6d2, exit 1, package 0.086s). Its actual Node24 run
failed the job-count/deploy-stage outcome (chunk 37fb19, exit 1). Blocking
manual, optional running and allowed-failure controls passed in both runs.
No new reproductions or product checks were run during design.

The receipt's private Go/Node sources are protected, mode 0400; do not inspect,
copy, replay or mutate them. Author permanent tests independently from the
contract and committed test patterns. Before implementation, recheck HEAD and
the two relevant source files; checkpoint ROOT if the qualified baseline or
relevant source changed. Preserve other contributors' work.

## Technical approach

### Classification and projection

In `apps/backend/internal/gitlab/models_pipeline_jobs.go`, change only
`pipelineJobBucket`'s manual case. In
`apps/web/lib/gitlab/pipeline-buckets.ts`, mirror it in `bucketJob`.
Update adjacent predicate comments to explain accepted/pass semantics and
avoid suggesting optional manual execution succeeded. `summarizePipelineJobs`,
`bucketJobCounts` and `groupJobsByStage` continue deriving all counts from
their respective predicate. Their total, failing set, stage priority and
dominant-bucket URL selection remain unchanged.

Audit but do not edit `PATClient.GetMRStatus`/`GetMRFeedback`, existing
service/store task-MR projection, feedback controller, `useMRFeedback`,
`MRCIPopover`, or automation gates. The bucket fix reaches status counts,
live feedback summaries and cached associations on their normal refresh.
The popover computes precise counts from jobs when available, otherwise
uses cached `TaskMR` totals; raw job metadata remains available to consumers.

| Provider/transport | Identity/capability | Intended outcome | Evidence/fallback |
| --- | --- | --- | --- |
| GitLab PAT or environment token / REST | Existing workspace host + full project path + MR IID; jobs report status and `allow_failure` | Consistent status, feedback and pass buckets | Real PATClient `httptest`; false/missing allowance is blocking; unknown status stays in progress |
| GitLab `glab` connection / embedded PATClient REST | Existing authenticated host/token discovery | Same job classification | Source-grounded embedding plus existing package tests; no CLI/auth mutation |
| GitLab mock client | Existing mock jobs | Same backend summary | Existing owning-package tests; no new mock policy |
| First-party web / feedback HTTP and cached TaskMR | Existing same-MR job list or aggregate counts | Live pass/stage summary and aggregate fallback agree | Real hook/API/component fetch harness; no jobs means aggregate counts, no invented stage rows |
| Other providers | Independent clients and bucket contracts | Outside this repair | No source or tests changed |

### Test-value boundary

Backend unit tables protect the exact predicate and unchanged failing-job set;
real provider-API tests separately protect JSON decoding, count population in
both reads and raw status retention. Frontend unit tests protect stages and
dominant-state hrefs absent from backend models. Real `MRCIPopover` tests
protect live-job precedence and aggregate fallback through the production hook
and API, which the existing mocked-hook suite cannot establish. No test-only
production exports, flags, injection hooks or reset helpers are needed.

## ASCII UI preview

UI-01: existing MR CI body, opened by desktop hover or phone chip tap. These
are data outcomes within the existing popover/drawer, not new product copy.
The current outcome is source-grounded by ROOT's count/stage reproduction;
the final rendering remains to be proved by the real component tests.

```text
Success test + optional manual deploy
Before                         After
Pass rate 1/2 (50%)             Pass rate 2/2 (100%)
[yellow] deploy       0/1       [green] deploy       1/1

Mixed jobs in one deploy stage (after)
optional manual + running      [yellow] deploy       1/2
optional manual + blocking     [yellow] deploy       1/2
optional manual + real failed  [red]    deploy       1/2
```

Desktop keeps the existing fine-pointer popover. Phone keeps
`MRStatusChipDrawer` with fixed header and the existing scrolling body,
rendering the same `MRCIPopover` data in the same order. Its entry point is
the existing chip tap; dismissal, touch targets and safe-area behavior remain
unchanged. Counts, stage priority and native links are requirements; spacing
and color words above illustrate existing classes rather than new styling.
AC .16-.19 map to the live/fallback rendered regression.

## Tests

| Acceptance | Primary regression evidence |
| --- | --- |
| AC-INTEGRATIONS-GITLAB-INTEGRATION-001.16 | `TestPATClient_MRJobCounts_OptionalManual` in `pat_client_pipeline_jobs_test.go` independently exercises both real reads; `MRCIPopover optional manual CI` suite proves 2/2 live and cached fallback |
| AC-INTEGRATIONS-GITLAB-INTEGRATION-001.17 | Mixed backend summarizer case; `bucketJobCounts`/`groupJobsByStage` mixed tables; real deploy-stage count, icon and dominant-job href assertions |
| AC-INTEGRATIONS-GITLAB-INTEGRATION-001.18 | Extended `TestPipelineJobBucket` and `bucketJob` policy tables for both allowance values and nonterminal/failed/skipped/unknown controls |
| AC-INTEGRATIONS-GITLAB-INTEGRATION-001.19 | Provider API raw metadata assertions and unchanged failing set; existing owning-package automation and readiness tests remain green; real live data cannot enable merge through count-only readiness |

Tests listed above are authored and their targeted checks passed. See the work order for
exact independently rooted RED/GREEN and lightweight validation commands.
Run one frontend `pnpm run typecheck` for the typed API/store fixture, and
the existing `pnpm run i18n:ratchet` against all actually staged new/changed
owned paths before first publication. Both checks passed during authorized implementation. Normal active
Go/frontend hooks and repository-required lint remain required; never bypass
them. No full-web tests/lint/build, E2E or generic QA pass is added.

## E2E tests

No new Playwright case or browser build is required under the mobile skill's
pure state/data-normalization exception. Real fetch-to-hook-to-component
outcome tests cover the shared CI body on desktop and phone. No rendered
layout, touch, scrolling, navigation or viewport-dependent logic changes.
Existing `mobile-mr-status-chip.spec.ts` covers the drawer entry path but is
not claimed as executed or as new classification evidence. If implementation
requires surface changes, stop and revise this package before widening scope.

## Public documentation assessment

Read the existing GitLab review/pass-rate explanations in
`docs/public/integrations.md` and `docs/public/sessions-and-review.md`, and
searched the public docs, root README and screenshot catalog. None establishes
a contrary optional-manual classification. This draft clarification belongs
in internal specs/plans; no public docs edit is required for this design turn.
It adds no command, configuration, endpoint shape, label or operator procedure.
Do not repair unrelated wording found during the search.

## Work orders

- [x] [Task 01: Classify optional manual CI jobs](task-01-classify-optional-manual.md)

## Verification results

Implementation RED/GREEN, owning Go race package, 57-test frontend matrix,
final 8-test component, focused frontend lint and one frontend typecheck passed.
An authorized frozen install ran once. Canonical local `scripts/lint-go-changed`
for the two staged owned Go files passed with zero issues. Staged i18n ratchet
passed (0 added + 1 modified source file; 643 guard entries intact). See the
order's Results for exact command/exit evidence. Normal hooks and publication
remain delivery steps, with their receipts recorded in the live task plan.

Whole-backend local scans are incomplete, not successful: original 5m
package-loading timeout exited 4; the one authorized 10m/outer620s rerun exited
124 without a terminal report. Both handles actually joined, known linter PID
and current owned processes absent. ROOT clarified from the repository hook
that complete changed-package lint is the initial local gate; CI owns the full
backend scan. No third full scan, resource expansion or workflow weakening.

The following design checks are historical; implementation catalog/spec checks
and whitespace also passed again. No product validation is inferred from them.

Design checks on 2026-10-13 passed:

- `python3 scripts/list-docs.py validate`: 379 decisions, 1580 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests, OK.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Integrations catalog query discovers the amended existing requirement and
  split design. Both remain below their applicable size limits.
- Node24 `validateCoverage` with actual four-document inventory: `ok: true`,
  `errors: []`, documentation-only exempt. Separately labelled hypothetical
  two-production-file trigger: covered by the single order, `errors: []`.
- Four ACs resolve to the existing REQ; the design declares it; the manifest
  and order share the existing design path; exactly one work order exists.
- Full-file readback and four-file inventory checks passed. Product files
  remain unchanged at the qualified head. Whitespace/status gates passed;
  the four docs remain unstaged/uncommitted. Final SHA256 evidence belongs
  to the live task plan so recording it does not invalidate these files.

## Risks

- A blanket allowance rule falsely accepts active/blocking jobs; use an exact
  manual predicate and mixed controls.
- Backend-only repair leaves the live popover inconsistent; both classifiers
  and the real consumer must be covered in the same order.
- Cached totals can retain their old value until ordinary sync; no migration
  or forced refresh is part of this deterministic repair.
- A count can be mistaken for executed-test success or merge authorization;
  preserve raw statuses, failing set and existing readiness gates.
- Heavy-check admission belongs to ROOT; preserve every returned process
  handle and await actual completion serially after the lease is granted.
