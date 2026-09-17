---
id: "07-private-ci"
title: "Optional private CI"
status: pending
wave: 2
depends_on: ["00-private-publication"]
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 07: Optional private CI

## Summary

Add a narrowly scoped private validation workflow if remote checks are useful.
This is optional for the first local pilot; inherited upstream Actions remain
disabled until publication/review integrations are explicitly isolated.

## In scope

1. Inventory inherited workflows and their scheduled, push, PR, reusable-workflow,
   registry and third-party review triggers. Identify all that must remain off.
2. Add a private validation workflow for feature branches/manual dispatch with
   read-only repository permissions, concurrency cancellation and bounded jobs.
   Reuse verified toolchain setup and locked dependencies; do not use live data.
3. Restrict enabled workflows before enabling repository Actions. Confirm dormant
   upstream schedules cannot begin on enablement. Do not enable repo-wide execution
   first and try to disable publishing afterward.
4. Run docs/typecheck/unit/lint plus a scoped synthetic browser job if resources
   permit. Keep disposable PostgreSQL isolated, avoid overlapping full suites,
   redact logs and set short retention for test artifacts.
5. Review allowed actions, app access and token permissions. Add no production
   secrets or external review integration by default. Record a trial run and
   verify no release, registry write, public artifact or external post occurred.

## Out of scope

Stable/nightly publishing, package/container uploads, automatic dependency merges,
provider credentials, production data and making CI mandatory before local work.

## Acceptance

- Only the intended private validation workflow can execute; publication and
  external messaging workflows stay disabled.
- A trial run completes with minimal permissions, synthetic fixtures and bounded
  artifact retention, and its result is tied to an exact commit.
- The review entry explains what CI covers and which local/operational checks
  still run separately.

## Verification

```bash
gh api repos/Corey-Fogg/kandev-orchestration/actions/permissions
gh workflow list --repo Corey-Fogg/kandev-orchestration --all
gh run list --repo Corey-Fogg/kandev-orchestration --limit 10
git diff --check
```

Verify individual workflow states and run permissions before enabling Actions.
Fetch current GitHub Actions documentation when implementing configuration/API
changes; the initial publication does not require activating this task.

## Files likely touched

Private workflow file, guarded inherited workflow configuration if necessary,
repository Actions settings and private review documentation. Keep CI-only
changes out of the later public coordinator patch series.

## Dependencies

Delivery 00. Does not block delivery 01–06 when local checks supply the evidence.

## Risks

Enabling inherited schedules or registry jobs can publish artifacts unexpectedly.
Review the complete trigger set and default-branch behavior before activation.

## Parallelism

`sequential`

## Inputs

Existing `.github/workflows/`, repository Actions permission state and local
qualification commands.

## Results

Pending, optional. Actions are currently disabled for the private import.
