---
id: "14-ux-terminology-documentation"
title: "UX terminology documentation"
status: done
wave: 8
depends_on:
  - "11-shared-local-remote-selector"
  - "12-unified-files-workspace-actions"
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 14: UX terminology documentation

## Acceptance

- Public docs direct users through **Files → Workspace actions → Add sources** and describe the
  shared **Local / Remote** modes rather than the removed three-card taxonomy.
- Documentation preserves executor capability, branch, atomicity, and folder limitations without
  implying a backend contract change.
- Feature-status and cross-linked coordination/developer-tool descriptions use the same concise
  terminology.

## Verification

```bash
rtk rg -n "Files → Add sources|saved workspace repository, a local Git repository, a remote Git repository" \
  docs/public
make docs-check
rtk git diff --check
```

## Files likely touched

- `docs/public/feature-status.md`
- `docs/public/tasks-and-workflows.md`
- `docs/public/coordination.md`
- `docs/public/developer-tools.md`

## Dependencies

Tasks 11 and 12.

## Inputs

- Spec: **What**, **Scenarios**, and unchanged executor/failure contracts.
- Plan: UX delta overview and Files/source-selector sections.

## Output contract

- Update only this task file to `in_progress` when starting and `done` after acceptance and
  verification pass.
- Return a compact handoff capsule with files changed, terminology decisions, command results,
  risks, and any uncertainty.
- Do not update `plan.md`; the planner serializes shared-plan status.
