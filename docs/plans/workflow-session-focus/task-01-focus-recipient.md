---
id: "01-focus-recipient"
title: "Focus the committed workflow recipient"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-003
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-003.1
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-003.2
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-003.3
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-003.4
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-003.5
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 01: Focus the committed workflow recipient

## Summary

Implement one manual-move focus handoff tied to the committed step entry.
Use TDD for entry correlation, selection races, and rendered activation.

## In scope

Own the move response identity, local intent, selection action, desktop/phone
consumers, targeted tests, and the public documentation described in the plan.
Preserve the existing background pin policy and session routing.

## Out of scope

Automatic transition policy, board navigation, agent lifecycle changes, and model selection.

## Acceptance

1. The matching committed recipient becomes the visible conversation on desktop
   and phone, including new, reused, and already-selected sessions.
2. A later navigation or move cancels the pending handoff. Missing identity,
   stale routes, no-op moves, errors, and unrelated sessions cannot steal focus.
3. All five criteria have passing evidence. Selection adds no launch or prompt.

## ASCII UI preview

UI-01, excerpt from the [full preview](plan.md#ascii-ui-preview), covers .1-.5.

```text
Desktop: [Astra] [Luna selected] [Plan] -> Implement chat
Phone:   [Sessions: Luna v]            -> Chat selected
Pending/error: retain current conversation and existing status/error UI
```

Retain phone scroll ownership and safe areas. Close the step picker, but do
not focus the composer. Verify actual active panels, not only store values.

## Verification

Run from the repository root. In a fresh worktree, first install dependencies
with `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/task/dto ./internal/orchestrator -run 'MoveTask|WorkflowSession|WorkflowFocus' -count=1)
(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-workflow-step-move.test.ts lib/state/workflow-session-focus.test.ts components/task/dockview-session-tabs.hook.test.tsx components/task/dockview-session-tab-activation.test.ts components/task/mobile/session-mobile-layout.test.tsx lib/ws/handlers/agent-session-pure.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-session-focus.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-session-focus.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Name added backend tests with `WorkflowFocus` so the command includes them.
The new helper and E2E filenames above are owned deliverables. Run changed-file
ESLint as well. Record its exact file list and command with the results.
Managed E2E commands build fresh artifacts. Run the two projects sequentially.

## Files likely touched

- `apps/backend/internal/task/service/service_workflow.go` and focused tests.
- `apps/backend/internal/task/dto/dto.go` and move-response tests.
- `apps/backend/internal/task/handlers/task_http_handlers.go` and focused tests.
- `apps/backend/internal/orchestrator/workflow_session_target.go` and publication tests if required.
- `apps/web/lib/types/http.ts` and `lib/api/domains/kanban-api.ts`.
- `apps/web/hooks/domains/kanban/use-workflow-step-move.ts` and its tests.
- `apps/web/lib/state/workflow-session-focus.ts` and its new tests.
- `apps/web/lib/state/slices/kanban/{types,kanban-slice}.ts`.
- `apps/web/lib/ws/handlers/tasks.ts` for reconciliation if needed.
- `apps/web/components/task/dockview-session-tabs.ts` and activation tests.
- `apps/web/components/task/mobile/session-mobile-layout.tsx` and its tests.
- Both new E2E specs named in the verification block.
- `docs/public/tasks-and-workflows.md`.

## Dependencies

None. Use existing route metadata, transition identity, and manual navigation revision.

## Risks

Do not conflate `move_id`, entry identity, and route operation identity.
Do not let a refreshed task replace the response's committed entry identity.
Reconcile both response-first and event-first sequences. Keep cancelled intents
cancelled across reconnect, remount, and later repeated destinations.

## Parallelism

`sequential`

## Inputs

- Requirement 003 and the design's Manual move recipient focus section.
- The plan's evidence, scenario matrix, and UI-01 preview.
- Existing workflow agent-switch fixtures and Dockview activation tests.
- Scoped backend/web guidance; TDD, E2E, and mobile-parity skills.

## Results

Implementation is complete. The move response carries the committed entry
identity, shared browser-local intent reconciles response-first and event-first
updates, and desktop plus phone consume the same recipient focus request.

The exact verification commands passed:

```bash
(cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/task/dto ./internal/orchestrator -run 'MoveTask|WorkflowSession|WorkflowFocus' -count=1)
(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-workflow-step-move.test.ts lib/state/workflow-session-focus.test.ts components/task/dockview-session-tabs.hook.test.tsx components/task/dockview-session-tab-activation.test.ts components/task/mobile/session-mobile-layout.test.tsx lib/ws/handlers/agent-session-pure.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-session-focus.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-session-focus.spec.ts)
```

The desktop E2E passed 4 tests and the phone E2E passed 1 test. The focused
Vitest gate passed 69 tests; the original six-file gate passed 81 tests. The
changed-file lint command also passed:

```bash
(cd apps/web && pnpm exec eslint --max-warnings 0 components/kanban-with-preview.tsx components/task/dockview-session-tab-activation.test.ts components/task/dockview-session-tab-activation.ts components/task/dockview-session-tabs.ts components/task/mobile/mobile-task-move-options.tsx components/task/mobile/session-mobile-layout.tsx components/task/mobile/session-task-switcher-sheet-props.ts components/task/mobile/session-task-switcher-sheet.tsx components/task/task-move-context-menu.tsx hooks/domains/kanban/use-workflow-step-move.test.ts hooks/domains/kanban/use-workflow-step-move.ts hooks/use-task-sessions.ts lib/state/default-state.ts lib/state/slices/kanban/kanban-slice.ts lib/state/slices/kanban/types.ts lib/state/store-overrides.ts lib/types/http.ts lib/ws/handlers/agent-session.ts lib/ws/handlers/tasks.ts lib/state/workflow-session-focus.ts lib/state/workflow-session-focus.test.ts)
```

Public docs validation, specification validation, the E2E sleep ratchet, and
`git diff --check` passed.

Review remediation is complete. Profile-only workflow steps now publish an
entry-correlated committed recipient route for profile changes, reuse,
same-profile keep-current, and terminalized-session replacement. Delayed HTTP
move metadata is accepted only after route, entry, and freshness checks against
the live task projection. Dockview subscribes to the scoped focus request ID
and acknowledges it only after activating the requested session panel.

The remediation regressions passed for profile-only pinned-source routing,
profile reuse and keep-current routing, event-before-response and obsolete HTTP
metadata, and same-session Dockview activation from a non-chat panel. The full
orchestrator package also passed after the compatibility and replacement-route
fixes.
