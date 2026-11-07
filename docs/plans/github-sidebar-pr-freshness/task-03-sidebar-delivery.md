---
id: 03-sidebar-delivery
title: Prove inactive sidebar delivery
status: done
wave: 3
depends_on:
  - 01-status-progress
  - 02-admission-continuation
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-001
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-001.6
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-001.7
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-001.8
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
---

# Task 03: Prove inactive sidebar delivery

## Summary

Prove that corrected background monitoring publishes a task summary and updates
an unopened task's icon on desktop and phone. Keep presentation and navigation unchanged.

## In scope

- Add `TestPRSidebarDelivery_InactiveTaskReceivesSummary` with real watch storage,
  service monitoring, event bus, and task-summary projector.
- Add desktop and mobile no-selection browser scenarios through the existing mock APIs.
- Record exact verification and update delivery status after all work orders pass.
- Reconcile specification lifecycle without promoting unrelated draft discovery contracts.

## Out of scope

New frontend behavior, controls, copy, layout, per-row polling, test-only production
routes, live GitHub calls, and unrelated sidebar repairs are outside this task.

## Acceptance

1. The corrected monitor produces persisted TaskPR status and a newer
   `task.status_summary.updated` for an inactive task. Failed or obsolete writes emit no newer summary.
2. Desktop and phone icons consume that newer state without target selection,
   disclosure, manual refresh, reload, or a target-task sync request.
3. Required implementation commands pass and their results are recorded in this
   package. Existing draft discovery documents retain their lifecycle status.

## Backend integration

Add `pr_sidebar_status_delivery_test.go` in `internal/github`.
Reuse `pr_watch_retirement_integration_test.go` for full repository/service setup
and imports. Use the actual summary projector with configured PR loaders.
Mix a known PR and a deferred searching sibling. Assert publication identity,
summary revision, unchanged inactive task/session, and no duplicate summary on no-op.

The service regression in Task 01 remains the RED gate for the pacing defect.
These delivery tests can already pass against a direct mock publication.
Do not report that browser mock publication reproduces provider admission.

## Desktop and phone scenarios

Create `pr-sidebar-background-refresh-fixtures.ts` with two tasks.
Keep the navigation task active and the target task unopened.
Seed an attached PR and its underlying feedback with existing mock helpers.
Arm `watchWs` before navigation and all correlated waits before the stimulus.

Publish newer pending, failed, and successful PR states through
`mockGitHubAssociateTaskPR`. Await the target's newer summary event/revision.
Assert its existing `data-pr-state` using the established display mapping.
Keep the route and active task unchanged. Record outbound frames and reject
`github.task_pr.sync` for the target during each stimulus-to-assertion interval.
Do not hover, tap, focus, or select the target icon or task row.
Do not inject state into Zustand or use a helper that reloads on failure.

Desktop uses the existing sidebar. Phone opens `mobile-task-picker-trigger`
and scopes its row to the existing Tasks dialog. Leave that dialog open while
the summary changes. Use the configured `mobile-chrome` Pixel 5 device.
The nearest exemplars are `pr-sidebar-hover-hydration.spec.ts` for desktop rows
and `mobile-pr-sidebar-automation-indicators.spec.ts` for the phone picker.
No new composition, touch target, or UI preview is required.

## Verification

Run from the repository root. In a fresh worktree, install dependencies once
before the first pnpm command: `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -tags fts5 -race ./internal/github -run 'TestPRStatusProgress_|TestPRMonitorContinuation_|TestPRSidebarDelivery_' -count=1 -timeout=5m)
(cd apps/web && pnpm e2e:run --project chromium tests/pr/pr-sidebar-background-refresh.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/pr/mobile-pr-sidebar-background-refresh.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed runner rebuilds the backend and production web assets.
Run desktop and mobile sequentially and record discovered test counts.
Run changed-file Go lint after implementation, using `bash scripts/lint-go-changed`
with the explicit changed Go paths. Record those exact arguments in Results.
Rerun the repository PR-documentation coverage preflight against the final changed paths.

## Files likely touched

- `apps/backend/internal/github/pr_sidebar_status_delivery_test.go` (new)
- `apps/web/e2e/tests/pr/pr-sidebar-background-refresh-fixtures.ts` (new)
- `apps/web/e2e/tests/pr/pr-sidebar-background-refresh.spec.ts` (new)
- `apps/web/e2e/tests/pr/mobile-pr-sidebar-background-refresh.spec.ts` (new)
- This package's plan, work orders, and paired discovery requirement/design.

## Dependencies

Tasks 01 and 02 supply the corrected monitoring and publication path.

## Inputs

- [Discovery requirements](../../specs/integrations/requirements/github-pr-discovery-health.md).
- [Sidebar delivery design](../../specs/integrations/system-design/github-pr-discovery-health.md#sidebar-delivery-and-verification).
- [Bounded task delivery](../../specs/platform/system-design/bounded-task-status-delivery.md).
- `/e2e` fixture-state and UI-state/cleanup rules.
- Existing PR mock helpers and semantic icon attributes.

## Risks

Hover and task selection can silently trigger the refresh that the test must
exclude. Aggregate-only PR fixtures can change eligibility during feedback reads.
Use causal event waits and underlying provider feedback, not arbitrary sleeps.

## Parallelism

`sequential`

## Results

Done on 2026-10-28.

Backend integration:

- `TestPRSidebarDelivery_InactiveTaskReceivesSummary` uses the real watch store,
  the real service apply path, a memory event bus, and the real
  `statussummary.Projector` with a PR loader. A known PR is published while its
  searching sibling still defers; the inactive task's summary moves
  `pending` → `passing` with a newer revision, repeating unchanged evidence emits
  no third summary, the searching task gets no PR association, and the task and
  session rows keep their state and timestamps.
- `(cd apps/backend && go test -tags fts5 -trimpath -race ./internal/github
  -run 'TestPRStatusProgress_|TestPRMonitorContinuation_|TestPRSidebarDelivery_|TestTriggerPRSyncAll'
  -count=1 -timeout=10m)`: ok, 6.7s.

Browser scenarios:

- `apps/web/e2e/tests/pr/pr-sidebar-background-refresh-fixtures.ts` seeds two
  tasks, keeps the navigation task active, associates the target's PR,
  publishes newer states through `mockGitHubAssociateTaskPR`, and records
  outbound gateway frames.
- `(cd apps/web && pnpm e2e:run --host --project chromium
  tests/pr/pr-sidebar-background-refresh.spec.ts -- --retries=0)`: 1 passed
  (11.5s). The unopened row's `data-pr-state` moves `open` → `merged` after the
  target's `task.status_summary.updated` frame, the route stays on the
  navigation task, and no `github.task_pr.sync` targets the row.
- `(cd apps/web && pnpm e2e:run --host --project mobile-chrome
  tests/pr/mobile-pr-sidebar-background-refresh.spec.ts -- --retries=0)`: 1
  passed (10.4s) with the Tasks picker left open for the whole stimulus.

Lint and documentation:

- `bash scripts/lint-go-changed <the changed Go paths listed in the plan>`:
  0 issues.
- `python3 scripts/list-docs.py validate`: 413 decisions, 1795 specifications.
- `python3 scripts/lint-spec-files.py --all`: all files passed.
- PR-documentation coverage preflight: `covered`, all three work orders, no
  errors.
- `git diff --check`: passed.

The paired discovery requirement and system design keep their `draft`
lifecycle; this repair does not promote the whole capability.

