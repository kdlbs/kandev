---
id: "11-pr-scope-evidence"
title: "Narrow PR scope and remove raw evidence"
status: in_progress
wave: 5
depends_on:
  - "01-source-adoption"
  - "07-remove-host-write-api"
  - "08-retire-legacy-write-path"
  - "09-shims-and-snapshots"
  - "10-release-toggle"
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-005
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-005.5
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 11: Narrow PR scope and remove raw evidence

## Summary

Stop 36 MB of evidence and unrelated fixes landing in `main` with this feature. Keep the evidence readable, and do not rewrite branch history.

## In scope

- Extract unrelated production fixes into separate PRs, then merge `main` so they leave this PR's diff. Candidates: `port_tunnel.go`, the remote helper race in `agentctl_resolver.go`, `manager_startup.go`, readiness timestamps in `task_notifications.go` and `agentctl-readiness`, `git-status.ts`, `use-pr-commits.ts`, `task-navigation.ts`, and `changes-panel-file-row.tsx`.
- Extract E2E stabilization, the shard timeout, and the lint timeout in the same way when they are not needed by usage history.
- Remove `docs/plans/usage-history/evidence/**` and its `.gitattributes` rule in a normal commit.
- Keep a short evidence summary in `docs/plans/usage-history/plan.md`, with permanent links to the last commit that contains the raw files.

## Out of scope

- Force-pushing or rewriting history.
- Merging any PR. Each push and PR needs user authorization when this runs.

## Acceptance

- `git diff --name-only origin/main...HEAD -- docs/plans/usage-history/evidence` is empty.
- Every remaining changed production file maps to a usage history requirement, with exceptions listed in Results.
- Summary links resolve on GitHub through a commit reachable from `refs/pull/3660/head`.

## Verification

```bash
git fetch origin main
git diff --name-only origin/main...HEAD -- docs/plans/usage-history/evidence | wc -l
git diff --numstat origin/main...HEAD | awk '{a+=$1} END {print a}'
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check origin/main...HEAD
```

## Files likely touched

- `docs/plans/usage-history/plan.md`
- `.gitattributes`
- The extracted fix files listed above, in their own branches.

## Dependencies

Tasks 01, 07, 08, 09, and 10.

## Risks

- Some "unrelated" E2E changes may be what made usage suites pass. Check each against CI before extracting it.
- Existing evidence links use repository-relative paths. They must become commit links, or the documentation link checks fail.

## Parallelism

`sequential`

## Inputs

- PR body and commit list of #3660.
- The `/pr` and `/push` skills, when the user authorizes them.

## Results

Raw evidence files were removed from the current diff in a normal staged deletion, and `docs/plans/usage-history/plan.md` now links to the permanent evidence-bearing commit `a0d13b9de8248f8ea8af8130e0dc7609ba030c0f`. No force-push or history rewrite was used. The evidence cleanup acceptance check is expected to pass after the current branch update.

The unrelated production and E2E fixes listed above have not all been extracted into separate PRs. This work order remains in progress until that scope review is complete; the current PR body will identify the remaining exceptions and open validation gates.
