---
id: "03-update-refresh-documentation"
title: "Update Public Refresh Documentation"
status: completed
wave: 3
depends_on:
  - "02-prove-local-fallback-launch"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.2
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 03: Update Public Refresh Documentation

## Summary

Update public Git and executor documentation for local-first worktree refresh.
Keep remote-only materialization limits clear.

## In scope

- Describe pull-before-worktree as best effort when a local base exists.
- Describe credential-safe warnings for local fallback.
- Describe strict failure when no local or remote base is available.
- Remove guidance that users must disable refresh for normal local branches.
- Keep empty-remote and remote executor guidance accurate.

## Out of scope

- New public pages or navigation entries.
- UI copy or localization changes.
- Release notes.

## Acceptance

- Public docs do not claim that all refresh errors stop host worktrees.
- Public docs state when remote materialization remains required.
- Public-doc validators and link checks pass.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs/public
```

## Files likely touched

- `docs/public/git-operations.md`
- `docs/public/executors.md`

## Dependencies

- Task 02 must be complete.

## Risks

- Broad wording can imply that Docker, SSH, or Sprites can use host-only refs.

## Parallelism

`sequential`

## Inputs

- Worktree Base Refresh requirements and system design.
- ADR `2026-08-31-local-worktree-refresh-best-effort`.
- Existing Git operations and executor documentation.

## Results

- Git operations now describe best-effort host refresh with credential-safe
  local fallback warnings.
- Executor guidance distinguishes host local bases from strict remote-only
  refresh and checkout requirements.
- Public documentation tests, link validation, and whitespace checks passed.
