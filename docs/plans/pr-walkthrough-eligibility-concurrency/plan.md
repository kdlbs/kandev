---
created: 2026-09-22
status: complete
requirements:
  - REQ-CI-PR-WALK-003
  - REQ-CI-PR-TRUST-001
  - REQ-CI-PR-FAIL-005
system_design:
  - ../../specs/ci/system-design/unified-contributor-pr-automation.md
legacy_specs: []
---

# Implementation Plan: PR walkthrough eligibility concurrency

## Overview

Update the PR walkthrough workflow so an event rejected by the generation gate
cannot cancel an eligible contributor walkthrough run. Keep latest-run
cancellation for eligible triggers, preserve the existing authorization gate,
and add workflow contract regression coverage.

## Scope

### In scope

- Separate every event rejected by the full generation gate from the eligible
  per-pull-request concurrency group.
- Preserve `safe-to-review` as the only contributor approval label.
- Add contract tests for the reported `safe-to-review` and `safe-to-test` race
  and an unauthorized fork non-labeled event.
- Document the manual rerun procedure for an approved contributor PR.

### Out of scope

- Changing walkthrough generation, rendering, publication, or R2 hosting.
- Adding a new GitHub Actions trigger or changing the model configuration.
- Removing labels or changing PR #3708 from this repository.

## Technical approach

Change `.github/workflows/pr-walkthrough.yml` so its concurrency expression
mirrors the complete generation job gate, including the feature toggle, event
type, draft state, repository, action, label, and allowlist conditions. Eligible
same-repository `generate-pr-walkthrough` events and authorized contributor
events use the existing shared group. Every rejected event uses an
`-ineligible` suffix. Keep `cancel-in-progress: true` and the job-level
authorization expression unchanged.

Extend `.github/scripts/pr-walkthrough-workflow-contract_test.py` to require the
ineligible group branch and the complete source, action, label, and allowlist
conditions. The test continues to require latest-run cancellation and the
absence of `safe-to-test` from authorization expressions.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| AC-CI-PR-WALK-003.1 | Existing generation gate contract plus the eligible-group assertions. |
| AC-CI-PR-WALK-003.2 | Existing generation and publication contract tests. |
| AC-CI-PR-WALK-003.5 | New concurrency contract tests covering eligible labels, ineligible labels, and unauthorized fork non-labeled events. |
| AC-CI-PR-TRUST-001.2 | Existing assertion that `safe-to-test` is absent from authorization. |
| AC-CI-PR-FAIL-005.1 | Existing trusted job-gate contract plus the new group isolation assertion. |

## Operational rerun

For an approved contributor PR, a new head commit triggers `synchronize` while
the `safe-to-review` label remains present. A label-only rerun can remove any
stale `safe-to-test` label, remove `safe-to-review`, and add `safe-to-review`
again. The `safe-to-test` removal is not a trigger; the final `safe-to-review`
addition is the trigger. Same-repository PRs can use the existing
`generate-pr-walkthrough` label.

## Work orders

- [x] [Task 01: Guard ineligible walkthrough triggers](task-01-guard-ineligible-triggers.md)

## Verification results

- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` passed.
- `python3 .github/scripts/lint-action-pinning_test.py` passed.
- `python3 scripts/list-docs.py validate` passed.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
- `zizmor .github/workflows` ran but remains non-zero for existing findings,
  including the pre-existing `pull_request_target` warning in the changed
  walkthrough workflow; the new concurrency expression introduced no finding.
- `go run github.com/rhysd/actionlint/cmd/actionlint@latest` with
  `.github/workflows/pr-walkthrough.yml` passed.

## Risks

- The workflow-level expression must remain semantically aligned with the
  complete job-level generation gate, including its toggle and draft checks.
- GitHub evaluates concurrency before the job condition, so the contract test
  must protect the group split as well as the authorization expression.
