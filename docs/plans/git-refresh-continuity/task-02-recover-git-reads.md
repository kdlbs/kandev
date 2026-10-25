---
id: "02-recover-git-reads"
title: "Recover slow session Git reads automatically"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-REFRESH-CONTINUITY-001
acceptance_criteria:
  - AC-PLATFORM-GIT-REFRESH-CONTINUITY-001.4
system_design:
  - ../../specs/platform/system-design/git-refresh-continuity.md
---

# Task 02: Recover Slow Session Git Reads

## Summary

Resolve [issue #4333](https://github.com/kdlbs/kandev/issues/4333) when a Git response exceeds the default WebSocket timeout or a temporary refresh fails.
Reuse shared reads to recover automatically while keeping the last successful result visible.

## Scope

Session commits and cumulative diffs get a 30-second timeout and five-second retries for timeouts and temporary server errors.
Preserve cached results through recovery; show initial loading before a successful commits result.
Permanent errors and terminal sessions stop. Existing coordinator ownership and lifecycle cancellation remain authoritative.
Backend publication, continuous polling, mutation behavior, and historical cache redesign are excluded.

## Acceptance

1. Responses arriving at 6.5 and 29 seconds publish normally; unanswered reads retry automatically without duplicate requests across consumers.
2. Failed refreshes preserve commits and diff data until replacement, including an authoritative empty response. Scope retirement cancels retries.
3. The initial Changes view shows loading and recovers through a new request without a manual action or false clean message.

## ASCII UI preview

UI-03 from the [plan](plan.md#ascii-ui-preview-1), first read recovering on desktop and phone:

```text
| Changes                         |
|       Loading changes...        |
```

With prior data, retain its rows or confirmed empty state. Keep the existing desktop panel and phone tab hierarchy.
This is the structure required by `AC-PLATFORM-GIT-REFRESH-CONTINUITY-001.4`; spacing is illustrative.

## Verification

Run from the repository root using the pinned toolchain:

```bash
mise exec -- pnpm --dir apps/web exec vitest run hooks/domains/session/use-session-git-reads.test.ts hooks/domains/session/use-session-commits.test.ts hooks/domains/session/use-cumulative-diff.test.ts hooks/domains/session/use-session-git-pending.test.tsx lib/state/session-read-coordinator.test.ts lib/ws/client.test.ts components/task/changes-panel-body-context.test.tsx components/task/changes-panel-pr-files.test.tsx components/task/mobile/mobile-changes-panel.test.tsx
(cd apps && mise exec -- env NODE_OPTIONS='--dns-result-order=ipv4first --max-old-space-size=8192' pnpm --filter @kandev/web typecheck)
mise exec -- pnpm --dir apps/web build:e2e
mise exec -- scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/git/git-read-recovery.spec.ts -- --retries=0
mise exec -- scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/git/git-changes-panel.spec.ts -- --retries=0 --grep='multiple commits persist after page refresh|cumulative diff shows all changes from multiple commits'
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run strict ESLint and Prettier on every changed frontend file. Keep capture assets in ignored `.pr-assets` and publish them with the PR.

## Files and dependencies

- Session hooks, `session-git-read.ts`, and transport-boundary regression tests.
- Changes body/data/props and its existing rendered regression.
- `e2e/tests/git/git-read-recovery.spec.ts`.
- Existing `SessionRead` coordinator and WebSocket error types; no new dependency.

## Results

Local implementation is complete: 103 focused tests in nine files, the initial-recovery browser test, existing commit-persistence and cumulative-diff browser checks, typecheck, build, strict ESLint, formatting, and localization/wait guards passed.
The regression fails before recovery because no new request arrives after a temporary server error.
The extended body regression also fails before the initial-loading gate because it shows a false clean message.
PR CI and automated review are external delivery checks and are not claimed complete by these local results.
No public documentation change is needed: this repairs automatic refresh behavior without changing a command, setting, or user workflow.
