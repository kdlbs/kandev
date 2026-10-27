---
status: draft
created: 2026-10-25
requirements:
  - REQ-AGENTS-GITISOLATION-001
  - REQ-AGENTS-GITISOLATION-002
  - REQ-AGENTS-GITISOLATION-003
  - REQ-AGENTS-GITISOLATION-004
system_design:
  - ../../specs/agents/system-design/managed-git-repository-code-isolation.md
  - ../../specs/agents/system-design/managed-git-isolation-rollout.md
---

# Managed Git repository-code isolation

## Overview

Managed Git (backend and agentctl, outside the agent's process) currently runs
hooks, a file-system monitor, filters, text converters, and merge drivers that a
repository supplies. Make the default run none of them, keep Git LFS working, and
add a per-repository "Run Git hooks" opt-in. The default hardening and the opt-in
ship as one change so that no release disables hooks without a way to keep them. The whole behavior ships behind `features.managedGitIsolation` (off in `prod`).

## Technical approach

One policy seam in `subproc.startManagedGit` (the point where the final `Dir`,
`Env` and context are known; `NewGitCommand` stays the only construction site)
applies the always-on pairs, driver neutralization, and the hooks path; the
`gitinit` helper applies the same policy inside the helper through
`subproc.ManagedGitArgs`. The hooks decision rides on the command context. A new
repository column feeds it in the backend directly and reaches agentctl through
the instance create request, the same route as `base_branches`. The
value is fixed when an agentctl instance is created; there is no runtime route.
Git LFS uploads on push use a Kandev-owned `pre-push` hooks
directory. The settings switch and public guide follow.

## Delivery order

1. [Task 01: Policy seam and default isolation](task-01-policy-seam-and-default-isolation.md)
2. [Task 02: Hooks setting, persistence and propagation](task-02-hooks-setting-persistence-and-propagation.md) (after 01)
3. [Task 03: Settings switch and public guide](task-03-settings-switch-and-public-guide.md) (after 02)
4. [Task 04: Rollout flag](task-04-rollout-flag.md) (after 03; its flag-off tests are written first and gate tasks 01 to 03 behind the mode)

## ASCII UI preview

See `UI-01` in the [system design](../../specs/agents/system-design/managed-git-repository-code-isolation.md#settings-control).
Only task 03 changes rendered UI.

## Risks

- Driver enumeration adds one process per filter-capable managed command; measure
  on the status poll and record the number in task 01's results.
- Tests that assert exact `git` argument vectors elsewhere in the tree will need
  the policy prefix; update them, never loosen them.
- The `raw_git_audit_test.go` guard must keep passing; the policy must not add a
  second construction site.

## Verification strategy

- `cd apps/backend && make test` for touched packages, plus the marker tests in
  task 01 and 02, and `bash scripts/lint-go-changed <changed-go-files>`.
- `cd apps/web && pnpm run typecheck && pnpm run i18n:check`, targeted vitest, and the
  Playwright repository-settings spec on desktop and mobile projects.
- `python3 scripts/lint-spec-files.py --all` and `python3 scripts/list-docs.py validate`.
- Playwright "Run Git hooks" specs run with the flag on through `KANDEV_FEATURES_MANAGED_GIT_ISOLATION=true` set by those specs only (the `e2e` profile default is off), desktop and mobile.
- Delivery: one pull request on `main`; commits are organized by task, not by review round.
