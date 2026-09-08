---
id: "02-release-documentation"
title: "Document the Stable release bypass"
status: done
wave: 2
depends_on: ["01-release-workflow"]
plan: "plan.md"
spec: "../../specs/release/requirements/release-pr-queue-bypass.md"
---

# Task 02: Document the Stable release bypass

## Acceptance

- Maintainer documentation states the token permissions, owner requirement, expiration, rotation, environment configuration, and administrator bypass.
- Repository guidance explains that ordinary pull requests still use the merge queue and required CI.
- The repair spec is `shipped`, and the ADR matches the implemented token boundary.

## Verification

- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `python3 scripts/lint-harness-files.test.py`
- `python3 .github/scripts/lint-harness-files.py --all`
- `git diff --check -- docs/public/release-process.md docs/ci-merge-queue.md .agents/skills/release/SKILL.md AGENTS.md docs/specs/release/requirements/release-pr-queue-bypass.md docs/specs/INDEX.md docs/decisions/2026-08-17-release-pr-ruleset-bypass.md docs/decisions/INDEX.md`

## Files likely touched

- `docs/public/release-process.md`
- `docs/ci-merge-queue.md`
- `.agents/skills/release/SKILL.md`
- `AGENTS.md`
- `docs/specs/release/requirements/release-pr-queue-bypass.md`
- `docs/specs/INDEX.md`
- `docs/decisions/2026-08-17-release-pr-ruleset-bypass.md`
- `docs/decisions/INDEX.md`

## Dependencies

Task 01 must pass before this task starts.

## Parallelism

Sequential. The documents record the workflow contract from Task 01.

## Inputs

- The implemented workflow and contract from Task 01.
- `docs/specs/release/requirements/release-pr-queue-bypass.md`.
- ADR `2026-08-17-release-pr-ruleset-bypass`.
- The existing how-to structure of the release page.

## Output contract

Report the documentation class, changed files, validator results, and external rollout requirements. Update this task and `plan.md` in the same conversation.

## Results

- Public docs updated: `docs/public/release-process.md` remains a how-to guide and now includes the PAT creation and rotation procedure.
- Internal docs updated: `docs/ci-merge-queue.md`, `.agents/skills/release/SKILL.md`, `AGENTS.md`, the repair spec, and the ADR.
- `node --test scripts/validate-public-docs.test.mjs` passed 61 tests.
- `node scripts/validate-public-docs.mjs` validated 41 pages.
- `python3 scripts/lint-harness-files.test.py` passed 19 tests.
- `python3 .github/scripts/lint-harness-files.py --all` passed 118 files.
- `pre-commit run harness-lint --files AGENTS.md .agents/skills/release/SKILL.md` passed.
- `git diff --check` passed.
