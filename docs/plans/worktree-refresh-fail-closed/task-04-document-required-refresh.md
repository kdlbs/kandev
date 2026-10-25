---
id: "04-document-required-refresh"
title: "Document required repository refresh"
status: done
wave: 4
depends_on:
  - "03-project-refresh-launch-failures"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.1
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.2
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 04: Document Required Repository Refresh

## Summary

Document the SSH configuration boundary, required-refresh failure behavior,
offline opt-out, and safe retry procedure in the public Git guidance.

## In scope

- Update executor documentation to state where host and remote executors read
  Git and SSH credentials.
- Update Git operations documentation to explain that pull-before-worktree
  stops launch when refresh fails.
- Document `gh config set git_protocol ssh --host github.com` for a host that
  uses executor-inherited GitHub access.
- State that Kandev must restart after host GitHub protocol changes so managed
  checkout origins are reconciled before the next task launch.
- Explain how to disable pull-before-worktree for an intentional offline local
  workflow.
- Add credential-safe troubleshooting steps for authentication and network
  errors.

## Out of scope

- New settings, screenshots, or UI copy.
- Provider-specific SSH installation guides beyond the existing supported
  configuration surface.
- Documentation for setup-script failure policy.

## Acceptance

- Public docs distinguish workspace automation identity from task Git
  credential policy and selected Git transport.
- Public docs state that a required refresh failure stops launch and does not
  use a stale local fallback.
- Commands, internal links, and headings pass the public documentation
  validators.

## Verification

```bash
# From the repository root:
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

## Files likely touched

- `docs/public/executors.md`
- `docs/public/git-operations.md`
- `docs/plans/worktree-refresh-fail-closed/plan.md`
- `docs/plans/worktree-refresh-fail-closed/task-04-document-required-refresh.md`

## Dependencies

- Task 03 verifies the exact launch and retry behavior that the public docs
  describe.

## Risks

- Documentation can imply that a named GitHub CLI automation connection makes
  SSH available to every executor. State the separate task policy and runtime
  boundary explicitly.
- Remote executors need their own SSH key and known-host configuration. Do not
  describe host configuration as portable to Docker or SSH executors.

## Parallelism

`sequential`

## Inputs

- Verified behavior from Tasks 01 through 03.
- `ADR-2026-07-27-task-git-credential-policy`.
- `docs/public/executors.md` and `docs/public/git-operations.md`.

## Results

- Updated [Executors](../../public/executors.md) with workspace automation versus
  task Git credential boundaries, executor-local SSH requirements, host GitHub
  SSH configuration, and restart guidance.
- Updated [Git Operations](../../public/git-operations.md) with required-refresh
  failure behavior, the offline opt-out, and credential-safe troubleshooting.
- Verified with public-doc unit and live validators, specification lint, and
  `git diff --check`.
