---
id: "05-public-documentation"
title: "Document repository-set bases"
status: done
wave: 5
depends_on:
  - "04-saved-base-e2e"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-SETS-002
  - REQ-WORKSPACES-REPOSITORY-SETS-003
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-SETS-002.1
  - AC-WORKSPACES-REPOSITORY-SETS-002.3
  - AC-WORKSPACES-REPOSITORY-SETS-002.6
  - AC-WORKSPACES-REPOSITORY-SETS-003.3
  - AC-WORKSPACES-REPOSITORY-SETS-003.4
  - AC-WORKSPACES-REPOSITORY-SETS-003.7
system_design:
  - ../../specs/workspaces/system-design/repository-sets.md
---

# Task 05: Document Repository-Set Bases

## Summary

Update the public repository-set guide after the verified behavior exists. Use
the same terms as the settings and task-create interfaces.

## In scope

- Explain saved bases, `Task default`, and the repository-default context.
- Explain apply-time copy and existing-row preservation.
- Explain unavailable saved bases and user repair.
- Update API payload examples for ordered member objects and compatibility IDs.

## Out of scope

- General Git branch-policy documentation.
- Release notes and marketing copy.

## Acceptance

- The guide no longer states that sets hold repositories only.
- The guide distinguishes task defaults, repository defaults, saved bases, and
  local checkout.
- The API example includes the new ordered member field and compatibility note.

## Verification

Run these commands from the repository root.

```bash
git diff --check -- docs/public/tasks-and-workflows.md
rg -n "repository set|saved base|Task default|repository_ids" docs/public/tasks-and-workflows.md
```

## Files likely touched

- `docs/public/tasks-and-workflows.md`

## Dependencies

- Task 04 proves the final visible behavior.

## Risks

- The guide can describe planned UI instead of the implemented UI if this task
  starts before E2E completion.

## Parallelism

`sequential`

## Inputs

- `REQ-WORKSPACES-REPOSITORY-SETS-002`
- `REQ-WORKSPACES-REPOSITORY-SETS-003`
- Verified UI labels from Task 04

## Results

- Updated the public task guide with saved bases, Task default and repository
  default context, local checkout separation, unavailable-branch recovery, and
  ordered API member objects with `repository_ids` compatibility.
- Verification: the public-docs validator passed 61 tests, all 41 published
  pages validated, and the task-specific diff and search checks pass.
