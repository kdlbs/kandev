---
id: "07-run-recorded-events"
title: "Show recorded runs without a reload"
status: done
wave: 4
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-RUNS-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-RUNS-001.23
system_design:
  - ../../specs/office/system-design/automation-run-recorded-events.md
---

# Task 07: Show recorded runs without a reload

## Summary

The Automations page and an automation's history page poll only while a run
is open, so a run recorded by the GitHub poller while nothing was open stayed
hidden until a reload. Publish a workspace-scoped `automation.run.created`
event when a run row is committed, and refresh those pages when it arrives.

## In scope

- The store's run-recorded observer and its calls on every insert path.
- The service publisher, the gateway forwarder, and the action constant.
- The web handler, the channel, `useAutomationRunRecorded`, and its use in
  `RunsListPage` and `AutomationDetailPage`.

## Out of scope

- A status stream for open runs, which keep the live-refresh cadence.

## Acceptance

1. Store tests prove one observer call per committed insert and none on a
   failed insert.
2. A gateway test proves workspace scoping and the payload fields.
3. Web tests prove the handler's workspace filter, the hook's debounce and
   automation filter, and a page refresh on an event while no run is open.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/automation/ -run 'RunRecorded' -count=1)
(cd apps/backend && go test -trimpath ./internal/gateway/websocket/ -run 'AutomationRun' -count=1)
(cd apps && pnpm --filter @kandev/web test -- automation-run-recorded components/runs lib/ws/handlers/automation-runs)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/automations-live-runs.spec.ts)
```

## Parallelism

`sequential`

## Results

Done on 2026-11-24.

- `(cd apps/backend && go test -trimpath ./internal/automation/ -run 'RunRecorded' -count=1)`: ok.
- `(cd apps/backend && go test -trimpath ./internal/gateway/websocket/ -count=1)`: ok.
- `(cd apps && pnpm --filter @kandev/web test -- components/automations components/runs lib/ws/handlers lib/automation-run-recorded)`: 121 files, 1109 tests passed.
- `(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/automations-live-runs.spec.ts)`: 2 passed.

Deviation: the frontend message type lives in `AutomationBackendMessageMap`, composed into `OfficeBackendMessageMap`, because `lib/types/backend.ts` is at its line limit.
