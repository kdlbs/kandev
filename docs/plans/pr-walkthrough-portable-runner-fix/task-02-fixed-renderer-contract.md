---
id: "02-fixed-renderer-contract"
title: "Fixed renderer contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/pr-walkthrough/spec.md"
---

# Task 02: Fixed Renderer Contract

Give every managed agent one fixed draft path and one fixed renderer command.
Keep trusted identity binding and atomic final outputs.

- **Acceptance:**
  `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-render` reads only
  `.pr-walkthrough/draft.json` and requires no model-controlled command
  arguments or standard input.
- **Acceptance:** The renderer resolves `references/build.py` and
  `references/shell.html` relative to the copied skill directory. It does not
  depend on a repository-root helper or an OpenCode-named staging directory.
- **Acceptance:** The renderer binds trusted PR identity, removes link
  overrides, and writes the final JSON and HTML to their existing fixed paths.
- **Acceptance:** A malformed or missing draft returns a clear error and leaves
  no partial final outputs.
- **Acceptance:** The `pr-walkthrough` skill tells a managed CI agent to use the
  host-provided draft path and renderer command until validation succeeds.
- **Verification:**

  ```text
  python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py
  cd .agents/skills/pr-walkthrough/references && python3 -m unittest test_build
  cd ../../../.. && python3 .github/scripts/lint-harness-files.py .agents/skills/pr-walkthrough
  git diff --check
  ```

- **Files likely touched:**
  `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-render`,
  `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py`,
  `.agents/skills/pr-walkthrough/SKILL.md`.
- **Dependencies:** None.
- **Parallelism:** parallel-safe with Task 01. The tasks have disjoint helper,
  test, and skill files.
- **Inputs:** The spec generation contract, the filesystem runner ADR, and the
  current trusted metadata tests.
- **Output contract:** Report files changed, exact test and harness results,
  output paths, blockers, and task or plan status changes.

## Results

- Added the fixed renderer and its test under the self-contained skill bundle.
  The renderer reads `.pr-walkthrough/draft.json`, ignores standard input and
  model-controlled arguments, and resolves `references/` beside its script.
- Trusted PR identity binding, URL override removal, atomic output, and clear
  draft failures remain covered.
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py`
  — passed (4 tests).
- `python3 -m unittest test_build` from
  `.agents/skills/pr-walkthrough/references` — passed (59 tests).
- `python3 .github/scripts/lint-harness-files.py .agents/skills/pr-walkthrough`
  — passed.
- `git diff --check` — passed.
