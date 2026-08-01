---
id: "04-publish-sdk-version"
title: "Publish and record the first immutable SDK version"
status: pending
wave: 2
depends_on: ["03-ci-release-and-docs"]
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 04: Publish and record the first immutable SDK version

- **Acceptance:** The initial SDK module PR is merged before any tag is created.
- **Acceptance:** `pluginsdk/v0.1.0` points to the reviewed merge commit and Go
  resolves both that tag and the commit-derived pseudo-version.
- **Acceptance:** The merge SHA, tag, pseudo-version, and SDK verification run are
  recorded in `plan.md` for all consumer tasks.
- **Verification:** `go list -m -json github.com/kdlbs/kandev/pluginsdk@v0.1.0`
- **Verification:** `go list -m -json github.com/kdlbs/kandev/pluginsdk@<merge-sha>`
- **Files likely touched:** no source files; `plan.md` verification results and
  Git tag `pluginsdk/v0.1.0`.
- **Dependencies:** Tasks 01–03 merged to Kandev main.
- **Parallelism:** sequential release checkpoint; all consumer migrations are blocked on it.
- **Inputs:** merged Kandev PR, SDK tag workflow, ADR-2026-08-01.
- **Risks:** Tag creation is an external publishing action and requires explicit
  user authorization at execution time; never tag a PR-head commit.
- **Output contract:** report immutable identifiers, GitHub workflow result,
  commands, blockers, and status updates.

## Results

Pending.
