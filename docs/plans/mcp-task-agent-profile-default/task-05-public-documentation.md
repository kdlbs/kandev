---
id: "05-public-documentation"
title: "Public documentation"
status: done
wave: 3
depends_on: ["02-mcp-profile-resolution", "03-responsive-task-actions-setting"]
plan: "plan.md"
spec: "../../specs/tasks/mcp-task-agent-profile-default/spec.md"
---

# Task 05: Public Documentation

## Acceptance

- Public task documentation explains where to select the MCP-created task profile policy, its compatibility default, and the behavior of each option.
- Documentation states that explicit `agent_profile_id` overrides the preference, workspace-default mode still honors workflow step/default profiles first, and creation fails when neither workflow nor target workspace supplies a profile.

## Verification

```bash
rg -n "MCP-created|Current task profile|Workspace default profile|agent_profile_id" docs/public/tasks-and-workflows.md docs/public/feature-status.md
```

## Files Likely Touched

- `docs/public/tasks-and-workflows.md`
- `docs/public/feature-status.md`

## Dependencies

- `02-mcp-profile-resolution`
- `03-responsive-task-actions-setting`

## Inputs

- Spec: What, Failure modes, Scenarios, and Out of scope.
- Plan: Public Documentation.
- Follow `/docs-maintainer`; document observable behavior and exact Settings navigation without implementation details.

## Output Contract

Report documentation sections updated, terminology used, verification run, files touched, blockers, and any behavior that remained undocumented. Set this task to `done` and update its plan checkbox after review.
