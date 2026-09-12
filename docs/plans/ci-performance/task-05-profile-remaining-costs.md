---
id: "05-profile-remaining-costs"
title: "Profile remaining CI costs"
status: done
wave: 5
depends_on: ["04-partition-frontend-verification"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-004
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-004.1
  - AC-PLATFORM-CI-PERFORMANCE-004.2
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 05: Profile remaining CI costs

## Summary

Produce a focused report of E2E fixture/retry costs and Windows compile/test costs.
Select exact follow-up targets from evidence without speculative source changes.

## In scope

- Attempt-specific job and step timings from representative recent runs.
- Existing E2E timing/retry artifacts and Windows sensitive-package logs.
- A ranked report with proposed targets, expected mechanism, confidence, and missing evidence.

## Out of scope

- Application fixes, release builds, cache redesign, reruns, workflow dispatch, and paid-capacity activation.

## Acceptance

- Report approval, dependency, queue, and execution intervals separately, with unknowns explicit.
- Identify the largest measured E2E and Windows costs and distinguish compilation, setup, test bodies, retries, and cache transfer.
- Each follow-up names source/test targets and a reproduction command, or explicitly states why available evidence cannot support a change.

## Verification

Run commands from the repository root. Install workspace dependencies first in a fresh worktree.

```bash
gh run list --repo kdlbs/kandev --workflow e2e-tests.yml --limit 20 --json databaseId,headSha,event,status,conclusion
gh run list --repo kdlbs/kandev --workflow backend-tests.yml --limit 20 --json databaseId,headSha,event,status,conclusion
python3 scripts/lint-spec-files.py --all
git diff --check
```

After selecting run IDs, record each attempt and download its jobs and artifacts through these read-only commands:

```bash
: "${CI_RUN_ID:?Set the selected run ID}"
: "${CI_RUN_ATTEMPT:?Set the selected attempt number}"
gh api --paginate "repos/kdlbs/kandev/actions/runs/$CI_RUN_ID/attempts/$CI_RUN_ATTEMPT/jobs?per_page=100" > /tmp/kandev-ci-profile-jobs.json
gh api "repos/kdlbs/kandev/actions/runs/$CI_RUN_ID/artifacts?per_page=100"
```

The artifact producer must publish an attempt-qualified name. Reject the
unqualified `e2e-timing-diagnostics` name because a rerun can publish the same
name for another attempt. Select and download only the exact name below:

```bash
: "${CI_ARTIFACT_DIR:?Set an empty temporary artifact directory}"
CI_ARTIFACT_NAME="e2e-timing-diagnostics-attempt-${CI_RUN_ATTEMPT}"
ARTIFACTS_JSON="$(gh api "repos/kdlbs/kandev/actions/runs/$CI_RUN_ID/artifacts?per_page=100")"
test "$(jq --arg name "$CI_ARTIFACT_NAME" '[.artifacts[] | select(.name == $name and .expired == false)] | length' <<<"$ARTIFACTS_JSON")" = 1
gh run download "$CI_RUN_ID" --repo kdlbs/kandev --name "$CI_ARTIFACT_NAME" --dir "$CI_ARTIFACT_DIR"
```

Check the downloaded manifest against both `CI_RUN_ID` and `CI_RUN_ATTEMPT`.
Reject the artifact when the exact name is absent, duplicated, expired, or the
manifest identifies another run or attempt. Do not mix retry attempts silently.
Select the Windows job ID from the jobs response, then use the job logs endpoint for package and compile timing.
Delete owned temporary logs after the curated report is complete.

## Files likely touched

- `docs/plans/ci-performance/evidence.md (new)`
- `docs/plans/e2e-ci-efficiency/plan.md (relevant evidence links only)`

## Dependencies

Task 04.

## Risks

Do not treat missing logs as zero execution, or successful-only samples as reliability evidence.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), acceptance IDs in frontmatter.
- [System design](../../specs/platform/system-design/ci-performance.md), corresponding implementation boundary.
- [Plan](plan.md), baseline and companion-package status.
- Existing workflow contract tests under `.github/scripts/`.

## Results

Completed 2026-09-12. Added [CI performance evidence](evidence.md) with
attempt-specific run and job timestamps, approval versus queue versus runner
execution intervals, frontend cache failure evidence, E2E timing and retry
artifact results, and Windows compile, race-test, and cache-save timings. The
report ranks E2E fixture and retry profiling, Windows package profiling, cache
verification, and runner capacity measurement. It names reproduction commands
and records unknowns instead of treating missing jobs or logs as zero time.

The report uses read-only data from runs 34355720657, 34025129928,
34689871445, 34027695660, 34687600985, and 34692849510. Temporary raw logs
and downloaded artifacts remain outside the repository.
