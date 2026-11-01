---
id: "03-split-unrelated-fixes"
title: "Split unrelated fixes out of #3660"
status: pending
wave: 3
depends_on:
  - "02-deletion-of-unprovable-usage"
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-005
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-005.5
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 03: Split unrelated fixes out of #3660

## Summary

Move fixes that have nothing to do with usage history into their own PRs, so #3660 is one logical change. Use normal commits and merges only.

## In scope

Create one branch from `origin/main` and one PR per row, using `git cherry-pick -x`. Each PR needs a Conventional Commit title and its own tests, which the commits already contain.

| PR | Commit | Main file |
| --- | --- | --- |
| Tunnel listener close | `3ae93df2d39` | `gateway/websocket/port_tunnel.go` |
| Remote helper race | `546d2fff1e5` | `lifecycle/agentctl_resolver.go` |
| Readiness ordering | `da2860d4ae3` | `websocket/task_notifications.go` |
| Git refresh ownership | `ec81296c4e3` | `lib/ws/handlers/git-status.ts` |
| Provider commit loading | `19af4666a08` | `hooks/domains/github/use-pr-commits.ts` |
| Sidebar scroll | `848e3e1051f` | `lib/sidebar/task-navigation.ts` |
| E2E shard timeout | `8d978e9254d` | `.github/workflows/e2e-tests.yml` |

Three files came in larger commits. Inspect each hunk with `git log -p`, then move it only if it stands alone: `manager_startup.go` (`7fa12b404e8`), `changes-panel-file-row.tsx` (`6be930974f3`), and `scripts/lint-go-changed` (`8632d5dcf84`).

After the user merges those PRs, merge `origin/main` into the #3660 branch and confirm that the files leave the diff.

## Out of scope

- Reverting the fixes from #3660 before their replacements merge, or force-pushing.
- Other E2E stabilization commits. List remaining non-usage files in Results instead.

## Acceptance

- Each row is an open or merged PR, or has a stated reason for staying in #3660.
- After merging `main`, `git diff --name-only origin/main...HEAD` shows none of the moved files.
- Results list every remaining changed production file that isn't usage history, with a reason.

## Verification

```bash
git fetch origin main
git diff --name-only origin/main...HEAD -- apps/backend/internal/gateway/websocket/port_tunnel.go apps/backend/internal/agent/runtime/lifecycle/agentctl_resolver.go apps/backend/internal/gateway/websocket/task_notifications.go apps/web/lib/ws/handlers/git-status.ts apps/web/hooks/domains/github/use-pr-commits.ts apps/web/lib/sidebar/task-navigation.ts .github/workflows/e2e-tests.yml
python3 scripts/list-docs.py validate
```

## Files likely touched

The files in the table, on their own branches, and `docs/plans/usage-history-merge-readiness/task-03-split-unrelated-fixes.md`.

## Dependencies

Task 02. This needs authorization to push and open PRs. If the implementation request doesn't grant it, STOP and report.

## Risks

- `19af4666a08` also changes `docs/plans/remote-contribution-head-drift/` and a spec. Keep those with the fix.
- A cherry-pick may conflict with newer `main`. Skip that row and report it if it can't be resolved in a few lines.
- Merging depends on the user, so this work order can stay `blocked` while Task 04 waits.

## Parallelism

`sequential`

## Inputs

- The commit table above.
- `.agents/skills/commit/SKILL.md` and `.agents/skills/pr/SKILL.md`.

## Results

Pending.
