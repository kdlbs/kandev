---
id: "02-retry-incomplete-opencode-output"
title: "Retry incomplete OpenCode output"
status: done
wave: 2
depends_on: ["01-unify-trusted-workflow-provenance"]
plan: "plan.md"
spec: "../../specs/pr-walkthrough/spec.md"
---

# Task 02: Retry Incomplete OpenCode Output

Retry once when OpenCode exits zero without both rendered files.

- **Acceptance:** The adapter runs at most two OpenCode attempts. It retries
  only when an attempt exits zero without both non-empty final files.
- **Acceptance:** Before each attempt, the adapter writes an empty draft and
  removes final JSON and HTML files. A retry cannot accept stale output.
- **Acceptance:** Each attempt stores separate status, standard output,
  standard error, draft, and partial output diagnostics.
- **Acceptance:** A non-zero OpenCode exit fails immediately. A second
  incomplete zero-exit attempt fails before publication.
- **Acceptance:** Failing contract assertions cover the retry rules before the
  workflow change.
- **Verification:**

  ```text
  python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
  python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py
  python3 .github/scripts/lint-action-pinning_test.py
  python3 .github/scripts/lint-action-pinning.py
  actionlint .github/workflows/pr-walkthrough.yml
  git diff --check
  ```

- **Files likely touched:** `.github/workflows/pr-walkthrough.yml`,
  `.github/scripts/pr-walkthrough-workflow-contract_test.py`,
  `docs/plans/pr-walkthrough-runner-reliability-fix/plan.md`, and this task
  file.
- **Dependencies:** Task 01.
- **Parallelism:** sequential. This task extends Task 01 in the same workflow
  and contract test.
- **Inputs:** The spec failure modes and retry scenarios, Task 01, and run
  `32644590701` diagnostics.
- **Output contract:** Report changed files, exact test results, diagnostics,
  blockers, risks, and plan or task status changes.

## Results

Implemented the bounded clean retry in `.github/workflows/pr-walkthrough.yml`.
Each attempt resets the draft and final files, captures status/stdout/stderr
and partial outputs under its own diagnostics directory, retries only after a
zero exit with incomplete outputs, and fails before publication after the
second incomplete attempt or any non-zero exit.

Validation passed:

- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` - 22 tests.
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py` - 4 tests.
- `python3 .github/scripts/lint-action-pinning_test.py` - 9 tests.
- `python3 .github/scripts/lint-action-pinning.py` - 20 workflow files.
- `actionlint .github/workflows/pr-walkthrough.yml` - passed with v1.7.7 in a temporary directory.
- `git diff --check` - passed.
