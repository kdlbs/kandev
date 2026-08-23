---
id: "01-trusted-pr-context"
title: "Trusted PR context"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/pr-walkthrough/spec.md"
---

# Task 01: Trusted PR Context

Replace dynamic PR-head file reads with a bounded filesystem context. The
helper reads immutable Git objects and never checks out the PR head.

- **Acceptance:**
  `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-context` requires valid
  base and head SHAs, resolves their merge base, and uses the triple-dot
  changed-file set.
- **Acceptance:** The helper writes a deterministic manifest and materializes
  only regular UTF-8 files from the exact head commit.
- **Acceptance:** Each file is limited to 512 KiB. All materialized files share
  an 8 MiB total budget.
- **Acceptance:** The manifest records deleted, non-regular, binary, oversized,
  unsafe, and budget-excluded paths without following symlinks.
- **Acceptance:** The old dynamic read helper and its test are removed after
  equivalent security cases pass through the new context test.
- **Verification:**

  ```text
  python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py
  git diff --check
  ```

- **Files likely touched:**
  `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-context`,
  `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py`,
  `scripts/pr-walkthrough-read-file`,
  `scripts/pr-walkthrough-read-file.test.py`.
- **Dependencies:** None.
- **Parallelism:** parallel-safe with Task 02. The tasks have disjoint helper,
  test, and skill files.
- **Inputs:** The spec permissions and scenarios, the filesystem runner ADR,
  and the validation rules in `scripts/pr-walkthrough-read-file`.
- **Output contract:** Report the manifest schema, files changed, exact test
  results, security boundaries, blockers, and task or plan status changes.

## Results

- Added `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-context` with
  merge-base resolution, triple-dot changed-file discovery, immutable
  Git-object reads, and deterministic output.
- Materialized files are regular UTF-8 blobs under the 512 KiB per-file and
  8 MiB total limits. The manifest records deleted, non-regular, binary,
  oversized, unsafe, and budget-excluded entries.
- The helper and its test are self-contained inside the skill directory. Task
  03 removed the old dynamic reader from the workflow and repository.
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py`
  — passed (4 tests).
- `git diff --check` — passed.
