---
id: "01-stop-deleted-session-reads"
title: "Stop deleted session reads"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-DELETED-TASK-CLIENT-STATE-001
acceptance_criteria:
  - AC-UI-DELETED-TASK-CLIENT-STATE-001.1
  - AC-UI-DELETED-TASK-CLIENT-STATE-001.2
  - AC-UI-DELETED-TASK-CLIENT-STATE-001.3
  - AC-UI-DELETED-TASK-CLIENT-STATE-001.4
  - AC-UI-DELETED-TASK-CLIENT-STATE-001.5
system_design:
  - ../../specs/ui/system-design/deleted-task-client-state.md
---

# Task 01: Stop Deleted Session Reads

## Summary

Retire reads owned by deleted sessions and make missing-session Git errors
non-retryable across the WebSocket boundary.

## In scope

- Task deletion cleanup, session read lifetime, Git read responses and tests.
- Existing desktop and mobile consumers of shared session read state.

## Out of scope

- Archived session behavior, unrelated Git operations, and visual changes.

## Acceptance

- Deletion cancels scheduled and in-flight reads without reviving them or
  touching a live session, including when they share an environment.
- Missing sessions return `NOT_FOUND` for both Git reads; transient failures
  still follow existing recovery behavior.
- Focused regression tests, typecheck, and browser deletion scenario pass.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/handlers -run 'TestWs(GitCommits|CumulativeDiff|GitReads)')
(cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/session/use-session-git-reads.test.ts lib/ws/handlers/tasks.deleted.test.ts lib/state/session-read-coordinator.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=7168 pnpm run typecheck)
(cd apps && ../scripts/run-quiet e2e --summary -- pnpm --dir web e2e:run --host --shards 1 --project chromium tests/task/deleted-task-client-state.spec.ts -- --retries=0)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/domains/session/use-session-git-reads.test.ts lib/state/session-read-coordinator.ts lib/ws/handlers/tasks.ts e2e/tests/task/deleted-task-client-state.spec.ts)
bash scripts/lint-go-changed apps/backend/internal/agent/handlers/git_handlers.go apps/backend/internal/agent/handlers/git_handlers_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/web/lib/state/session-read-coordinator.ts`
- `apps/web/hooks/domains/session/use-session-read.ts`
- `apps/web/lib/ws/handlers/tasks.ts`
- `apps/web/hooks/domains/session/use-session-git-reads.test.ts`
- `apps/web/e2e/tests/task/deleted-task-client-state.spec.ts`
- `apps/backend/internal/agent/handlers/git_handlers.go`
- `apps/backend/internal/agent/handlers/git_handlers_test.go`

## Dependencies

None.

## Risks

Retire by session, not shared environment; distinguish typed absence from
transient errors and prevent stale responses from changing replacement state.

## Parallelism

Sequential.

## Inputs

Deleted task client state requirement and design; existing read coordinator
and Git handler tests.

## Results

All commands passed. The frontend suite passed 36 tests; the Chromium spec
passed two tests with retries disabled on the rebuilt follow-up. An earlier
run failed with an HTTP 401 while creating fixture data before the test body;
the subsequent isolated run passed. The initial default-heap typecheck ran out
of memory; the listed 7 GiB heap command passed.

Review follow-up added a red/green remount regression, retained the deletion
fence until scope reset, and made the browser assertion wait for client state
eviction before its negative observation interval.

After merging the latest base, the focused Go tests and all 54 selected
frontend tests passed. A rebuilt Chromium run of this spec alongside the full
pseudo-locale spec passed all 25 tests with retries disabled.
