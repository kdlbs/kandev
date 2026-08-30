---
id: "01-walkthrough-skill-renderer"
title: "Walkthrough skill renderer"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/pr-walkthrough.md"
---

# Task 01: Walkthrough skill renderer

Adapt the final upstream `pr-walkthrough` skill into Kandev's `.agents/skills`
layout. Keep the JSON-driven renderer, fixed HTML shell, example data, and
standard-library tests. Add the CI output-mode instructions without changing
the skill's boundary from explanation to code review.

- **Acceptance:** The skill documents both normal generation and the exact CI
  JSON output contract, with Kandev-compatible paths and no hosting steps.
- **Acceptance:** The renderer produces a non-empty HTML file from the example
  JSON and rejects malformed required fields, invalid edges, invalid risk data,
  and unreplaced template tokens.
- **Acceptance:** Code and agent-controlled Markdown are escaped or sanitized
  by the generated page before insertion into HTML.
- **Verification:**

  ```text
  cd .agents/skills/pr-walkthrough/references && python3 -m unittest test_build
  python3 .github/scripts/lint-harness-files.py .agents/skills/pr-walkthrough
  git diff --check
  ```

- **Files likely touched:**
  `.agents/skills/pr-walkthrough/SKILL.md`,
  `.agents/skills/pr-walkthrough/references/build.py`,
  `.agents/skills/pr-walkthrough/references/example.json`,
  `.agents/skills/pr-walkthrough/references/shell.html`,
  `.agents/skills/pr-walkthrough/references/test_build.py`.
- **Dependencies:** None.
- **Parallelism:** sequential, because the workflow consumes this skill.
- **Inputs:** The spec's generation contract, permissions, failure modes, and
  scenarios; the final upstream PR walkthrough skill and renderer.
- **Output contract:** Report changed files, unit/lint results, generated sample
  artifact path, and any renderer or dependency risks. Mark the task done after
  implementation and verification.

## Results

Implemented the adapted upstream skill, fixed renderer shell, example JSON,
and standard-library tests under `.agents/skills/pr-walkthrough/`. Added the
exact CI marker-block output mode, kept hosting outside the skill, and aligned
the shell with the landing site's Figtree, Geist Mono, indigo, dark-surface,
compact-radius, and mobile-navigation language.

Verification: 56 renderer tests passed; the example rendered a non-empty
1,161-line HTML file; harness lint passed for the new skill directory.
