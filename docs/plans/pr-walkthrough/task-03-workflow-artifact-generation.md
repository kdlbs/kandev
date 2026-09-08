---
id: "03-workflow-artifact-generation"
title: "Workflow artifact generation"
status: done
wave: 2
depends_on: ["01-walkthrough-skill-renderer", "02-agent-rendering-contract"]
plan: "plan.md"
spec: "../../specs/ui/requirements/pr-walkthrough.md"
---

# Task 03: Workflow artifact generation

Add a dedicated PR walkthrough workflow with a generation job. The job checks
out the immutable base commit, fetches the PR head only as a Git object, uses
base-commit copies of the skill and helpers, gives OpenCode narrow trusted
rendering and PR-file tools, and uploads the agent-generated files and logs for
the separate R2 publication job.

- **Acceptance:** A non-draft authorized pull request event invokes OpenCode,
  creates distinct `docs/pr-walkthrough/pr-<number>.json` and `.html` files,
  and uploads them as an artifact. The generation job runs on `opened`,
  `reopened`, `ready_for_review`, `synchronize`, and the
  `generate-pr-walkthrough` label.
- **Acceptance:** The job cannot execute PR-owned scripts through OpenCode or
  modify source. OpenCode may invoke only the base-controlled rendering
  adapter, which writes the fixed walkthrough outputs. The job does not request
  `id-token`, hosting credentials, R2 credentials, or GitHub write permissions.
- **Acceptance:** A contract test covers the trigger, immutable base checkout,
  head-object fetch, constrained PR-file reads, trusted base assets, read-only
  agent permissions, output paths, artifact upload, and generation-only
  boundary. The artifact handoff is explicit for the dependent R2 publication
  job.
- **Acceptance:** The workflow uses `PR_WALKTHROUGH_ENABLED`, the
  `opencode-go/muse-spark-1.2-contributor` model reference, and the model's
  native `high` reasoning variant. The workflow passes the variant through
  `--variant`. The normal review workflow remains independently gated by
  `OPENCODE_REVIEW_ENABLED`.
- **Verification:**

  ```text
  python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
  python3 .github/scripts/lint-action-pinning_test.py
  python3 .github/scripts/lint-action-pinning.py
  zizmor .github/workflows/pr-walkthrough.yml
  git diff --check
  ```

- **Files likely touched:** `.github/workflows/pr-walkthrough.yml`,
  `.github/workflows/lint-action-pinning.yml`,
  `.github/scripts/pr-walkthrough-workflow-contract_test.py`, `.gitignore`.
- **Dependencies:** Tasks 01 and 02.
- **Parallelism:** sequential, because it edits the shared OpenCode workflow
  and consumes both earlier artifacts.
- **Inputs:** The spec's permissions, failure modes, persistence guarantees,
  and scenarios; tasks 01 and 02; current OpenCode workflow gates and action
  pinning conventions.
- **Output contract:** Report changed files, contract/security check results,
  artifact paths from any local dry run, and any unavailable external CI
  dependency. Mark the task done after implementation and verification. This
  task does not publish to R2; task 04 owns that boundary.

## Results

Added the isolated same-repository OpenCode generation job to the dedicated
`pull_request_target` walkthrough workflow. It materializes the skill,
renderer, shell,
and narrow rendering adapter from the base SHA. OpenCode builds and repairs the
JSON/HTML through that adapter while generic edit and Bash remain denied. The
job preserves logs and outputs as a uniquely keyed artifact, including on
failure.

The three OpenCode jobs now share a base-SHA-materialized setup action with one
version and checksum. The walkthrough generation worktree stays on the trusted
base commit. A bounded helper reads regular UTF-8 files from the immutable PR
head Git object without exposing PR-controlled filesystem entries. Walkthrough
artifacts use a visible staging directory and fail when empty. Workflow-level
per-PR concurrency serializes generation, publication, and linking, and the
walkthrough label no longer runs the normal code-review job.

The walkthrough has its own enable variable and uses the Muse Spark contributor
model with its native high-reasoning variant. The workflow passes the model and
variant through separate OpenCode 1.17.7 CLI options. Live validation on PR
`#2936` found and removed the invalid `#high` model suffix.

Verification: workflow contract tests passed; action pinning tests and linter
passed; `actionlint` passed; `git diff --check` passed. The targeted `zizmor`
audit still reports the workflow's existing `pull_request_target` warning.
