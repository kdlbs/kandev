---
id: "05-dedicated-workflow-pr-link"
title: "Separate walkthrough workflow and link it from the PR body"
status: done
wave: 4
depends_on: ["04-r2-html-publication"]
plan: "plan.md"
spec: "../../specs/ui/requirements/pr-walkthrough.md"
---

# Task 05: Separate walkthrough workflow and link it from the PR body

Move walkthrough generation and publication out of the OpenCode review
workflow. Give walkthroughs their own enable variable and model configuration,
then add a post-publication job that makes the hosted page prominent without
taking ownership of contributor-authored PR content.

- **Acceptance:** `.github/workflows/pr-walkthrough.yml` contains generation,
  R2 publication, and PR linking. `.github/workflows/opencode-code-review.yml`
  contains no walkthrough jobs.
- **Acceptance:** `PR_WALKTHROUGH_ENABLED` controls the walkthrough workflow
  independently of `OPENCODE_REVIEW_ENABLED`.
- **Acceptance:** The walkthrough runner selects
  `opencode-go/muse-spark-1.2-contributor` and passes `high` through the
  OpenCode 1.17.7 `--variant` option.
- **Acceptance:** Only the final job receives `pull-requests: write`. It runs
  after public R2 validation, uses the trusted base-commit helper, and receives
  no OpenCode or R2 credential.
- **Acceptance:** The helper places one marker-owned GitHub alert at the top of
  the PR description, replaces it idempotently on rerun, preserves all other
  content, and fails closed on invalid markers or mismatched PR identity.
- **Verification:**

  ```text
  python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
  python3 scripts/pr-walkthrough-pr-body.test.py
  actionlint .github/workflows/pr-walkthrough.yml
  zizmor .github/workflows/pr-walkthrough.yml
  git diff --check
  ```

- **Files likely touched:** `.github/workflows/pr-walkthrough.yml`,
  `.github/workflows/opencode-code-review.yml`,
  `.github/scripts/pr-walkthrough-workflow-contract_test.py`,
  `scripts/pr-walkthrough-pr-body`, and
  `scripts/pr-walkthrough-pr-body.test.py`.

## Results

Created the dedicated workflow and independent gate. The workflow passes the
Muse Spark model and its native `high` variant through separate OpenCode 1.17.7
CLI options. Added the marker-owned PR-body helper and a minimum-permission link
job after R2 public validation. Contract and helper tests pass.
