---
id: "03-update-public-docs"
title: "Document live add-branch compatibility"
status: completed
wave: 3
depends_on: ["02-return-materialized-paths"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/attach-workspace-sources.md"
---

# Task 03: Document live add-branch compatibility

## Acceptance

- Public MCP and coordination documentation distinguishes idle/restart-capable batch attachment
  from active-turn legacy add-branch.
- Documentation states sibling placement, unchanged agent/terminal CWD, no process restart, exact
  returned path, and clean Git status in the original repository.
- Public documentation validation passes.

## Verification

```bash
rtk rg -n 'add_branch_to_task_kandev|worktree_path|agent_cwd_changed' docs/public/automation-and-mcp.md docs/public/coordination.md docs/public/feature-status.md
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files Likely Touched

- `docs/public/automation-and-mcp.md`
- `docs/public/coordination.md`
- `docs/public/feature-status.md`

## Dependencies

- Task 02 finalizes the MCP response and tool wording.

## Parallelism

`sequential`. Public documentation must reflect the implemented response contract exactly.

## Inputs

- Spec **What**, **API surface**, **Scenarios**, and **Out of scope**.
- ADR-2026-07-27-legacy-add-branch-live-rescan.
- `/docs-maintainer` task-oriented public documentation guidance.

## Output Contract

Report the public pages updated, terminology changes, exact validation results, files changed,
risks, and blockers. Mark this task `done` and update its plan checkbox in the primary conversation.
