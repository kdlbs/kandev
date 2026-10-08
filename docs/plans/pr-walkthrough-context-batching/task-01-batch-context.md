---
id: "01-batch-context"
title: "Batch immutable context reads and promised blobs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PR-WALKTHROUGH-001
acceptance_criteria:
  - AC-UI-PR-WALKTHROUGH-001.1
  - AC-UI-PR-WALKTHROUGH-001.6
  - AC-UI-PR-WALKTHROUGH-001.7
system_design:
  - ../../specs/ui/system-design/pr-walkthrough.md
---

# Task 01: Batch immutable context reads and promised blobs

## Scope

Own `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-context` and
`.agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py`.
Batch literal tree lookup within argument limits. Reuse metadata and content
readers. Prefetch only missing changed regular blobs through the configured
promisor remote in one request before opening readers. Preserve Git 2.43
compatibility, immutable head identity, framing, limits, and exclusion reasons.
Do not move refs, modify the checkout, or write `FETCH_HEAD`.

## Acceptance and regression coverage

- A 200-file change materializes exact head contents with fewer than 20 Git
  processes. The old implementation uses 662 and fails this assertion.
- A real 50-file partial clone materializes exact content with no more than two
  upload-pack requests. Persistent readers without prefetch make 54 and fail.
- Empty files, missing final newlines, literal glob characters, Unicode, and
  newline-containing paths preserve contents and following records.
- Existing unsafe-path, symlink, deleted, binary, file-size, and total-budget
  assertions remain passing. Metadata is checked before requesting contents.

## Verification

```text
python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py
python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
python3 scripts/lint-harness-files_test.py
python3 scripts/lint-harness-files.py --all
python3 scripts/lint-spec-files_test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
git diff --check
```

## Results

- Both performance regressions were observed failing before their corrections.
- Context tests pass: 7 tests, including a real filtered partial clone. The
  prefetch fixture performs one upload-pack request and preserves refs and HEAD.
- Workflow contract tests pass: 30 tests. Harness lint tests pass: 19 tests.
  Specification lint tests pass: 36 tests. Full harness and specification lint
  pass. Catalog and documentation-coverage validation are recorded after this
  work-order addition below.
- Actual PR #3598 head context preparation uses 12 Git processes and takes
  1.313 seconds locally. The original used 1,514 processes and 91.83 seconds.
  Local timings do not prove hosted-runner completion.
- CI and review disposition remain pending on the updated PR #4147 head.
