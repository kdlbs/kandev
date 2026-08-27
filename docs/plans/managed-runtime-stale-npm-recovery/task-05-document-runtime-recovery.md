---
id: "05-document-runtime-recovery"
title: "Document npm runtime recovery"
status: done
wave: 4
depends_on: ["03-present-npm-recovery"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 05: Document npm runtime recovery

Update public troubleshooting guidance after the behavior and labels are
stable.

- **Acceptance:** Public docs explain the automatic same-version online retry
  and its one-attempt limit.
- **Acceptance:** The troubleshooting path explains how to inspect the
  configured npm registry when the focused error remains.
- **Acceptance:** The docs state that global npm cache cleanup is not the normal
  recovery path and do not instruct users to run `npm cache clean --force`.
- **Acceptance:** Commands, labels, and behavior match the implementation.
- **Verification:** Run:

  ```bash
  node --test scripts/validate-public-docs.test.mjs
  node scripts/validate-public-docs.mjs
  ```

- **Files likely touched:** `docs/public/agents-and-profiles.md`.
- **Dependencies:** Task 03.
- **Parallelism:** Can run in parallel with Task 04 after Task 03.
- **Inputs:** Final retry behavior and UI labels from Tasks 01 through 03.
- **Output contract:** Report files changed, validation commands and results,
  verified links and commands, and synchronized task and plan status.
