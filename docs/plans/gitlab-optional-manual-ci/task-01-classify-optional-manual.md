---
id: "01-classify-optional-manual"
title: "Classify optional manual GitLab CI jobs consistently"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.16
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.17
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.18
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.19
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-02.md
---

# Task 01: Classify optional manual CI jobs

## Summary

Make backend status/feedback counts and frontend live pass/stage summaries
accept an untriggered optional manual job under the existing pass policy.
Deliver this as one sequential regression-first change, with real provider
API and real first-party component evidence.

## In scope

- Only the Go `pipelineJobBucket` and TypeScript `bucketJob` manual predicate,
  with adjacent invariant comments.
- Extend `pat_client_pipeline_jobs_test.go` and `pipeline-buckets.test.ts`;
  add `mr-ci-popover.optional-manual.test.tsx` independently from committed
  patterns and the documented contract.
- Keep the owning four-document package and live external plan accurate.

## Out of scope

All exclusions in the [manifest](plan.md#scope) apply. In particular, no new
provider actions, raw-status rewrite, automation/readiness changes, data-race
fixes, copy, layouts, dependencies, cache/persistence policies or companion
features. No delegates or new tasks/sessions. No inspection, copying, replay
or mutation of the protected private discovery sources. Preserve others' work.

## Acceptance

1. Real status and feedback API reads report 2/2 for success plus optional
   manual, retaining raw job metadata; cached-field and live display paths
   preserve the design's projection and precedence (AC .16, .19).
2. Backend and frontend tables preserve blocking/nonterminal/failure policy,
   and mixed stages retain failure/in-progress priority and correct hrefs
   rather than becoming falsely all-green (AC .17, .18, .19).
3. The real popover shows corrected live and fallback pass counts with stage
   counts/icons/links from actual classifiers; raw pipeline readiness and the
   failing-job set remain unchanged. All exact task checks pass before done.

## ASCII UI preview

UI-01 from the [full manifest preview](plan.md#ascii-ui-preview): same CI body
inside existing desktop popover and phone drawer, shared data semantics.

```text
Success + optional manual: 1/2 (50%) -> 2/2 (100%)
deploy: yellow 0/1 -> green 1/1
optional + running/blocking: yellow 1/2
optional + real failed: red 1/2
```

AC .16-.19 require these data outcomes, not altered spacing, copy or wrapper
geometry. The existing phone drawer header/body scroll ownership is preserved.

## Implementation sequence

Only after ROOT's later explicit same-primary implementation message and
heavy-check lease, mark this order `in_progress` and re-read its inputs.
Recheck the qualified source baseline; checkpoint ROOT on relevant drift.
The managed workspace's dependencies must already be usable. If absent,
report to ROOT instead of installing or replacing deps without authorization.

1. Independently write the permanent backend/TS/real-component regressions
   before changing either production predicate. Use `@covers` AC references
   near test cases where needed; never embed AC/history text in production.
2. Run the targeted RED commands below. API and live component optional-manual
   assertions must fail on counts/stage behavior, not transport setup,
   imports, wrong Vitest project or compilation. Record actual exits and
   causal assertions; positive controls should already pass.
3. Add only the manual allowance case in both production functions. Update
   adjacent comments with acceptance semantics. Run GREEN, then the complete
   owning Go package and focused frontend regression suites/lint serially.
   Run a single frontend typecheck to validate the real API/store fixture.
   Before first publication, run the existing i18n ratchet against all actually
   staged new/changed owned paths; retain normal active Go/frontend hooks and
   repository-required lint without bypassing them.
4. Record exact final commands/results and update this order/manifest.
   Keep the existing active requirement and unrelated draft split design
   lifecycle honest; do not promote unrelated migrated contracts.

### Independent backend regressions

Extend `TestPipelineJobBucket` with manual true/false and active/unknown true
controls; retain existing success/skipped and failed/canceled both-allowance
coverage. Extend `TestSummarizePipelineJobs` with optional, blocking, running,
pending and real/allowed-failed jobs in one list. Assert literal totals/pass
count and exact failing IDs, retaining raw input values.

Add `TestPATClient_MRJobCounts_OptionalManual` using the committed disposable
`newTestServer` pattern and real `NewPATClient`. Serve full provider-shaped
MR, approval, discussion, pipeline and job endpoints. Exercise BOTH
`GetMRStatus` and `GetMRFeedback` for the two-job 2/2 regression, and mixed
states with explicit expected accepted/total counts. Assert decoded raw
`manual`/allowance/status/URL, raw pipeline status and actual response fields.
No credentials or remote GitLab calls. Do not mock client methods or populate
pipeline counts in the provider fixture; these endpoints must derive them.

### Independent frontend regressions

Extend `bucketJob`, `bucketJobCounts` and `groupJobsByStage` suites in the
existing helper test. Prove optional manual alone and success plus optional;
combine optional with blocking manual, allowed running/pending, non-allowed
failed/canceled, skipped and allowed-failed controls. Cover created, scheduled
and unknown true/false. Assert stage priority, exact counts, raw statuses and
the first dominant-bucket native URL, including optional plus real failure
and optional plus pending in the same stage. Keep empty-stage fallback/order.

New `mr-ci-popover.optional-manual.test.tsx` mounts real `MRCIPopover` with
real store/StateProvider, locale, Tooltip/Toast providers and real
`useMRFeedback`/API/classifiers. Mock only global fetch and serve complete
feedback/files/commits and automation option reads for one disposable MR
identity. No stub popover, helper, feedback hook or API. Settle all responses
and clear providers/timers after each case.

Prove live 2/2 and optional deploy 1/1 green icon/native href even when cached
counts differ. Before a controlled feedback response, assert cached 2/2 and
no stage rows; with an absent live jobs list or unavailable initial feedback,
retain cached summary. These are fallback controls, not a new race contract.
Mixed real-component cases assert 1/2 yellow for optional plus blocking or
allowed active jobs, and 1/2 red with the real-failure href for optional plus
failed. Include raw non-success pipeline state with accepted counts and
otherwise ready MR inputs: no merge action becomes available from counts.
The real component test proves live read/consumer wiring; helper tests alone
or mocked child elements are insufficient.

## Verification

Run from repository root, serially, only with ROOT's implementation and heavy
lease. Existing Node24/Go1.26 paths are fixed; do not search for runtimes,
change caches, install packages or pass all-worker overrides. Capture and
join every long-running handle; the actual exit is authoritative.

RED (before production edits; expected causal exit 1):

```bash
(cd apps/backend && timeout --signal=TERM --kill-after=10s 280s env GOMAXPROCS=2 GOMEMLIMIT=512MiB /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -trimpath -race -tags fts5 -p 1 -count=1 -timeout=240s -run 'Test(PATClient_MRJobCounts_OptionalManual|PipelineJobBucket|SummarizePipelineJobs)$' ./internal/gitlab)
(cd apps/web && timeout --signal=TERM --kill-after=10s 180s env PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" pnpm exec vitest run lib/gitlab/pipeline-buckets.test.ts components/gitlab/mr-ci-popover.optional-manual.test.tsx)
```

GREEN/final (after the last relevant edit):

```bash
(cd apps/backend && timeout --signal=TERM --kill-after=10s 280s env GOMAXPROCS=2 GOMEMLIMIT=512MiB /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -trimpath -race -tags fts5 -p 1 -count=1 -timeout=240s ./internal/gitlab)
(cd apps/web && timeout --signal=TERM --kill-after=10s 180s env PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" pnpm exec vitest run lib/gitlab/pipeline-buckets.test.ts components/gitlab/mr-ci-popover.test.tsx components/gitlab/mr-ci-popover.optional-manual.test.tsx components/gitlab/mr-merge-button.test.tsx lib/gitlab/mr-automation.test.ts)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s env PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" pnpm exec eslint --max-warnings 0 lib/gitlab/pipeline-buckets.ts lib/gitlab/pipeline-buckets.test.ts components/gitlab/mr-ci-popover.optional-manual.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 240s env PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" pnpm run typecheck)
git diff --check
```

Before first publication, after all new/changed owned paths are actually
staged under the later delivery authorization, run the ratchet once with that
complete staged inventory present. The existing runner compares the fork
point to working-tree source; staging new files admits them to that diff.
Confirm no unstaged changes remain in the owned files so the checked content
matches the index. Its normal test-fixture exclusions remain unchanged.
Do not substitute hypothetical paths, run it before staging new paths, or
bypass normal active hooks and required lint. This command remains pending
in DESIGN:

```bash
git diff --cached --name-only
git diff --exit-code -- apps/backend/internal/gitlab/models_pipeline_jobs.go apps/backend/internal/gitlab/pat_client_pipeline_jobs_test.go apps/web/lib/gitlab/pipeline-buckets.ts apps/web/lib/gitlab/pipeline-buckets.test.ts apps/web/components/gitlab/mr-ci-popover.optional-manual.test.tsx docs/specs/integrations/requirements/gitlab-integration.md docs/specs/integrations/system-design/gitlab-integration-02.md docs/plans/gitlab-optional-manual-ci
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s env PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH" pnpm run i18n:ratchet)
```

Leave Vitest project selection at the existing config: the new component
suite belongs to browser-locales, existing helper to its reviewed project.
No-tests/import errors are not evidence. Stop after three focused failures of
the same gate and report ROOT the actual blocker. No broad builds, E2E,
full-web tests/lint or generic QA/review passes are part of this order. The
single frontend typecheck and staged i18n ratchet above are required, pending
implementation/lease authorization; do not run either during design.

Lightweight documentation checks (allowed now and after implementation):

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py specs --system integrations --format paths
git diff --check -- docs/specs/integrations docs/plans/gitlab-optional-manual-ci
git status --short -- docs/specs/integrations docs/plans/gitlab-optional-manual-ci
```

Also call `.github/scripts/pr-docs.cjs`'s `validateCoverage` with Node24 and
actual raw `git diff --name-only -z HEAD` plus `git ls-files --others
--exclude-standard -z` inventory and the four full file contents. Require
`ok: true`, `errors: []`. At design, assert exactly the four documentation
paths; separately test and label a hypothetical planned two-production-file
trigger. Never claim hypothetical source as changed. Verify all four ACs exist
in the owning requirement, the design declares the REQ, the manifest includes
the order's design path, and exactly one order exists. Publish full-file
readback, four-document SHA256 values and actual cheap check results in the
live task plan before handing off.

## Files likely touched

- `apps/backend/internal/gitlab/models_pipeline_jobs.go` (production).
- `apps/backend/internal/gitlab/pat_client_pipeline_jobs_test.go` (existing tests).
- `apps/web/lib/gitlab/pipeline-buckets.ts` (production).
- `apps/web/lib/gitlab/pipeline-buckets.test.ts` (existing tests).
- `apps/web/components/gitlab/mr-ci-popover.optional-manual.test.tsx` (new tests).
- `docs/specs/integrations/requirements/gitlab-integration.md`.
- `docs/specs/integrations/system-design/gitlab-integration-02.md`.
- This order and sibling `plan.md`.

Production popover/hooks/API/services/stores/automation and other-provider
files are audit inputs, not planned edit owners. Any necessary scope expansion
requires a checkpoint with ROOT before changing them.

## Dependencies

None. Later explicit implementation message, usable managed dependencies and
ROOT's global heavy-check lease are execution gates, not work-order dependencies.

## Risks

See [manifest risks](plan.md#risks). The data-only mobile exception is valid
only while shared normalization is the entire frontend production change.
Counts must never substitute for execution evidence or merge readiness.

## Parallelism

`sequential`

## Inputs

- [Existing requirement](../../specs/integrations/requirements/gitlab-integration.md), AC .16-.19.
- [Paired split design](../../specs/integrations/system-design/gitlab-integration-02.md#pipeline-job-summary-classification).
- [Adjacent chip requirement](../../specs/integrations/requirements/gitlab-mr-status-chip.md), existing shared disclosure mechanics only.
- ROOT qualification receipt and qualified source head named in the manifest.
- Scoped backend/web guidance; `/fix`, `/planner-orchestration`, `/plan`,
  `/mobile-parity`, `/docs-maintainer`, `/tdd` and committed adjacent tests.

## Results

Implementation and task-defined local verification are complete after ROOT's
explicit later release. Only the two production manual predicates changed.
The existing requirement's scoped clarification and split-design section
record implemented behavior, retaining unrelated active/draft lifecycles.
Hosted CI, review and merge remain pending delivery evidence.

- Authorized frozen install: exit 0, reused 935, downloaded 0.
- Independent Go RED: exit 1, actual status/feedback 1/2 versus 2/2 and mixed
  4/10 versus 5/10 assertions; controls passed.
- Independent complete frontend RED: exit 1, 30 tests, 16 causal count/bucket
  failures and 14 passing controls, including the real fetch-to-popover path.
- Exact owning Go GREEN command above with `GOTOOLCHAIN=local`: exit 0,
  race-enabled package passed in 18.534s.
- Exact five-suite frontend GREEN command: exit 0, 5 files and 57 tests passed.
  A first log wrapper failed on a reserved shell variable after printing passing
  tests; its exit 1 was not accepted. The corrected wrapper confirmed exit 0.
- Final changed component after duplicate-literal constant extraction: exit 0,
  8 tests passed. Final targeted frontend ESLint: exit 0; its first run found
  two fixture duplicate-string warnings, corrected without type-shape changes.
- Single initial `pnpm run typecheck`: exit 0. Spec/catalog and 36 spec-linter
  regression tests passed again after implementation; whitespace check passed.
- Whole-backend local lint attempts, same base and resource limits: original internal 5m
  run exited 4 at package loading; authorized internal 10m/outer 620s rerun
  exited 124 with no terminal lint report. Neither is a clean lint result.
  Original handle 79393 joined 71bd15; calibrated handle 85300 joined e66e87.
  Known calibrated PID 2273988 and cwd-scoped owned Go/lint processes are absent.
- Canonical initial-local gate: `scripts/lint-go-changed` with both staged owned
  Go files, fixed Go1.26/Node24 PATH, `GOTOOLCHAIN=local`, `GOMAXPROCS=2`,
  `GOMEMLIMIT=512MiB`, outer320s: exit 0, zero issues. Native handle86737
  actually joined fdce99. Repository normal hooks lint complete changed
  packages; the full-backend scan remains a required CI responsibility.
- Staged `pnpm run i18n:ratchet`: exit 0, 0 added + 1 modified source clean,
  643 guard entries intact. Native handle9875 actually joined c68190.
- Normal active commit hooks and publication are next delivery steps, with
  exact receipts recorded in the live external plan; no bypass is permitted.

Durable logs are under `/tmp/kandev-child118-delivery.I1Hk8h`. ROOT resolved
the operational lint scope from the actual repository hook, not a procedure
change. No third full-backend local retry, memory/cache changes or check
weakening occurred. Preserve the original incomplete scan receipts; do not
claim whole-backend local success. No production or test changes followed
passing verification apart from the fixture-literal cleanup noted above.
