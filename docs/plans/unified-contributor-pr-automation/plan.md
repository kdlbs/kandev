---
created: 2026-08-24
status: in_progress
requirements:
  - ../../specs/ci/requirements/unified-contributor-pr-automation.md
system_design:
  - ../../specs/ci/system-design/unified-contributor-pr-automation.md
legacy_specs:
  - ../../specs/claude-fork-review-allowlist/spec.md
  - ../../specs/pr-walkthrough/spec.md
---

# Unified contributor pull request automation

## Overview

Use one maintainer approval label, `safe-to-review`, for trusted contributor
AI review, preview deployment, and PR walkthrough generation. Extend the
walkthrough workflow to authorized fork pull requests while preserving its
trusted workflow-input boundary. Reuse the existing privileged preview path
selected by the user.

## Scope

- Replace active `safe-to-test` authorization with `safe-to-review`.
- Make the allowlist label bridge add only `safe-to-review`.
- Preserve direct allowlist gates because `GITHUB_TOKEN` label writes do not
  create recursive workflow runs.
- Allow approved fork walkthroughs to fetch and inspect the exact PR head
  without checking it out in the secret-bearing generation worktree.
- Update workflow contract tests, standalone script tests, and related design
  records.
- Provide a post-rollout cleanup step for the old repository label.

## Out of scope

- A redesign of the privileged fork preview executor.
- Changes to AI provider prompts, models, or review follow-up policy.
- Removal of existing direct allowlist variables.
- Kandev application code or UI changes.
- Making `generate-pr-walkthrough` a contributor trust label.

## Technical approach

1. Update the Claude allowlist bridge and preview gate to use one approval
   label, while keeping their direct allowlist paths.
2. Update the walkthrough job gate for approved forks and direct allowlisted
   contributors. Fetch `refs/pull/<number>/head`, verify the exact head SHA,
   and keep all executable assets at `github.workflow_sha`.
3. Keep generation, R2 publication, and PR-body linking as separate jobs.
4. Remove stale test assertions and update contract tests to fail if
   `safe-to-test` returns to an active authorization expression.
5. After deployment, remove old `safe-to-test` labels from open pull requests
   and delete the repository label after confirming no active workflow uses it.

## Work orders

- [x] [Task 01: Unify fork approval label gates](task-01-unify-fork-approval-label.md)
- [x] [Task 02: Enable approved fork walkthroughs](task-02-enable-approved-fork-walkthroughs.md)

## Verification strategy

Workflow contract tests and `zizmor` provide local static evidence. A live
post-merge check must cover an approved labeled fork, an allowlisted fork, an
unauthorized fork, a follow-up push, and approval revocation. No browser E2E is
needed because this change is limited to GitHub Actions.

## Risks

- `safe-to-review` now authorizes the existing preview job to execute
  contributor code with deployment credentials. The label and allowlist must
  remain limited to contributors the maintainers trust.
- The pull request ref must resolve to the event head SHA. A mismatch must
  fail before context generation.
- The old label can remain visible on existing pull requests during migration.
  It must not remain active in workflow expressions.

## Verification results

The implementation checks pass. The direct `zizmor` audit reports only the
intentional `pull_request_target` trigger findings on the four affected
workflows. The old checkout credential-persistence findings are resolved.
The PR fixup also addressed the remaining single-label wording in the legacy
specification and tightened the artifact-upload pin assertion to cover every
upload reference. The focused OpenCode test and specification lint passed.
The final PR check snapshot had 15 passed, 0 failed, and 0 pending checks, with
no unresolved review threads and a clean merge state. Post-merge live
verification and the one-time `safe-to-test` label cleanup remain operational
follow-up.
