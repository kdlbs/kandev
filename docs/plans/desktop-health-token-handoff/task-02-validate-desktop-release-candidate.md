---
id: "02-validate-desktop-release-candidate"
title: "Validate desktop release candidate"
status: pending
wave: 2
depends_on: ["01-restore-desktop-token-ownership"]
plan: "plan.md"
spec: "../../specs/executors/requirements/port-collision-safety.md"
---

# Task 02: Validate desktop release candidate

## Acceptance

- Task 01 is committed, reviewed, merged to `main`, and required checks are green.
- A `desktop_validation_only` Release workflow run from that exact `main` commit succeeds for all
  five runtime bundles and desktop targets.
- The validation run creates no release PR, tag, GitHub Release, GHCR tag, npm package, or Homebrew
  update.

## Verification

Confirm `main` contains the merged fix and capture that immutable commit before dispatching the
non-publishing validation mode:

```bash
main_sha=$(gh api repos/kdlbs/kandev/git/ref/heads/main --jq '.object.sha')
test -n "$main_sha"
gh workflow run release.yml --repo kdlbs/kandev --ref "$main_sha" \
  -f channel=stable \
  -f bump=patch \
  -f dry_run=false \
  -f desktop_validation_only=true \
  -f backfill_tag=
gh run watch <validation-run-id> --repo kdlbs/kandev --exit-status
validation_head_sha=$(gh run view <validation-run-id> --repo kdlbs/kandev --json headSha --jq '.headSha')
test "$validation_head_sha" = "$main_sha"
gh run view <validation-run-id> --repo kdlbs/kandev --json headSha,jobs,url
```

Inspect the job list rather than only the aggregate conclusion. Record successful runtime and
desktop jobs for Linux x64/arm64, macOS x64/arm64, and Windows x64, plus skipped publication jobs.
After the run, verify that no release PR, `v0.86.2` tag or GitHub Release, GHCR `0.86.2` tag,
npm `0.86.2` package, or Homebrew `0.86.2` formula update exists.

## Files likely touched

- `docs/plans/desktop-health-token-handoff/plan.md`
- `docs/plans/desktop-health-token-handoff/task-02-validate-desktop-release-candidate.md`

## Dependencies

Task 01 and its merged fix PR.

## Parallelism

`sequential`. Validation must use the merged fix commit and the repository-wide Release workflow.

## Inputs

- Task 01 results and merged PR URL.
- `.github/workflows/release.yml`, `desktop_validation_only` mode.
- `docs/public/release-process.md`, Before dispatch and Desktop validation requirements.

## Output contract

Report the exact `main` SHA, workflow run URL, per-target results, absence of publication side
effects, blockers, risks, and synchronized task/plan status in the primary conversation.

## Results

Pending.
