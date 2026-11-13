---
id: "04-prove-end-to-end"
title: "Prove adoption end to end"
status: complete
wave: 4
depends_on:
  - "03-detach-and-files-roots"
plan: "plan.md"
requirements:
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-001
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-002
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-003
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-004
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-005
acceptance_criteria:
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-001.1
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-002.1
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-003.3
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-004.1
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-004.3
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-004.4
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-004.5
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-004.7
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-005.1
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-005.2
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-005.3
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-005.4
system_design:
  - ../../specs/tasks/system-design/agent-worktree-adoption.md
---

# Task 04: Prove adoption end to end

## Summary

Mock-agent scenarios create a worktree inside the task workspace, with and without reporting it.
Desktop and phone Playwright tests prove the user flow: the root appears, can be committed, opens
its root menu by right-click or long-press, can be removed, stays removed under detection, and returns on a report.

## In scope

- Mock-agent scenarios `agent-worktree-unreported` and `agent-worktree-reported`.
- `e2e/tests/task/agent-worktree-adoption.spec.ts` and
  `e2e/tests/task/mobile-agent-worktree-adoption.spec.ts`, with a shared helper.
- Public docs update through `/docs-maintainer` for the new tool input and Remove from task.

## Out of scope

- Containers project coverage; remote executors are excluded.

## Acceptance

- Both specs pass on their projects with the scenarios above.
- The phone spec shows no page-level horizontal overflow during removal.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./cmd/mock-agent -run 'TestAgentWorktree' -count=1)
(cd apps/web && pnpm exec oxlint -c oxlint.e2e-sleeps.config.mjs --deny-warnings e2e/tests/task/agent-worktree-adoption.spec.ts e2e/tests/task/mobile-agent-worktree-adoption.spec.ts)
(cd apps/web && pnpm e2e:run --host --project chromium tests/task/agent-worktree-adoption.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-agent-worktree-adoption.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/cmd/mock-agent/agent_worktree.go` (new), `scenarios.go`
- `apps/web/e2e/helpers/agent-worktree-adoption.ts` (new)
- `apps/web/e2e/tests/task/agent-worktree-adoption.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-agent-worktree-adoption.spec.ts` (new)
- `docs/public/**` pages that describe `add_branch_to_task_kandev` and task Changes

## Dependencies

Task 03.

## Risks

- Detection latency is bounded by the git poll interval; tests must wait on the rendered group,
  not on fixed sleeps.

## Parallelism

`sequential`

## Inputs

- Pattern: `cmd/mock-agent/live_repository_attachment.go` and
  `e2e/tests/task/live-repository-attachment.spec.ts`.

## Results

Added mock-agent `agent-worktree-unreported` and `agent-worktree-reported` scenarios, shared E2E helpers, and desktop plus mobile Playwright specs. Both specs pass on their projects. Public docs now describe `worktree_path`, passive detection, the root menu, and Remove from task.
