---
spec: docs/specs/pr-walkthrough/spec.md
created: 2026-08-22
status: done
---

# Implementation Plan: Portable PR Walkthrough Runner Fix

## Overview

Repair the failed PR-head fetch before the workflow prepares agent context.
Then replace the OpenCode TypeScript tools with a provider-neutral filesystem
contract. Package every reusable generation helper and its focused tests inside
the `pr-walkthrough` skill directory. Keep the existing local setup action,
publication job, and PR-link job as GitHub-specific adapters.

The helper changes are independent. The workflow integration consumes both
helpers and removes the OpenCode-specific tools last.

## Root Cause

The workflow checks out the full base history, then fetches the PR head with
`--depth=1`. Git records the PR head as a shallow root. The later triple-dot
diff cannot find a merge base and exits with code 128.

An isolated Git reproduction reports `shallow=true` and no merge base with the
current command. The same reproduction succeeds after the depth limit is
removed.

## Architecture Decision

Follow
[ADR-2026-08-22-pr-walkthrough-filesystem-runner](../../decisions/2026-08-22-pr-walkthrough-filesystem-runner.md).
The runner receives trusted files and a fixed prompt. It can change one draft
JSON file and invoke one renderer command.

## Repository Harness

### Trusted PR-head context

Add
`.agents/skills/pr-walkthrough/scripts/pr-walkthrough-context`. The helper
receives the repository path, base SHA, head SHA, and output directory. It
resolves the merge base and reads changed files from Git objects without a
PR-head checkout.

The helper writes `manifest.json` and regular UTF-8 files under `head/files/`.
It rejects unsafe paths and non-regular Git entries. It records binary,
oversized, deleted, and budget-excluded files in the manifest.

Keep the existing 512 KiB limit for each file. Add an 8 MiB total limit for
materialized file data. Remove the dynamic `pr-walkthrough-read-file` helper
after the new context test covers its security cases.

Keep the context helper and its test inside the skill directory. Do not retain
a repository-root compatibility copy after the workflow migration.

### Fixed renderer entry point

Add `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-render`. It reads
`.pr-walkthrough/draft.json`. The command takes no model-controlled arguments
and continues to bind trusted PR identity from environment values. It resolves
`references/build.py` and `references/shell.html` relative to its own skill
directory so the complete skill folder remains copyable.

The helper writes the final JSON and HTML through its existing atomic output
path. A renderer error leaves no partial final output. Update the
provider-neutral skill with the fixed draft and renderer command contract for
managed CI runners.

## CI Workflow

### PR history and context preparation

Change `.github/workflows/pr-walkthrough.yml` to fetch the exact head SHA
without `--depth=1`. Assert that `git merge-base "$BASE_SHA" "$HEAD_SHA"`
succeeds before the workflow computes the triple-dot diff.

Materialize the complete skill directory and setup action from the exact base
SHA. Prepare the bounded PR-head filesystem context with the skill-local helper
before the agent starts.

### Prompt-driven OpenCode runner

Keep `.github/actions/setup-opencode/action.yml` as the pinned installation
adapter. Invoke `opencode run` with one fixed prompt in the workflow.

The OpenCode agent can read, glob, and grep the trusted worktree. It can edit
only `.pr-walkthrough/draft.json`. Its Bash rules permit only the exact
skill-local renderer command.

Remove `.github/scripts/opencode-pr-file-tool.ts` and
`.github/scripts/opencode-pr-walkthrough-tool.ts`. Do not create
`.opencode/tools` during the job.

Keep the generation logs, draft, manifest, final JSON, and final HTML in the
diagnostic artifact. Keep R2 credentials and GitHub write permissions out of
the generation job.

## Tests

- **What:** Context preparation reads exact regular UTF-8 blobs and rejects
  unsafe, binary, symlink, oversized, and over-budget inputs.
  **File:**
  `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py`.
  **How:** Use temporary Git repositories and Python standard-library tests.
- **What:** The renderer reads only the fixed draft, binds trusted identity,
  and leaves no partial outputs after an error.
  **File:**
  `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py`.
  **How:** Write controlled drafts in temporary worktrees and invoke the fixed
  command.
- **What:** The workflow preserves a merge base and prepares trusted context.
  It uses the local setup action, fixed prompt, narrow write access, and one
  renderer command.
  **File:** `.github/scripts/pr-walkthrough-workflow-contract_test.py`.
  **How:** Add failing contract assertions before the workflow change.
- **What:** The workflow contains no OpenCode TypeScript tools or unpinned
  action references.
  **File:** `.github/scripts/pr-walkthrough-workflow-contract_test.py` and
  `.github/scripts/lint-action-pinning.py`.
  **How:** Run the workflow contract, action-pinning tests, and `actionlint`.

## E2E Tests

There is no Kandev UI change. Local tests cover the trusted context and runner
contract. PR #2936 provides the live post-merge workflow test.

## Post-Merge Validation

After the fix reaches `main`, add the `generate-pr-walkthrough` label to PR
`#2936`. Require successful generation, publication, and link jobs. Open the
hosted HTML and make sure that the PR description contains the owned callout.

Close PR #2936 and remove its remote branch after the live test succeeds.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential in this primary session):

- [x] [task-01-trusted-pr-context](task-01-trusted-pr-context.md)
- [x] [task-02-fixed-renderer-contract](task-02-fixed-renderer-contract.md)

Wave 2:

- [x] [task-03-prompt-driven-workflow](task-03-prompt-driven-workflow.md)

Wave 3:

- [x] [task-04-public-url-and-shell-presentation](task-04-public-url-and-shell-presentation.md)

Execute tasks sequentially in the primary conversation unless the user
explicitly authorizes subagents.

## Verification Results

- Task 01 targeted tests passed: 4 tests.
- Task 02 targeted tests passed: 4 renderer tests and 59 renderer reference
  tests. Harness validation passed.
- Task 03 contract and security checks passed: 20 workflow contract tests, 9
  action-pinning tests, 19 pinned workflow files, and actionlint v1.7.12.
- `git diff --check` passed.

## Out of Scope

- Generic R2 retention or merge-promotion changes beyond the 12-character URL
  contract recorded in ADR-2026-08-23-pr-walkthrough-short-urls.
- Generic GitHub automation for Claude or Codex.
- Walkthrough generation for fork pull requests.
