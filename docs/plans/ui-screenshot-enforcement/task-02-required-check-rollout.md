---
id: "02-required-check-rollout"
title: "Activate the required screenshot check"
status: pending
wave: 2
depends_on: ["01-validate-evidence"]
plan: "plan.md"
requirements:
  - REQ-CI-UI-SCREENSHOTS-003
acceptance_criteria:
  - AC-CI-UI-SCREENSHOTS-003.7
system_design:
  - ../../specs/ci/system-design/ui-screenshot-evidence.md
---

# Task 02: Activate the required screenshot check

## Summary

Make the deployed screenshot status a required merge condition after controlled live proof.

## In scope

- After merge, run authorized same-repository and fork smoke PRs for pass, fail, body edit, and exception-label add/remove.
- Capture current main rules and the observed trusted integration before preparing the minimal ruleset delta.
- After explicit administrator authorization, add only the required context and read the live rule back.
- Backfill existing open PRs through supported retries and prove missing evidence blocks an otherwise qualified smoke PR.

## Out of scope

Unapproved hosted mutations, unrelated ruleset changes, automatic merging, and creating a queue solely for a smoke test.

## Acceptance

1. The record identifies deployed workflow SHA, smoke PR/head, trusted status origin, and the live required context.
2. Missing evidence blocks the smoke PR. Valid evidence satisfies this check. Every previous required check and bypass actor remains preserved.
3. This work order and package remain pending until remote enforcement is verified. Local CI success alone cannot complete rollout.

## Verification

Run from the repository root:

```bash
gh api repos/kdlbs/kandev/rulesets/13341245
gh api repos/kdlbs/kandev/rulesets/13341245 | jq -e 'any(.rules[] | select(.type == "required_status_checks") | .parameters.required_status_checks[]; .context == "UI screenshots")'
gh api "repos/kdlbs/kandev/commits/${KANDEV_UI_SMOKE_HEAD}/status" --jq '.statuses[] | select(.context == "UI screenshots") | {state, context, target_url}'
gh pr view "$KANDEV_UI_SMOKE_PR" --repo kdlbs/kandev --json headRefOid,mergeStateStatus,statusCheckRollup
git diff --check -- docs/plans/ui-screenshot-enforcement
```

## Files likely touched

- `GitHub main ruleset 13341245 (authorized hosted mutation only)`
- `docs/plans/ui-screenshot-enforcement/task-02-required-check-rollout.md`
- `docs/plans/ui-screenshot-enforcement/plan.md`

## Dependencies

Complete `01-validate-evidence` first.

## Risks

Rules can change concurrently. Refresh them before the authorized update and preserve unrelated fields. Hosted smoke interactions also require authorization.

## Parallelism

`sequential`

## Inputs

Read Trusted workflow and rollout in the design and current GitHub Actions execution policy. Set KANDEV_UI_SMOKE_PR and KANDEV_UI_SMOKE_HEAD to the authorized smoke PR and exact source head.

## Execution notes

This work order requires merged implementation and explicit administrator authorization for hosted policy and smoke interactions.
Prepare the exact delta and smoke procedure before that authorization request.
Do not change live rules, create PRs, post comments, or remove queue entries merely to complete local implementation.

## Results

Pending implementation.
