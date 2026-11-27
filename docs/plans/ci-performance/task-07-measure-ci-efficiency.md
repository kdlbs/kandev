---
id: "07-measure-ci-efficiency"
title: "Measure CI execution and occupied capacity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-004
  - REQ-PLATFORM-CI-PERFORMANCE-005
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-004.1
  - AC-PLATFORM-CI-PERFORMANCE-004.2
  - AC-PLATFORM-CI-PERFORMANCE-005.1
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 07: Measure CI execution and occupied capacity

## Summary

Report execution cost and scheduling delay from attempt-specific GitHub evidence. Keep the helper read-only and outside the always-on workflow graph.

## In scope

- Add `.github/scripts/ci-efficiency.py` with JSON-file input and optional bounded GET collection by run ID and attempt.
- Emit workflow/job/step provenance, runner-minutes, incomplete data, and dependency-ready scheduling gaps when the matching graph is known.
- Add fixture tests for overlapping jobs, reruns, cancelled/unfinished/skipped jobs, missing timestamps, and incorrect graph identity.

## Out of scope

Workflow dispatch, automated reruns, fleet settings, external telemetry services, or another required CI job.

## Acceptance

- A fixture with overlapping jobs reports their summed occupancy separately from elapsed time; missing execution is unknown.
- A rerun report cannot silently combine attempts or treat skipped timestamps as execution.
- The October samples are reproducible; unknown scheduling causes remain labelled unknown.

## Verification

Run from the repository root. Install workspace dependencies once in a fresh worktree.
New test and helper paths below are implementation deliverables.

```bash
python3 .github/scripts/ci-efficiency_test.py
python3 .github/scripts/ci-efficiency.py --run-id 37826061394 --attempt 1 --output /tmp/kandev-ci-4352-efficiency.json
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `.github/scripts/ci-efficiency.py (new)`
- `.github/scripts/ci-efficiency_test.py (new)`
- `docs/plans/ci-performance/evidence-2026-10-08.md`

## Dependencies

None.

## Risks

Historical workflow definitions and cache evidence can be unavailable. Report that limit instead of constructing unsupported queue or speedup claims.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), IDs in frontmatter.
- [System design](../../specs/platform/system-design/ci-performance.md), October continuation.
- [October evidence](evidence-2026-10-08.md).
- [Plan](plan.md), acceptance gates and preserved companion statuses.

## Results

Implemented the read-only helper and seven regression tests. PR 4352 attempt 1
reproduces 29 jobs, 546.4 runner-minutes, and 7,610 seconds observed elapsed.
Without a verified graph, the helper explicitly leaves critical path and
dependency-ready delay unknown. No workflow was dispatched.
