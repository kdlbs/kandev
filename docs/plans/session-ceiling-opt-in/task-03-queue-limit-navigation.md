---
id: "03-queue-limit-navigation"
title: "Explain queue limits and link configuration"
status: completed
wave: 3
depends_on:
  - "02-settings-surface"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-003
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
acceptance_criteria:
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.8
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.9
  - AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.7
  - AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.8
  - AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.9
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
---

# Task 03: Explain queue limits and link configuration

## Summary

Name the actual queue limit and link to its configuration from task details.
Verify that saved changes to either limit take effect without restart and that
the banner follows the authoritative reason as work passes through each gate.

## In scope

- Global banner label, instance scope, sessions count, and direct Settings link.
- WIP detail banner, including tasks without sessions, with workflow/destination
  identity, task-count units, and the correct workspace's workflow Settings link.
- Stable workflow-card Settings target and reveal after async loading.
- Reuse existing task-level desktop/mobile composition, not the transcript.
- Unknown/inaccessible destinations, stale counts, and read-only configuration.
- Live WIP reconciliation regression tests and queue reason refresh after saves.
- Localized labels, focused tests, public docs, and final package reconciliation.

## Out of scope

- New queue storage or admission ordering, queue dashboards, or manual bypass UI.
- Starting/resuming work when a configuration link or browser Back is used.
- A new step-specific deep-link API or a new workflow Settings layout.

## Acceptance

1. Global and WIP banners show the correct scope, units, and configuration link
   on desktop and phone. Missing identity never links to an unrelated workflow.
2. Saving increased/unlimited WIP reconciles eligible tasks through the existing
   backend event path; global saves use Task 01. Lower limits preserve admitted work.
3. Changing one limit cannot bypass the other. The displayed cause updates from
   WIP to global capacity when appropriate and clears only on confirmed settlement.

## ASCII UI preview

UI-03 excerpt; the [full preview](plan.md#ui-03-queue-reason-and-configuration-link-desktop-and-phone)
defines persistent placement and phone wrapping.

```text
Global:
Automatic launch: Queued
Global session limit (all workspaces)
5 of 5 sessions in use. Checked 11s ago.
[Configure global session limit]

Workflow:
Task queued: Workflow WIP limit
Development / Implement
2 of 2 tasks admitted.
[Configure workflow WIP limit]
```

Retain destination, queue time, freshness, and retry details from the full
preview. On phones the link wraps below the explanation with a 44px target.
Reuse the existing dedicated mobile task layout and its scroll/safe-area rules.
The Settings links navigate directly; no hover, extra dialog, or launch action.
Counts are examples, never fabricated values. Maps to this work order's ACs.

## Tests

- Extend `launch-queue-status.test.tsx`: global scope persists with stale/unknown
  counts, errors retain their own reason, and href points to the registered target.
- New `wip-queue-status.test.tsx`: destination differs from feeder/current step,
  correct workspace/route, unavailable identity, no session, and task count units.
- Extend workflow-card and Settings target tests for reveal after async load.
- Add `event_handlers_workflow_queue_test.go` with
  `TestWorkflowStepQueueLimitChange`: save a larger/zero/lower WIP value through
  the step update boundary, observe reconciliation, preserve admitted work, and
  ensure destination entry runs once. Cover the shared HTTP/MCP event contract.
- New `tests/workflow/queue-limit-navigation.spec.ts` and
  `mobile-queue-limit-navigation.spec.ts`: both banners navigate to the correct
  configuration; save a raised/disabled limit, return passively, and observe
  queue advancement without restart/reload. Include WIP promotion followed by
  global deferral, then global disable and exact-once launch. Cover phone link
  bounds, no horizontal overflow, and returning to a parked predecessor.
- Use controllable mock tasks and isolated worker settings. Capture and restore
  both global settings and workflow limits in `afterEach`/`finally`. Follow
  causal HTTP/WS events; no fixed sleeps or mutation of the developer instance.

## Verification

From repository root, after Task 02's dependency installation. Apply TDD for new
reason/navigation logic and the live WIP regression. Managed E2E rebuilds assets.

```bash
(cd apps/backend && go test ./internal/orchestrator ./internal/task/service ./internal/workflow/handlers ./internal/mcp/handlers -run 'WorkflowStepQueue|WIP|Wip|WorkflowStep.*Parity|Step.*Event' -count=1)
(cd apps/web && pnpm exec vitest run components/task/launch-queue-status.test.tsx components/task/wip-queue-status.test.tsx components/settings/workflow-card.test.tsx components/settings/settings-target-surfaces.test.tsx lib/settings-discovery/target.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/queue-limit-navigation.spec.ts tests/workflow/queued-session-ownership.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-queue-limit-navigation.spec.ts tests/workflow/mobile-queued-session-ownership.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Verify that every named suite is discovered. Add or adapt
`workflow-card.test.tsx` as necessary for the new target behavior. Run focused
ESLint on the actual changed TS/TSX files before marking done. Use the existing
Traditional Chinese generator after adding English/Portuguese/Simplified Chinese
copy. Record rendered desktop/phone evidence against UI-03.

## Files likely touched

- `apps/web/components/task/launch-queue-status.tsx`, its tests, and new
  `wip-queue-status.tsx` / `wip-queue-status.test.tsx`.
- Task-detail composition, including `task-chat-panel.tsx`,
  `mobile/session-mobile-layout.tsx`, and the no-session detail surface.
- `apps/web/components/settings/workflow-card.tsx` and target tests.
- `apps/web/lib/kanban/wip-queue.ts` only if shared destination context is needed.
- Task/summary types and projection only where required to supply authorized
  destination identity or complete admitted counts; prefer existing data.
- `apps/backend/internal/orchestrator/event_handlers_workflow_queue_test.go` (new).
- `apps/backend/internal/orchestrator/event_handlers_workflow_queue.go` and
  task-service WIP paths only if the regression exposes a live-update gap.
- New desktop/phone queue-limit-navigation E2E and shared helper.
- `apps/web/src/locales/*/task.json`, workflow/settings locale keys as needed.
- `docs/public/tasks-and-workflows.md` and the linked plan/spec status/results.

## Dependencies

Task 02 supplies the global Settings target; Task 01 supplies live global updates.
Existing WorkflowStepUpdated events already invoke queue reconciliation.

## Risks

Workflow WIP counts tasks before destination entry; global capacity counts
sessions after selection. Never show one record as the other or use a feeder's
limit as the destination's. A settings save schedules reconciliation but cannot
promise immediate agent output. Unknown observations and permissions must remain
visible without inventing settings links or skipping backend checks.

## Parallelism

`sequential`

## Inputs

- Task-owned specs: Limit scope and configuration navigation; Limit updates and
  queue explanations. Agent-owned design remains the global settings authority.
- `/mobile-parity`, `/e2e`, scoped web/backend guidance, existing queue and
  mobile workflow Settings examples.

## Results

Implemented separate global session-capacity and workflow WIP banners with
scope-specific copy, units, configuration links, async workflow-card targets,
and desktop/mobile placement. Added live WIP reconciliation coverage through
the existing `WorkflowStepUpdated` event path.

- The focused WIP view-model and banner tests passed with the frontend suite.
- The workflow queue reconciliation regression passed with the orchestrator
  tests, and the global/WIP navigation scenarios passed in managed Chromium and
  mobile Chromium runs.
- Existing queued-session ownership desktop and mobile regressions passed with
  explicit environment capacity setup.
- Public documentation, settings catalog, specification catalog, full spec lint,
  and whitespace checks passed.
