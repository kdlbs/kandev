---
id: "04-public-url-and-shell-presentation"
title: "Public URL and shell presentation"
status: done
wave: 3
depends_on: ["03-prompt-driven-workflow"]
plan: "plan.md"
spec: "../../specs/ui/requirements/pr-walkthrough.md"
---

# Task 04: Public URL and Shell Presentation

Shorten hosted walkthrough links and align the generated page with the
website and documentation shell.

- **Acceptance:** The publication job and PR-body helper use the first 12
  lowercase hexadecimal characters of the trusted full head SHA.
- **Acceptance:** The generated shell uses the website brand image, favicon,
  and documentation dark-gray palette.
- **Acceptance:** Focused helper, workflow-contract, renderer, and responsive
  page checks pass.
- **Verification:**

  ```text
  python3 scripts/pr-walkthrough-pr-body.test.py
  python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
  cd .agents/skills/pr-walkthrough/references && python3 -m unittest test_build
  git diff --check
  ```

## Results

- The implementation derives a 12-character public SHA prefix from the full
  event SHA and retains full-SHA validation for trusted workflow identity.
- The shell uses the website brand image and favicon links and the docs dark
  tokens for its shell surfaces, text, and borders.
- `python3 scripts/pr-walkthrough-pr-body.test.py` passed (8 tests).
- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` passed
  (20 tests).
- `python3 -m unittest test_build` passed (59 tests).
- Context, renderer, harness, action-pinning, and actionlint checks passed.
- Google Chrome captured the generated page at 390x844 and 1440x1000. Both
  views showed the brand mark and no topbar overflow.
