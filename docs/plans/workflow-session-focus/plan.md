---
created: 2026-09-19
status: done
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-003
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Workflow session focus

## Overview

After a manual step move, show the conversation selected for that entry.
One sequential work order covers entry correlation, local selection, and both
rendered surfaces. The implementation, review remediation, and permanent tests
are complete.

The task system owns the contract because workflow routing selects the session.
Extend the existing lifecycle requirement/design pair rather than creating a
second UI contract. No architectural decision is needed for this local extension.

## Scope

In scope: the shared stepper on task detail and task preview, new and reused
recipients, pinned source tabs, delayed routing, stale responses, desktop and phone.

Out of scope: changing agent routing, model selection, automatic transition
policy, background board moves, bulk moves, or session runtime recovery.

## Evidence and confirmed decisions

- The screenshot shows Implement with the earlier Astra conversation selected.
- `useWorkflowStepMove` awaits `moveTask` but discards its response.
- `shouldPreservePinnedSessionForTask` preserves nonterminal selected sessions.
- `shouldActivateSessionPanel` can preserve the previous panel for a new tab.
- The move response has `move_id`, but no committed step-entry identity.
- `WorkflowSessionRoute` already records entry identity and destination session.
- The linked hosted task was unavailable through the web tool. No live-instance
  reproduction was performed; the source supports the identified failure paths.
- Profile-only workflow routing previously recorded only a source binding, so a
  manual move could remain pinned to the source conversation. The remediation
  now records the committed recipient route for profile changes, profile reuse,
  same-profile keep-current, and replacement recovery.
- Delayed move responses previously supplied metadata without a freshness guard.
  The store now reconciles one freshness-accepted task projection before it
  considers the response projection.
- Dockview previously read the focus request only when session identity changed.
  The tab hook now subscribes to the scoped request ID and acknowledges only
  after activation.

The user confirmed both choices during the interview:

- Only manual step moves request this focus handoff. Automatic transitions keep
  their existing focus behavior to avoid interrupting conversation reading.
- A later manual session selection cancels the pending handoff. If the user
  selects Astra while Luna starts, Astra stays visible when Luna becomes ready.

These decisions confirm the scope and acceptance criterion 003.3.
No material interview questions remain. Implementation is complete.

## Technical approach

Expose the committed entry identity from `MoveTaskWithOptions` through
`dto.MoveTaskResponse` and `lib/types/http.ts`. Keep it separate from `move_id`.
Capture it before the service refresh can replace the transition result.

Use a shared ephemeral intent from `useWorkflowStepMove`. Reconcile the response
with committed route metadata and a task-owned session. Preserve request and
presentation guards. Cancel through explicit navigation revision changes.

Update selection and clear its obsolete pin atomically. Consume the presentation
request after Dockview panel insertion or in the phone chat layout. Avoid the
panel restoration branch for this request. Do not change global WS adoption rules.

Update `docs/public/tasks-and-workflows.md` with one short explanation of manual
move focus and later navigation.

## ASCII UI preview

UI-01: Task detail or preview, after moving from Plan to Implement.

```text
Desktop before (reported)          Desktop after
Implement 3/6                     Implement 3/6
[Astra selected] [Luna] [Plan]     [Astra] [Luna selected] [Plan]
Earlier conversation              Implement conversation and step prompt

Phone after
+--------------------------------+
| Task             Implement 3/6 |
| Sessions: Luna v               |
|--------------------------------|
| Implement conversation         |
| Step prompt and agent output   |
|                 (scrolls)      |
|--------------------------------|
| Composer                       |
| Chat selected | Other panels   |
+--------------------------------+
```

Required structure: the recipient conversation becomes visible without another
tab click. Phone closes the step picker and shows chat. Names are illustrative.
No tab order, panel geometry, scroll ownership, or touch sizes change.
Do not open the phone keyboard. While routing is pending, retain the current
conversation and existing progress indicator. On failure, retain it and show
the existing error. A later manual selection cancels the handoff.

## Tests

All criteria belong to `AC-TASKS-WORKFLOW-PROFILE-SESSIONS-003`.

| Criteria | Evidence |
| --- | --- |
| .1, .4 | Backend move-response tests preserve exact committed entry identity across refresh and omit it for no-op moves. |
| .1, .3, .4, .5 | Hook/store tests cover pin override, new/reused/same-session recipients, failures, queues, malformed routes, foreign rows, replay, and response/event order. |
| .3, .4 | Late response tests cover task departure/return, preview close/reopen, explicit tab choice, and overlapping moves including repeated destinations. |
| .2, .4 | Dockview and mobile component tests assert visible activation once, delayed panel insertion, and no extra launch or prompt. |

Use `use-workflow-step-move.test.ts`, a new focused store helper test,
`dockview-session-tabs.hook.test.tsx`, and `session-mobile-layout.test.tsx`.
Retain existing WS pin-preservation tests as regression evidence.

## E2E tests

The added `workflow-session-focus.spec.ts` and
`mobile-workflow-session-focus.spec.ts` under `apps/web/e2e/tests/workflow/`
reuse the agent-switch fixture helpers.
Perform the move through the rendered stepper after explicitly selecting the
source conversation. API-only moves before page navigation cannot prove this fix.

Cover new and reused recipients, a pinned source, and a later manual selection
while routing is delayed. Desktop proves task detail and preview focus. Phone
proves visible chat, picker selection, no keyboard autofocus, and no horizontal
overflow. Assert one workflow prompt, using causal HTTP/WS waits.

## Work orders

- [x] [Task 01: Focus the committed recipient](task-01-focus-recipient.md) (done)

Review remediation is included in Task 01. No new work order is required.

## Verification results

Passed:

- Focused backend Go tests in task service, handlers, DTO, and orchestrator.
- Focused frontend regression gate: 69 tests passed; the original six-file gate
  also passed with 81 tests.
- Changed-file ESLint with `--max-warnings 0`: passed.
- Web typecheck and both managed E2E projects: desktop 4 passed, phone 1 passed.
- Public docs tests and validation, specification validation, sleep ratchet, and
  `git diff --check`.

## Review remediation results

- [x] Profile-only steps publish an exact entry-correlated committed recipient
  route across profile change, profile reuse, same-profile keep-current, and
  terminalized-session replacement paths. Added profile-only pinned-source and
  reuse/keep-current regression coverage.
- [x] Delayed HTTP response metadata is accepted only when its route, entry
  identity, and freshness are compatible with the live task projection. Added
  event-before-response and obsolete-response store/pure regressions.
- [x] Dockview subscribes to the task/session-scoped focus request ID and
  acknowledges after activation. Added the mounted same-session, non-chat-panel
  regression.

Additional verification passed:

```bash
(cd apps/backend && go test ./internal/orchestrator -count=1)
(cd apps/web && pnpm exec vitest run lib/state/workflow-session-focus.test.ts hooks/domains/kanban/use-workflow-step-move.test.ts components/task/dockview-session-tab-activation.test.ts components/task/dockview-session-tabs.test.ts components/task/dockview-session-tabs.hook.test.tsx --pool=forks --maxWorkers=1)
(cd apps/web && pnpm e2e:run --host --no-build tests/workflow/workflow-session-focus.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/workflow/mobile-workflow-session-focus.spec.ts)
```

## Risks

- HTTP success can precede routing; reading response primary session alone is unsafe.
- Automatic Dockview selection must not look like a later manual selection.
- The linked hosted task was unavailable, so the local managed E2E scenarios are
  the reproduction evidence for this revision.
- Background transitions retain their current behavior by design.
