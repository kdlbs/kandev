---
id: "01-unify-trusted-workflow-provenance"
title: "Unify trusted workflow provenance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/pr-walkthrough/spec.md"
---

# Task 01: Unify Trusted Workflow Provenance

Use one immutable workflow commit for all trusted code and comparison inputs.

- **Acceptance:** Generation and PR-link jobs check out
  `github.workflow_sha`. Each job makes sure that its checked-out `HEAD` equals
  `TRUSTED_SHA`.
- **Acceptance:** The workflow uses `TRUSTED_SHA` for the skill bundle, setup
  action, repository guidance, context base, diff range, and PR-body helper.
  It does not use the event base SHA for executable inputs.
- **Acceptance:** The workflow still fetches the exact event head SHA as a Git
  object. It does not check out contributor-controlled code.
- **Acceptance:** A failing contract test covers a stale event base SHA before
  the workflow change.
- **Verification:**

  ```text
  python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
  python3 scripts/pr-walkthrough-pr-body.test.py
  python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py
  python3 .github/scripts/lint-action-pinning_test.py
  python3 .github/scripts/lint-action-pinning.py
  actionlint .github/workflows/pr-walkthrough.yml
  git diff --check
  ```

- **Files likely touched:** `.github/workflows/pr-walkthrough.yml`,
  `.github/scripts/pr-walkthrough-workflow-contract_test.py`,
  `docs/plans/pr-walkthrough-runner-reliability-fix/plan.md`, and this task
  file.
- **Dependencies:** None.
- **Parallelism:** sequential. Task 02 changes the same workflow and test file.
- **Inputs:** The workflow provenance ADR, the spec permissions and stale-base
  scenario, GitHub Actions context semantics, and run `32650200507`.
- **Output contract:** Report changed files, exact test results, trust
  boundaries, blockers, risks, and plan or task status changes.

## Results

Implemented in `.github/workflows/pr-walkthrough.yml` and covered by the
workflow contract test. Generation and PR-link jobs now check out and verify
`github.workflow_sha` through `TRUSTED_SHA`; the event head remains an
immutable fetched object and is never checked out.

Validation passed:

- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` - 22 tests.
- `python3 scripts/pr-walkthrough-pr-body.test.py` - 8 tests.
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py` - 4 tests.
- `python3 .github/scripts/lint-action-pinning_test.py` - 9 tests.
- `python3 .github/scripts/lint-action-pinning.py` - 20 workflow files.
- `actionlint .github/workflows/pr-walkthrough.yml` - passed with v1.7.7 in a temporary directory.
- `git diff --check` - passed.

## PR Review Fixup

The link job now uses the validated URL output from the publish job. It rejects
an empty URL and does not rebuild the URL from the full head SHA.

The contract test now covers this data flow. It also requires consistent
trusted-workflow terms in the agent prompt.

Fixup validation passed:

- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` - 22 tests.
- `python3 scripts/pr-walkthrough-pr-body.test.py` - 8 tests.
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py` - 4 tests.
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py` - 4 tests.
- `python3 .github/scripts/lint-action-pinning_test.py` - 9 tests.
- `python3 .github/scripts/lint-action-pinning.py` - 20 workflow files.
- `python3 scripts/lint-spec-files.py --all` - passed.
- `actionlint .github/workflows/pr-walkthrough.yml` - passed with v1.7.12.
- `git diff --check` - passed.
