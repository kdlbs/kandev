---
id: turn-changed-files-06
title: Summary history and authenticated historical reads
status: done
wave: 6
depends_on:
  - turn-changed-files-02
  - turn-changed-files-04
  - turn-changed-files-05
plan: plan.md
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-007
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-002.5
  - AC-TASKS-TURN-CHANGES-002.6
  - AC-TASKS-TURN-CHANGES-002.8
  - AC-TASKS-TURN-CHANGES-004.1
  - AC-TASKS-TURN-CHANGES-004.2
  - AC-TASKS-TURN-CHANGES-004.3
  - AC-TASKS-TURN-CHANGES-004.4
  - AC-TASKS-TURN-CHANGES-004.5
  - AC-TASKS-TURN-CHANGES-004.6
  - AC-TASKS-TURN-CHANGES-004.7
  - AC-TASKS-TURN-CHANGES-005.1
  - AC-TASKS-TURN-CHANGES-005.9
  - AC-TASKS-TURN-CHANGES-007.1
  - AC-TASKS-TURN-CHANGES-007.2
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Summary history and authenticated historical reads

## Summary

Publish compact revisioned summaries and expose lazy historical content through task-scoped application APIs.

## Scope and owned files

- New task changes service/controller/handler DTOs and route registration.
- Turn history, conversation hydration, boot projection, pagination, and event/gateway mappings.
- `session.turn.changes.updated` contract with monotonic revision and bounded root preview.
- Web changes API client/domain hook, HTTP types, session change-set state, turn actions, and WS handler.
- Authorization, relationship validation, summary/file pagination, and explicit content states.

## Exclusions

No browser-provided Git ref/path, executor relaunch for history, patch data in transcript WS, or transcript markup.

## Implementation acceptance

1. Reload/pagination/live events expose the same compact summary and final-turn anchor without losing newer revisions.
2. Authenticated lazy reads return exact retained file variants and reject cross-task/session/checkout IDs, unauthorized users, and arbitrary source inputs.
3. Expiry, pending, zero, unsupported, failed, and partial states remain distinct; history never falls back to live Git or current HEAD.

## Verification

New test families: `TurnChangeHistory`, `TurnChangeAPI`, `TurnChangeProjection`.
Prove no full patch or rendering blob enters normal conversation/history/WS payloads.
Page large catalogs and apply stale HTTP pages after live events to verify revision fencing.

```bash
cd apps/backend
go test -trimpath ./internal/task/service ./internal/task/handlers ./internal/gateway/websocket ./internal/task/repository/sqlite -run 'TurnChangeHistory|TurnChangeAPI|TurnChangeProjection|PostgresTurnChangeHistory' -count=1
```

```bash
cd apps/web
pnpm exec vitest run lib/api/domains/turn-changes-api.test.ts lib/state/slices/session/turn-changes-actions.test.ts lib/ws/handlers/turn-changes.test.ts hooks/domains/session/use-turn-changes.test.ts
```

Run database-dependent PostgreSQL cases with a real test DSN. Include access revocation and uncompressed response-size bounds.

## Dependencies and risks

Depends on 02, 04, and 05. Final assistant output can arrive after READY; revise only the anchor, never endpoints.
Components must consume domain hooks/state, not issue their own fetches. Normal workspace recovery must not create a historical replacement checkout.

## Results

Implemented revisioned turn history, paged summaries and files, authenticated retained-content reads, WebSocket updates, and transcript projection. Validation passed in `go test -trimpath ./...`, `pnpm test` (2,687 files, 23,689 tests passed, 4 skipped), and the full set of focused API, state, hook, service, repository, handler, and event tests. PostgreSQL-specific execution remains unverified because `KANDEV_TEST_POSTGRES_DSN` is unset.

### Review follow-up (2026-10-08)

History pagination now maps the API cursor and hydrates summaries for visible transcript turns; exact change-set lookup covers targets outside the initial history page. The typed projection hides uncaptured/unsupported and ready-zero turns, distinguishes preparing, unavailable, partial, and expired states, and only inserts a fallback in a genuinely terminal transcript row after older pages load. Focused history, projection, and transcript tests passed.
