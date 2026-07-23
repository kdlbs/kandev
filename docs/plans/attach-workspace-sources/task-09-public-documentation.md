---
id: "09-public-documentation"
title: "Public documentation"
status: done
wave: 5
depends_on: ["05-remote-materialization", "07-files-panel-surface"]
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 09: Public Documentation

## Acceptance

- Public task, coordination, executor, developer-tools, MCP, and feature-status docs explain the Files
  action, supported source kinds, idle-only mutation, executor behavior, Local/Worktree-only folder
  support, and legacy/new MCP tools.
- Statements that multi-repository tasks and post-create additions are Worktree-only are removed or
  replaced only where implementation now proves broader support.
- Security guidance explains that arbitrary folders remain live host grants and are unavailable to
  container/remote tasks.

## Verification

```bash
rtk rg -n "Worktree-only|worktree executor|add_branch_to_task_kandev|multi-repository" docs/public
```

## Files likely touched

- `docs/public/tasks-and-workflows.md`
- `docs/public/coordination.md`
- `docs/public/executors.md`
- `docs/public/developer-tools.md`
- `docs/public/automation-and-mcp.md`
- `docs/public/security.md`
- `docs/public/feature-status.md`

## Dependencies

Tasks 05 and 07.

## Inputs

- Final implemented UI/API/runtime behavior.
- Spec and ADR.

## Output contract

Summary, files changed, searches/checks run, blockers, risks, divergence, and task/plan status updates.
