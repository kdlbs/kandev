---
id: "05-guidance"
title: "Document Git failure and recovery"
status: done
wave: 5
depends_on: ["04-comparison-recovery"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-SUBPROCESS-ADMISSION-002
acceptance_criteria:
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.1
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.2
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.3
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.4
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.5
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.6
system_design:
  - ../../specs/platform/system-design/git-subprocess-execution.md
---

# Task 05: Document Git failure and recovery

## Summary

Public recovery guidance matches shipped behavior; all task checks and platform limitations have accurate recorded results.

## In scope

Update the existing Git operations how-to with concise failure/recovery guidance.
Link to the merged host-bridge guidance in `docs/public/integrations.md` instead of repeating its setup instructions.
Working SSH and eligible host HTTPS setups need no new user configuration for this fix.
Explain existing-helper retry versus launch/resume re-evaluation when optional bridge registration was skipped.
Preserve backend-account/profile credential-directory scope and remote-executor exclusions.
Do not state that `gh auth token` success proves repository access or token validity.
Explain noninteractive failure and repair in the selected credential environment without asserting token revocation from a provider error.
Do not instruct automatic logout, credential replacement, or transport switching.
Update scoped backend guidance with the required final preparation and direct execution lifecycle helper.
Synchronize design, caller inventory, work-order results, and linked companion-plan follow-up notes with the actual implementation.
Do not overwrite historical test results or mark unrun Windows checks complete.
No push, issue publication, PR, or live-instance operation is authorized by this package.

## Out of scope

Credential-authority changes, transport fallback, live-instance changes, and unrelated refactors.

## Acceptance

- Public recovery guidance matches shipped behavior; all task checks and platform limitations have accurate recorded results.
- Record the implementation work orders' actual checks; run the documentation validators below.
- Preserve the contracts and exclusions in the linked design.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `docs/public/git-operations.md`
- `apps/backend/AGENTS.md`
- `docs/specs/platform/system-design/git-subprocess-execution.md`
- `docs/specs/platform/system-design/workspace-git-status.md`
- `docs/plans/noninteractive-git-execution/`
- `docs/plans/noninteractive-comparison-target-git/plan.md`
- `docs/plans/fork-pr-comparison-targets/plan.md`
- `docs/plans/git-subprocess-admission/plan.md`

## Dependencies

04-comparison-recovery.

## Risks

Credential helper and process-lifecycle compatibility require real subprocess evidence. Do not infer success from environment assertions alone.

## Parallelism

`sequential`

## Inputs

- [Execution design](../../specs/platform/system-design/git-subprocess-execution.md).
- [Plan evidence and caller inventory](plan.md).
- Scoped AGENTS.md and relevant fix, TDD, and E2E skills.

## Results

Implemented after Task 04 on the verified PR #3635 descendant `9cc146ee21c296d553ae684531219a5c711a4213`.

Results: public Git Operations guidance now describes bounded noninteractive authentication failures, selected executor credential repair, existing-helper retry, and launch/resume re-evaluation for a skipped optional host bridge. The scoped backend guide and platform designs describe final preparation, owned lifecycle cleanup, finite post-admission budgets, and exact comparison-ref recovery. Companion plans link to this package without rewriting their historical results.

Checks passed:

- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
