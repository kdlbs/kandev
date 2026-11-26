---
created: 2026-11-16
status: integration-required
requirements:
  - REQ-CI-UI-SCREENSHOTS-001
  - REQ-CI-UI-SCREENSHOTS-002
  - REQ-CI-UI-SCREENSHOTS-003
system_design:
  - ../../specs/ci/system-design/ui-screenshot-evidence.md
legacy_specs: []
---

# Implementation plan: Require screenshots on UI PRs

## Overview

Add a required screenshot-presence check using the existing PR-check pattern.
Deliver one small script, one workflow, focused tests, and matching contributor instructions.
Activate the required status after merge and live verification.

## Scope

UI candidate paths require an image in the Screenshots section.
The exact `no-screenshots-allow` label is the maintainer exception.
Apply the same rule to forks and internal PRs.
Keep affected-viewport capture guidance in the existing PR skill and semantic review.

Exclude image downloads, image-to-commit declarations, custom exception commands, and a general policy framework.
Merge-queue support is deferred until a queue is actually introduced.

## Technical approach

Follow the [design](../../specs/ci/system-design/ui-screenshot-evidence.md).
Reuse the existing GitHub API client and trusted metadata-only workflow pattern.
Parse the Screenshots section rather than counting unrelated images elsewhere in the body.
Do not execute contributor code or fetch screenshot URLs.

## Tests

| Acceptance | Primary proof |
| --- | --- |
| UI-SCREENSHOTS-001.1 through .8 | UI/non-UI fixtures, real image embed, plain link, local path, unrelated badge, and clear failure message |
| UI-SCREENSHOTS-002.1 through .4 | Exact label, lookalike label, body self-waiver, label add/remove |
| UI-SCREENSHOTS-003.1, .2, .3, .6 | Fork metadata, event refresh, complete pagination, stale head, and trusted workflow contract |
| UI-SCREENSHOTS-003.7 | Live required rule and blocked/pass smoke evidence |

This is repository policy work. It does not change product UI.

## Work orders

- [x] [Task 01: Add the screenshot presence check](task-01-validate-evidence.md)
- [ ] [Task 02: Activate the required check](task-02-required-check-rollout.md)

Execute sequentially. Task 02 runs after Task 01 is merged and hosted changes are authorized.

## Verification results

Task 01 local implementation and its focused tests are complete. The
`UI screenshots` workflow passes its targeted `zizmor` audit, action pinning,
workflow-contract tests, and public documentation validation. The repository-
wide `zizmor` command still reports findings in unrelated workflows; see Task
01 for the affected paths. The backend build and lifecycle work-order checks
are recorded in the lifecycle plan.

Task 02 remains pending. It requires this implementation to be merged, smoke
PRs to be authorized, and an administrator to authorize the minimal live
ruleset change. Local success does not establish required merge enforcement.

## Risks

The path rule can produce false positives. Maintainers use the existing-style exception label for those cases.
An image can be irrelevant or stale. The check does not replace visual review.
Do not report merge enforcement before the live ruleset requires the context.
