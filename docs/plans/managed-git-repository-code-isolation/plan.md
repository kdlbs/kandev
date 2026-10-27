---
status: draft
created: 2026-10-25
requirements:
  - REQ-AGENTS-GITISOLATION-001
  - REQ-AGENTS-GITISOLATION-002
  - REQ-AGENTS-GITISOLATION-003
system_design:
  - ../../specs/agents/system-design/managed-git-repository-code-isolation.md
---

# Managed Git repository-code isolation

## Overview

Managed Git (backend and agentctl, outside the agent's process) currently runs
hooks, a file-system monitor, filters, text converters, and merge drivers that a
repository supplies. Make the default run none of them, keep Git LFS working, and
add a per-repository "Run Git hooks" opt-in. The default hardening and the opt-in
ship as one change so that no release disables hooks without a way to keep them.

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
- Delivery: ships in the follow-up PR that builds on kdlbs/kandev#4769.
