---
id: "05-dogfood-evidence"
title: "Dogfood evidence and fixes"
status: pending
wave: 7
depends_on: ["04-live-pilot"]
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 05: Dogfood evidence and fixes

## Summary

Use real daily coordinator work to discover gaps while keeping review evidence
free of real prompts. Reproduce findings with synthetic fixtures before turning
them into upstream issues, patches or media.

## In scope

1. Record candidate/version/flags and intended workflows: answering a workspace
   status question, delegating a small task, adopting existing work, handling
   review/input, restart/reconnect and optional manual Automation delivery.
2. For each trial record outcome, elapsed recovery, duplicate/missing task or
   callback count, account correctness, status freshness and whether the native
   input control matched the overview. Avoid transcript and task-title dumps.
3. Distinguish UI/data bugs, authority failures, provider quality and unavailable
   integration/account support. An unsupported provider does not become a code
   success or failure without the capability contract.
4. Reproduce each actionable defect on generic fixtures. Link a focused fix work
   order/commit, affected acceptance criterion and failing/passing check. Apply the
   repository's behavior-change planning rule; do not patch the running binary.
5. Rebuild and requalify changed candidates, repeat migration rehearsal for
   schema/ownership changes and repeat pilot smoke after deployment. Keep current
   installed SHA visible in a private receipt so review does not drift from use.
6. Summarize evidence for contribution: completed normal work cycles, a restart,
   no unexplained duplication/account/ownership regressions and outstanding
   limitations. Capture shareable UI media only in a fresh fictional workspace.

## Out of scope

Public posting without instruction, collecting user prompts as telemetry,
autonomous approvals, recurring schedules without an explicit trial and adding
unplanned product scope under the label of a bug fix.

## Acceptance

- At least one complete normal workflow and one controlled restart have outcomes
  tied to a candidate, with failures categorized and reproduced where possible.
- Fixed findings link to scoped commits and regression evidence; unresolved
  limitations and any unsupported provider paths remain visible.
- Committed/public-facing evidence contains only generic reproductions and safe
  aggregate observations, not real prompts or customer/account details.

## Verification

```bash
git rev-parse HEAD
git status --porcelain
git diff --check
```

Use each linked work order's tests for fixes. Inspect screenshots/video frames
and captions before publication; secret scanning cannot detect all prompt leaks.
Do not introduce a calendar-only pass criterion such as “ran for one day.”

## Files likely touched

`docs/review/orchestration/dogfood-results.md` (new), regression work orders and
synthetic fixtures/media in their owning feature package.

## Dependencies

Delivery 04. Continue normal use after initial acceptance; this record can grow
without relabeling later assistant experiments as coordinator validation.

## Risks

“No error seen” without exercising a workflow is weak evidence. Every claim must
state what actually ran, including any deliberately untested external operation.

## Parallelism

`sequential`

## Inputs

Pilot receipt, full scope audit, central-view acceptance and synthetic-media rules.

## Results

Pending. Existing pre-rebase demo media is a demonstration, not daily-use evidence.
