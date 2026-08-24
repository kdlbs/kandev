---
spec: docs/specs/ui/requirements/pr-walkthrough.md
created: 2026-08-23
status: done
---

# Implementation Plan: PR Walkthrough Runner Reliability Fix

## Overview

Use the workflow commit for every trusted input in the PR walkthrough pipeline.
Then add one clean retry for a zero-exit OpenCode attempt that produces no
complete output. The tasks are sequential because both change the same
workflow and contract test.

## Root Cause

Run `32650200507` published the correct 12-character URL for PR `#2957`.
However, the PR-link job checked out the stale event base SHA. Its old helper
required a 40-character URL, so the link job failed.

Run `32644590701` for PR `#2955` had a different error. OpenCode exited zero
after read calls, but it did not render the walkthrough. The draft stayed
empty, and both final files were absent. The workflow had no clean retry for
this incomplete result.

## Architecture Decision

Follow
[ADR-2026-08-23-pr-walkthrough-workflow-provenance](../../decisions/2026-08-23-pr-walkthrough-workflow-provenance.md).
Use `github.workflow_sha` for all executable workflow inputs. Keep the exact
event head SHA as immutable, untrusted data.

## CI Workflow

### Trusted workflow provenance

Update `.github/workflows/pr-walkthrough.yml`. Rename the workflow-controlled
commit variable from `BASE_SHA` to `TRUSTED_SHA`. Set it from
`github.workflow_sha` in generation and PR-link steps.

Check out `github.workflow_sha` in both jobs. Make sure that the checked-out
`HEAD` equals `TRUSTED_SHA`. Use `TRUSTED_SHA` for all skill, action, guidance,
context, comparison, and helper reads.

Keep `HEAD_SHA` fixed to `github.event.pull_request.head.sha`. Fetch this
commit as an object. Do not check it out or use it for executable content.

### Incomplete agent retry

Keep the OpenCode command in the provider adapter. Run at most two attempts.
Accept an attempt only when OpenCode exits zero and both final files are
non-empty.

If the first attempt exits zero without both files, save its diagnostics. Then
remove the draft and final files. Start the second attempt with an empty draft.

Use a separate diagnostics directory for each attempt. Save the process
status, standard output, standard error, draft, and any partial final files.
If OpenCode exits non-zero, fail immediately. If the second attempt is
incomplete, fail without publication.

## Tests

- **What:** Every executable input and comparison base uses one workflow SHA.
  **File:** `.github/scripts/pr-walkthrough-workflow-contract_test.py`.
  **How:** Add failing assertions for `github.workflow_sha`, `TRUSTED_SHA`, and
  the absence of event-base executable reads. Then change the workflow.
- **What:** A zero-exit attempt without both files gets one clean retry.
  **File:** `.github/scripts/pr-walkthrough-workflow-contract_test.py`.
  **How:** Add failing assertions for the two-attempt bound, state reset,
  output predicate, per-attempt diagnostics, and terminal failure.
- **What:** Existing skill, helper, URL, action-pinning, and workflow syntax
  contracts remain valid.
  **Files:** Existing PR walkthrough tests and action-pinning tests.
  **How:** Run all focused commands from the task files.

## E2E Tests

There is no Kandev UI change. The workflow contract tests provide local
coverage. A same-repository pull request provides the live workflow test after
the implementation reaches `main`.

## Verification Results

The shared focused checks passed after both workflow changes:

- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` — 22 tests passed.
- `python3 scripts/pr-walkthrough-pr-body.test.py` — 8 tests passed.
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py` — 4 tests passed.
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py` — 4 tests passed.
- `python3 .github/scripts/lint-action-pinning_test.py` — 9 tests passed.
- `python3 .github/scripts/lint-action-pinning.py` — 20 workflow files passed.
- `actionlint .github/workflows/pr-walkthrough.yml` — passed with actionlint v1.7.7 installed in a temporary directory because no local binary was present.
- `git diff --check` — passed.

## PR Fixup Results

The PR review found that the link job rebuilt the public URL with the full head
SHA. The job now reads the validated URL output from the publish job. It also
rejects an empty output before it updates the PR body.

The workflow prompt now uses the terms `trusted workflow checkout` and
`trusted workflow commit`. The contract test rejects the old terms and local
URL reconstruction.

The fixup checks passed:

- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` - 22 tests passed.
- `python3 scripts/pr-walkthrough-pr-body.test.py` - 8 tests passed.
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py` - 4 tests passed.
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py` - 4 tests passed.
- `python3 .github/scripts/lint-action-pinning_test.py` - 9 tests passed.
- `python3 .github/scripts/lint-action-pinning.py` - 20 workflow files passed.
- `python3 scripts/lint-spec-files.py --all` - passed.
- `actionlint .github/workflows/pr-walkthrough.yml` - passed with actionlint v1.7.12.
- `git diff --check` - passed.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Unify trusted workflow provenance](task-01-unify-trusted-workflow-provenance.md)

Wave 2:

- [x] [Task 02: Retry incomplete OpenCode output](task-02-retry-incomplete-opencode-output.md)

Run both tasks sequentially in the primary conversation. They change the same
workflow and contract test.

## Post-Merge Validation

Open a same-repository dummy pull request after this fix reaches `main`. Make
sure that generation, publication, and PR-link jobs pass. The published URL
must contain the 12-character head prefix. Close the dummy pull request after
the live test passes.

## Out of Scope

- Do not change the portable skill or renderer contract.
- Do not add retries for non-zero OpenCode exits.
- Do not generate walkthroughs for fork pull requests.
- Do not add Claude or Codex provider adapters in this fix.
