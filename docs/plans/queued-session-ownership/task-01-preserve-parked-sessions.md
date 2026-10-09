---
id: "01-preserve-parked-sessions"
title: "Preserve parked sessions during inspection"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-003
acceptance_criteria:
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.1
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.2
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.3
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.4
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.5
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.6
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.7
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
---

# Task 01: Preserve parked sessions during inspection

## Policy supersession, 2026-09-18

The [revised conversation recovery package](../session-open-recovery-eligibility/plan.md)
supersedes parked-session suppression and parking-note presentation in this
historical package. Opening an earlier conversation now follows normal recovery.
Keep queue identity, admission, callback, and reconciliation coverage. Replace
old no-resume and parked-note assertions in the revised package's work orders.
Historical results and outstanding PostgreSQL checks below are unchanged.

## Summary

Make workflow parking durable and keep passive inspection separate from explicit
execution. Deliver the backend guard and browser caller changes together so
opening the queued task cannot wake Astra or create another launch.

## In scope

- Implement the design's stamped `workflow_parking` metadata, no-runtime case,
  conservative legacy resolution, and conditional clear on legitimate activation.
- Add inspection source and status eligibility; guard launch and ensure before
  side effects. Propagate no-execution outcomes through browser fallbacks.
- Preserve actual manual overrides, direct conversation follow-ups, workflow
  re-entry, preference semantics for ordinary sessions, and existing permissions.
- Add backend/repository RED tests, hook tests, and the first desktop lifecycle
  scenario in `queued-session-ownership.spec.ts` using the shared fixture.
- Verify passive suppression creates no turn or empty-output warning. Keep the
  genuine empty-turn test; diagnose any failure rather than suppressing all notices.

## Out of scope

Queue UI markup belongs to Task 03. Task state/replay race repair belongs to
Task 02. Do not change background-work parking, lifecycle defaults, or Office.

## Acceptance

1. The full/free capacity and preference on/off matrix leaves parked Astra
   untouched after open, tab selection, preview, reload, and reconnect, while
   preserving Luna's accepted payload and queue time.
2. Explicit Astra execution targets Astra alone; valid workflow reuse succeeds.
   Old tombstones and no-runtime parking work across restart and stale-clear races.
3. Status and request handlers both enforce the decision; ownership errors and
   queued responses never trigger fresh/resume fallback or manual-override notes.

## Verification

Run from the repository root. Bootstrap dependencies once if absent.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -race ./internal/orchestrator -run 'TestQueuedSessionInspection|TestWorkflowParking|Test.*Resume|Test.*ProfileSwitch' -count=1)
(cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestWorkflowParking' -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test ./internal/task/repository/sqlite -run 'TestPostgresWorkflowParking' -count=1)
(cd apps/backend && go test ./internal/task/service -run 'Test.*Turn|Test.*SessionMessage' -count=1)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-ensure-task-session.test.ts hooks/use-ensure-task-session.test.ts lib/services/session-launch-helpers.test.ts lib/ws/handlers/empty-turn-notice.test.ts)
(cd apps/web && pnpm run typecheck)
make -C apps/backend build
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/queued-session-ownership.spec.ts)
git diff --check
```

Add planned missing tests before their commands. PostgreSQL tests use
`testutil.PostgresDSNFromEnv` and isolated schemas. Provision a disposable test
database through the repository's existing test setup and set
`KANDEV_TEST_POSTGRES_DSN` without printing it. The explicit environment guard
prevents a skipped PostgreSQL test from counting as a pass. If unavailable,
record the blocked database check and do not mark this work order done.
Frontend behavior changes here do not change layout, so hook tests plus the
desktop flow suffice for this slice; Task 03 owns the full mobile flow.

## Files likely touched

- `apps/backend/internal/task/models/models.go` and a focused parking model file.
- `apps/backend/internal/task/repository/interface.go`, `sqlite/session.go`,
  `sqlite/session_workflow.go`, and new `workflow_parking_test.go`.
- `apps/backend/internal/orchestrator/workflow_profile_session_lifecycle.go`.
- `apps/backend/internal/orchestrator/session_launch.go`, `task_operations.go`,
  `ceiling_seam4.go`, `ceiling_replay.go`, and new `queued_session_inspection_test.go`.
- `apps/backend/internal/orchestrator/dto/dto.go` (`TaskSessionStatusResponse`),
  `session_ensure.go`, and launch/ensure HTTP/WS mappings.
- `apps/web/lib/services/session-launch-service.ts`, `session-launch-helpers.ts`.
- `apps/web/hooks/domains/session/use-session-resumption.ts`,
  `use-session-resumption-operations.ts`, and both
  `hooks/use-ensure-task-session.ts` and `hooks/domains/session/use-ensure-task-session.ts`.
- Their test files and the workflow E2E file/helper named above.

## Dependencies

None. Read the existing profile-switch, resume, launch, and empty-turn tests first.

## Risks

Stop-intent presence is not current parking. A queued or suppressed launch
response must not be interpreted as a successful running session. A status read
must not clear markers or promote the selected tab.

## Parallelism

`sequential`

## Inputs

[Design: inspection intent and durable workflow parking](../../specs/tasks/system-design/queued-session-ownership.md#inspection-intent).
Use `event_handlers_workflow_profile_session_policy_test.go` and existing
`use-session-resumption.test.ts` fixtures. Read the [plan's E2E contract](plan.md#e2e-tests).

## Results

Implementation is present. The durable stamped parking marker, passive launch
source, status eligibility guard, and browser recovery propagation are covered
by the existing orchestrator/model tests and the new repository parking test.
The desktop workflow scenario confirms that inspecting the queued task and its
parked source does not start, prompt, or queue activity for the source session.

Passing checks:

- `go test -race ./internal/orchestrator -run
  'TestQueuedSessionInspection|TestWorkflowParking|Test.*Resume|Test.*ProfileSwitch'
  -count=1`
- `go test ./internal/task/repository/sqlite -run 'TestWorkflowParking' -count=1`
- The focused frontend recovery, ensure, launch-helper, and empty-turn tests
  passed (7 files, 97 tests).
- `pnpm run typecheck`
- `make -C apps/backend build`
- `pnpm e2e:run --project chromium
  tests/workflow/queued-session-ownership.spec.ts`
- `git diff --check`

The PostgreSQL counterpart `TestPostgresWorkflowParking` is implemented, but
the required command is blocked because `KANDEV_TEST_POSTGRES_DSN` is unset.
The work order therefore remains in progress until that database check runs.

## Follow-up: Session-open recovery eligibility

The [recovery eligibility repair](../session-open-recovery-eligibility/plan.md)
owns the historical-stop and settled-deferral regressions found after restart.
It replaces the parking suppression policy and removes the parking note, with
separate and combined recovery cases on desktop and phone. Existing results and PostgreSQL prerequisites here
remain unchanged. The follow-up work order is pending implementation.
