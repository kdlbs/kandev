---
id: "05-verification-and-commit"
title: "Required verification and commit"
status: done
wave: 4
depends_on: ["02-backend-issue-workflow", "03-frontend-dialog-and-mobile", "04-e2e-coverage"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/improve-kandev.md"
---

# Task 05: Required verification and commit

## Acceptance

- The user-requested repository formatting, typecheck, test, and lint commands
  pass after all feature files and planning statuses are final.
- The spec remains `building`, every implementation task and plan checkbox is
  updated accurately, and `git diff --check` is clean.
- All scoped changes are committed with a Conventional Commit and normal active
  pre-commit/commit-msg hooks.

## Verification

```bash
make fmt
make typecheck test lint
git diff --check
```

Then follow `.agents/skills/commit/SKILL.md` with explicit `git add` paths and a
message such as:

```text
feat: add issue-only Improve Kandev workflow
```

## Files likely touched

- Files owned by Tasks 01–04.
- `docs/specs/workspaces/requirements/improve-kandev.md`
- `docs/plans/improve-kandev/plan.md`
- `docs/plans/improve-kandev/task-*.md`

## Dependencies

Tasks 02, 03, and 04.

## Parallelism

Sequential final gate.

## Inputs

- Original Improve phase requirement to run `make fmt` before
  `make typecheck test lint`.
- `.agents/skills/commit/SKILL.md`.
- Completed task verification receipts.

## Risks

- The environment was nearly out of disk space during the planning session.
  Check `df -h .` before dependency installation or a broad build, and clean
  only recoverable generated/cache artifacts if needed.
- Do not bypass hooks. If formatting hooks modify files, review, restage, and
  create a new commit attempt without amending.

## Output contract

Mark every task and plan checkbox accurately, provide the commit hook receipt,
record exact verification results, and stop so the user can move the Kandev
task to the next workflow step.

## Recorded verification

- `make fmt` — passed; Go and frontend formatters completed.
- Initial `make typecheck test lint` was stopped by the RTK-injected indexed
  `GIT_CONFIG_*` environment in the unrelated
  `TestCollectAgentEnvPreservesParentIndexedGitConfig` test. With those
  environment variables unset, the exact `make typecheck test lint` command
  passed: all backend tests, 922 web test files (7,037 passing and 4 skipped),
  30 CLI test files, script tests, backend/web/harness lint, and typechecks.
- `git diff --check` — passed.

All verification is complete and the scoped changes are ready for the final
Conventional Commit.
