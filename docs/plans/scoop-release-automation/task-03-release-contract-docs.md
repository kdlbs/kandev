---
id: "03-release-contract-docs"
title: "Reconcile release documentation"
status: done
wave: 3
depends_on: ["02-release-workflow"]
plan: "plan.md"
spec: "../../specs/release/requirements/scoop-release-automation.md"
---

# Task 03: Reconcile release documentation

## Acceptance

- Public and internal release guidance lists Scoop as a Stable sibling channel
  with its own repository-scoped deploy key.
- The backfill decision and Nightly records state that backfill includes Scoop
  and Nightly excludes Scoop.
- The repair spec and index are `shipped`. Public docs and harness files pass
  their focused validation.

## Verification

- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `python3 scripts/lint-harness-files.test.py`
- `python3 .github/scripts/lint-harness-files.py --all`
- `pre-commit run harness-lint --files AGENTS.md .agents/skills/release/SKILL.md`
- `git diff --check -- AGENTS.md .agents/skills/release/SKILL.md apps/cli/README_internal.md docs/public/release-process.md docs/decisions/0029-release-backfill-and-desktop-diagnostics.md docs/decisions/2026-07-31-npm-nightly-release-channel.md docs/specs/release/requirements/npm-nightly-channel.md docs/specs/release/requirements/scoop-release-automation.md docs/specs/INDEX.md`

## Files likely touched

- `AGENTS.md`
- `.agents/skills/release/SKILL.md`
- `apps/cli/README_internal.md`
- `docs/public/release-process.md`
- `docs/decisions/0029-release-backfill-and-desktop-diagnostics.md`
- `docs/decisions/2026-07-31-npm-nightly-release-channel.md`
- `docs/specs/release/requirements/npm-nightly-channel.md`
- `docs/specs/release/requirements/scoop-release-automation.md`
- `docs/specs/INDEX.md`

## Dependencies

Task 02 must pass before this task starts.

## Parallelism

Sequential. These records must use the final workflow and secret names.

## Inputs

- The complete repair spec and release workflow from Tasks 01 and 02.
- The release skill and the Release and Versioning section in `AGENTS.md`.
- `docs/public/release-process.md`, classified as a how-to guide.
- ADR 0029 and the npm Nightly decision and spec.

## Output contract

Report each changed record, the public-docs classification, exact validation
results, and any stale release references found. Update this task and
`plan.md` in the same conversation.

## Results

Green:

- `node --test scripts/validate-public-docs.test.mjs` passed: 58 tests.
- `node scripts/validate-public-docs.mjs` passed: 41 published pages.
- `python3 scripts/lint-harness-files.test.py` passed: 19 tests.
- `python3 .github/scripts/lint-harness-files.py --all` passed: 118 files.
- `pre-commit run harness-lint --files AGENTS.md
  .agents/skills/release/SKILL.md` passed.
- The task's `git diff --check` command passed.

Public documentation remains a focused how-to guide and now covers Scoop in
Stable targets, credentials, publication order, verification, and backfill
recovery. Internal skill, CLI, ADR, Nightly, spec, and index records all state
that Scoop is a Stable sibling channel and uses its own deploy key.
