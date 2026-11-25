---
id: "20-measure-cache-and-partition-adoption"
title: "Measure cache and partition adoption"
status: in_progress
wave: 4
depends_on: ["19-balance-frontend-partitions"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-004
  - REQ-PLATFORM-CI-PERFORMANCE-005
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-004.1
  - AC-PLATFORM-CI-PERFORMANCE-004.2
  - AC-PLATFORM-CI-PERFORMANCE-004.4
  - AC-PLATFORM-CI-PERFORMANCE-005.1
  - AC-PLATFORM-CI-PERFORMANCE-005.6
  - AC-PLATFORM-CI-PERFORMANCE-005.7
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 20: Measure cache and partition adoption

## Summary

Decide each candidate using complete coverage and total occupied runner time.
Keep scheduling delays separate and retain every attempted measurement.

## In scope

- Add a `compare` mode to the frontend evidence helper, with executable identity/outcome regressions.
- Compare baseline and candidate artifacts by project, file, full test title, multiplicity, and terminal outcome.
- Use the existing read-only CI efficiency helper with attempt-specific data and verified workflow graphs.
- Collect three comparable baseline/candidate pairs for each cache or partition decision.
- Record cold population, warm restore, publication, setup, verification, artifact, gate, and full workflow intervals.
- Record runner labels, source, workflow identity, image digest, toolchain, profile mode, test counts, failures, skips, and retries.
- Preserve failed/cancelled/partial samples. Exclude them from successful-pair medians without hiding them.
- Record an adopted, rejected, or pending disposition for each candidate in the existing evidence note.
- Keep companion browser-cache rollout and complete-CI acceptance statuses independent.

## Out of scope

- Extra jobs, fleet activation, dispatches, commits, pushes, or PR operations without their separate authorization.
- Gate routing, repository-variable changes, billing changes, percentile claims from three samples, and guaranteed queue reductions.
- Completion claims for historical Tasks 02–04 or 14–16.
- Broad local verification or unrelated application remediation.

## Acceptance

1. Every comparison binds complete native identities/outcomes and attempt-specific workflow evidence to its source and declared cache state.
2. Adopted balance changes lower median execution critical path without higher median runner-minutes across three comparable pairs.
3. Adopted cache changes lower median total setup/verification occupancy. Reports include loaded feedback, failed attempts, separate waiting, and rejected or pending candidates.

Insufficient evidence leaves adoption pending. A measured regression requires rollback or a revised candidate.
Do not label lower queue delay as faster execution.
Do not attribute waiting to saturation without an overlapping concurrent-job observation.

## Verification

Run local helper checks from the repository root:

```bash
node --test .github/scripts/frontend-timings.test.mjs
python3 .github/scripts/ci-efficiency_test.py
python3 docs/plans/ci-performance/experiments/measurement-scripts_test.py
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The new `compare` mode rejects missing/duplicate identities, changed outcomes, skipped-only evidence, inconsistent provenance, and malformed reports.
Comparable timing requires identical application/test source, selection, runner class, toolchain, and declared cache state.
Record the candidate's intended configuration difference explicitly.

For each actual hosted run, set a positive run ID and attempt before this read-only collection:

```bash
: "${CI_RUN_ID:?Set an actual hosted run ID}"
: "${CI_RUN_ATTEMPT:?Set its observed attempt}"
python3 .github/scripts/ci-efficiency.py \
  --run-id "$CI_RUN_ID" --attempt "$CI_RUN_ATTEMPT" \
  --output "/tmp/kandev-ci-${CI_RUN_ID}-${CI_RUN_ATTEMPT}.json"
```

The helper leaves scheduling and execution critical path unknown without a verified graph.
For final comparisons, create a graph matching the exact workflow revision and every expanded job name.
Pass that graph through `--graph` on the same command.
Save raw API evidence, graphs, reports, and logs outside the repository.

`compare` validates complete identity and outcome parity. It is not the before/after performance measurement, and it requires timing-enabled artifact sets from the same source revision.
Retain the historical pre-instrumentation run for the hosted timing comparison, then produce a separate timing-enabled fallback run at the candidate source SHA as the paired identity baseline.
After you download both complete timing-enabled artifact sets, run the comparison entry point:

```bash
: "${CI_BASELINE_DIR:?Set the baseline artifact directory}"
: "${CI_CANDIDATE_DIR:?Set the candidate artifact directory}"
node .github/scripts/frontend-timings.mjs compare \
  --baseline-dir "$CI_BASELINE_DIR" --candidate-dir "$CI_CANDIDATE_DIR"
```

Use naturally occurring runs or separately authorized measurements, one candidate at a time.
Do not queue duplicate full suites to obtain samples faster.
Only update these work orders after their actual acceptance evidence exists.

## Files likely touched

- `.github/scripts/frontend-timings.mjs` and `.github/scripts/frontend-timings.test.mjs`: comparison mode and failure fixtures.
- `docs/plans/ci-performance/evidence-2026-10-26.md`.
- `docs/plans/ci-performance/plan.md` and Tasks 17–20: exact results and statuses.
- `docs/plans/e2e-ci-efficiency/plan.md`: link relevant evidence without closing its independent rollout requirements.

## Dependencies

Tasks 17–19 supply the candidates and their targeted results.
Task 20's evidence requirements apply from the first baseline, not only after implementation.

## Risks

Source changes, cache cold starts, differing runners, and incomplete artifact sets can invalidate a comparison.
The shared 60-job limit can dominate workflow elapsed time despite lower execution costs.
Naturally occurring comparable samples can take time. Unobserved evidence remains pending.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), acceptance IDs in frontmatter.
- [Design](../../specs/platform/system-design/ci-performance.md#october-26-cache-and-partition-continuation).
- [Diagnostic evidence](evidence-2026-10-26.md), which is not an adoption baseline.
- Existing CI efficiency helper, measurement entry-point tests, and native JSON reports.
- [Browser-cache rollout](../e2e-ci-efficiency/plan.md).

## Results

Implemented `compare` mode for complete native frontend artifact sets. It reconciles each two-shard set, requires matching source SHA, project/file identities, current file hashes, test titles, multiplicities, and outcomes, rejects skipped-only or incomplete evidence, and reports run/attempt provenance, allocation mode, outcome counts, retries, and flaky tests. The pure helper and actual CLI tests pass.

Hosted cache and partition samples have not been collected. Three matched pairs per candidate, attempt-specific workflow evidence, exact cache state, and loaded feedback are still required. Keep adoption pending; no speedup or cache benefit is claimed. No workflow dispatch, commit, push, or repository-variable change was performed.
