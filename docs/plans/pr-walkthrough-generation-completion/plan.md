---
created: 2026-09-30
status: done
requirements:
  - REQ-UI-PR-WALKTHROUGH-001
system_design:
  - ../../specs/ui/system-design/pr-walkthrough.md
legacy_specs: []
---

# Implementation Plan: PR walkthrough generation completion

## Overview

Make a verified render complete generation without another model response. First add actionable errors, a render receipt, and independent verification. Then integrate bounded process supervision and targeted history fetching into the workflow.

The existing walkthrough requirement owns the portable artifact contract. CI retains authorization, workflow events, permissions, and publication credentials.

## Evidence and root cause

[Run 36729911406](https://github.com/kdlbs/kandev/actions/runs/36729911406/job/109936460895) timed out while generating PR #4068. Checkout took about 3m35s. OpenCode ran for about 11m17s before cancellation.

Artifact 11105731845 records four JSON syntax errors, one missing-title error, and two successful renders. No completion marker or attempt status file exists. The saved final JSON parses successfully.

The adapter waits for process exit before accepting files. Its retry handles incomplete zero-exit attempts but cannot handle a process that remains live. The renderer hides the JSON parser's error position. The logs do not explain why the model remained running after rendering.

## Scope

### In scope

- JSON errors with the file, parser message, line, and column.
- A renderer-owned completion receipt and independent output verification.
- Owned-process supervision with a total deadline and retained diagnostics.
- Targeted Git histories with an exact-head check and a merge-base regression.
- Workflow wiring and focused CI test registration.

### Out of scope

- Model, provider, reasoning variant, or data-use policy changes.
- Page layout, localization, schema expansion, or browser behavior changes.
- New retry classes, external model fallback, or publishing cancelled runs.
- R2, description-write, eligibility, and concurrency policy changes.

## Technical approach

Task 01 extends the portable renderer and adds the fixed `pr-walkthrough-verify` entry point. It records hashes only after both outputs are complete. Verification uses `build.build()` for an in-memory comparison and does not rewrite final files.

Task 02 adds `.github/scripts/pr-walkthrough-runner.py` as the trusted host adapter. It monitors the receipt, stops live members of its process group, treats zombie-only groups as stopped, reaps its direct child, and runs the verifier. A 600-second total deadline covers both attempts. The generation job uses a 20-minute outer limit with bounded preparation steps and time for verification and artifact upload.

Checkout uses depth one for the trusted SHA. A trusted history helper fetches only the trusted SHA and PR-head ref without shallow boundaries. It does not fetch every branch or tag. It fails before generation if the exact event head or merge base is unavailable.

| Runner or transport | Completion behavior | Evidence | Unsupported condition |
| --- | --- | --- | --- |
| Current pinned OpenCode process on Ubuntu | Stop after receipt, then verify | Real renderer plus a fake agent process | Unexpected exit or invalid pair fails |
| Future runner with owned-process supervision | Same receipt and verifier contract | Adapter-specific process tests required | No implicit claim of provider support |
| Immutable Git objects over origin | Fetch two histories and preserve trusted checkout | Local bare remote with divergent branches | Missing object or merge base fails |

## Tests

| Acceptance criterion | Regression evidence |
| --- | --- |
| AC-UI-PR-WALKTHROUGH-001.12 | `test_invalid_json_reports_location` covers raw tabs, unescaped quotes, and malformed escapes in the renderer suite. |
| AC-UI-PR-WALKTHROUGH-001.13 | `test_render_then_idle_completes` uses the real renderer and a fake agent that remains live. |
| AC-UI-PR-WALKTHROUGH-001.13 | Verifier tests cover identity, hashes, schema, HTML comparison, and incomplete pairs. |
| AC-UI-PR-WALKTHROUGH-001.14 | Runner tests cover timeout, cancellation, stale receipts, unexpected exits, cleanup, and one bounded retry. |
| AC-UI-PR-WALKTHROUGH-001.7 | Workflow contract tests preserve fixed paths, command, trusted checkout, and narrow tool permissions. |

## End-to-end evidence

The runner integration uses the actual portable renderer and verifier in a temporary worktree. It requires no model credential or browser. A post-merge eligible workflow run remains necessary to observe GitHub publication and linking. A `pull_request_target` run on this implementation PR will still consume trusted base code.

## Work orders

- [x] [Task 01: Report rendering errors and verify completion](task-01-render-completion.md)
- [x] [Task 02: Supervise generation and narrow Git fetching](task-02-bounded-runner.md)

Tasks run sequentially. Task 02 consumes Task 01's receipt and verifier.

## Verification results

Implementation is complete. Task 01 and Task 02 are done. Focused regression
checks passed:

- Renderer diagnostics and receipt tests: 10 passed; verifier tests: 8 passed; renderer builder tests: 71 passed.
- Runner supervision tests: 17 passed, including already-observed non-zero exits taking precedence over receipts, natural zero-exit receipt acceptance, receipt-before-renderer-exit completion, zombie-only process groups, terminal cleanup failure, and cleanup diagnostics; verifier tests: 8 passed, including import and syntax error reporting; history tests passed against shallow and complete clones with blob filtering; workflow contract tests: 30 passed.
- Context tests: 4 passed; PR-body tests: 9 passed; action-pin tests: 9 passed; all 25 workflow files passed action-pin scanning.
- `go run github.com/rhysd/actionlint/cmd/actionlint@latest` passed for both changed workflows.
- Python compile checks, Bash syntax checks, harness lint, `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
- `zizmor .github/workflows` still exits 14 for existing repository findings. The focused walkthrough audit reports only the existing `pull_request_target` finding, which also reproduces against the workflow at `HEAD`; this change adds no findings.

## Related delivery records

The [portable runner package](../pr-walkthrough-portable-runner-fix/plan.md) established the current draft and renderer permissions. The [runner reliability package](../pr-walkthrough-runner-reliability-fix/plan.md) added the incomplete zero-exit retry. Their completed results remain historical evidence. This package replaces their process-exit acceptance rule and broad checkout fetch, while preserving their trust boundary and retry class.

## Risks

- A receipt is not enough without post-stop validation and process cleanup.
- A second render can race with supervision. Hash disagreement must fail closed.
- Narrow fetching must not recreate the earlier shallow merge-base failure.
- The new budget bounds execution but does not guarantee provider response time.
- Live rollout requires the workflow and helpers to reach the trusted branch together.
