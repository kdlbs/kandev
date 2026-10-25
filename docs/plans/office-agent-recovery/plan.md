---
created: 2026-09-08
status: done
requirements:
  - REQ-OFFICE-AGENT-RECOVERY-001
  - REQ-OFFICE-AGENT-RECOVERY-002
  - REQ-OFFICE-AGENT-RECOVERY-003
system_design:
  - ../../specs/office/system-design/agent-recovery.md
legacy_specs: []
---

# Implementation Plan: Office agent recovery

## Overview

Give an operator a control on the Office agent detail surface that returns a
`paused` or `stopped` agent to `idle`, so recovering an out-of-service agent no
longer requires a hand-written backend request.

The backend transition table already exists, but recovery needs a guarded write
around it. The delivery order is therefore: pin the transition contract, add
the compare-and-set recovery path, add the web API client that sends its
precondition, render the control, then prove the operator path in a browser.
The web control uses the guarded path so a stale browser cannot clear a live
working owner.

## Scope

### In scope

- A regression pin over the agent status transition table for the transitions
  this capability consumes, including the same-status no-op.
- A guarded recovery status request that accepts only a current `paused` or
  `stopped` agent, treats current `idle` as an idempotent success, and refuses a
  stale non-recoverable status.
- A web API client function for `PATCH /api/v1/office/agents/:id/status`.
- A recovery control and pause-reason text on the agent detail identity strip,
  present on every agent sub-route.
- In-flight, failure, and store-patch behavior for the recovery request.
- Recovery copy in the five complete locale catalogs and the pseudo catalog.
- Desktop and mobile browser coverage for the recovery path and for the agent
  list remaining reachable for out-of-service agents.

### Out of scope

- An event publisher or activity entry for the status endpoint.
- Pausing or stopping an agent from the UI.
- Resetting the consecutive-failure counter, dismissing inbox entries, or
  re-queueing runs. Those stay with the existing auto-pause recovery flow.
- The `pending_approval -> idle` transition.
- Bulk or multi-agent recovery.

## Technical approach

### Backend contract and guarded recovery

`apps/backend/internal/office/agents/service.go` owns `allowedTransitions` and
`validateStatusTransition`. The recovery handler accepts an optional
`expected_status` precondition. When present, the service uses
`UpdateAgentStatusFieldsIfCurrent` and retries a transition that moves between
`paused` and `stopped`; it treats `idle` as success and refuses `working`,
`pending_approval`, and unknown states. This preserves `working_run_id` for a
live run. Table tests pin the transition table, the guarded service path, and
the HTTP `409` response for a stale working agent.

### Web API client

`updateAgentProfile` cannot serve: `agentPayload` never emits `status`, and the
general `PATCH /agents/:id` endpoint assigns status without consulting the
transition table. `updateAgentStatus(id, status, options?)` posts the constant
target and, for the recovery control, includes the rendered status as
`expected_status`. It returns `normalizeAgent(res.agent)`.

### Recovery control

`apps/web/app/office/agents/[id]/layout.tsx` renders the identity strip
(`data-testid="agent-identity-strip"`) above the tab nav, so the control and the
pause reason live there and are present on every sub-route without repetition.
Extract them into `apps/web/app/office/agents/[id]/components/agent-recovery-control.tsx`
to keep the layout within the repo's component-size limits.

The control renders only when `agent.status` is `paused` or `stopped`; every
other value, including an empty or unrecognized one, renders nothing. On
activation it sets an in-flight flag that disables further activation, calls
`updateAgentStatus(id, "idle", { expectedStatus: agent.status })`, and on
success patches the store through the existing
`updateOfficeAgentProfile(workspaceId, id, patch)` action using the `status` and
`pauseReason` from the response body. No optimistic write precedes the
response. On failure the store is untouched, `toast` surfaces the error, and
the in-flight flag clears.

The pause-reason text renders whenever `agent.pauseReason` is a non-empty
string, and is driven by the same store row, so a successful recovery that
returns an empty `pause_reason` removes it without a reload.

### Copy

Add recovery keys to `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/office.json`
and regenerate the pseudo catalog. Use `pnpm run i18n:zh-hant` for the
Traditional Chinese pair.

### Mobile design contract

- Desktop outcome: a paused or stopped agent's detail surface shows the pause
  reason and a recovery control; activating it returns the agent to `idle` in
  place.
- Mobile entry point: the same identity strip on the same route. There is no
  separate mobile affordance and no capability the phone lacks.
- Nearest shipped exemplars: `app/office/agents/[id]/layout.tsx` for the strip's
  existing responsive behavior, and
  `e2e/tests/office/mobile-agent-dashboard.spec.ts` for the office viewport
  containment pattern.
- Hierarchy and surface: role badge, status, pause reason, then the recovery
  control. The strip is a single `flex` row today; adding two children can
  overflow a 393px viewport, so the strip wraps rather than scrolls
  horizontally.
- Scroll and safe area: the strip stays viewport-contained. No new scroll owner
  and no drawer; the interaction is a single action with no content to page
  through.
- State: request, store patch, and error handling are shared with desktop. Only
  the strip's layout is responsive.
- Mobile proof: a `mobile-*.spec.ts` flow on a Pixel 5 recovers a paused agent,
  asserts no horizontal document overflow, and asserts the control's active
  hitbox is at least 44px tall.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-OFFICE-AGENT-RECOVERY-001.1` | `agent-recovery-control.test.tsx` renders the control for `paused` and `stopped`; the control lives in the shared `layout.tsx` identity strip, so every sub-route inherits it by construction, and `e2e/tests/office/agents.spec.ts` exercises the `dashboard` sub-route |
| `AC-OFFICE-AGENT-RECOVERY-001.2` | `agent-recovery-control.test.tsx`, table over `idle`, `working`, `pending_approval`, `""`, and an unrecognized value |
| `AC-OFFICE-AGENT-RECOVERY-001.3` | `office-agent-status-api.test.ts` asserts method, URL, the constant `idle` target, and the optional `expected_status` precondition |
| `AC-OFFICE-AGENT-RECOVERY-001.4` | `agent-recovery-control.test.tsx` with a deferred response: no status change is rendered before it resolves |
| `AC-OFFICE-AGENT-RECOVERY-001.5` | `agent-recovery-control.test.tsx` (control unmounts after the response) and `agents.spec.ts` (control hidden after recovery, no reload) |
| `AC-OFFICE-AGENT-RECOVERY-001.6` | `agent-recovery-control.test.tsx`, non-empty and empty `pauseReason` |
| `AC-OFFICE-AGENT-RECOVERY-001.7` | `agent-recovery-control.test.tsx` with an empty `pause_reason` in the response; also `agents.spec.ts` |
| `AC-OFFICE-AGENT-RECOVERY-001.8` | `office-agent-status-api.test.ts` asserts exactly one request is issued and that it targets the status endpoint; no dismissal or run-queue call is made |
| `AC-OFFICE-AGENT-RECOVERY-001.9` | `agent-recovery-control.test.tsx` queries by role and accessible name; keyboard operability relies on native `<button>` semantics (no custom key handling) and is not separately E2E-tested |
| `AC-OFFICE-AGENT-RECOVERY-002.1` | `agent-recovery-control.test.tsx` with a deferred response: a second activation issues no second request and the control reports progress |
| `AC-OFFICE-AGENT-RECOVERY-002.2` | `agent-recovery-control.test.tsx` with a rejected response: status and pause reason unchanged, error surfaced, control re-enabled, `toast.error` called |
| `AC-OFFICE-AGENT-RECOVERY-002.3` | `service_status_transition_test.go`, `idle -> idle` accepted, plus the guarded service idempotence test |
| `AC-OFFICE-AGENT-RECOVERY-002.4` | `service_status_transition_test.go`, repeated guarded recovery converges on `idle` |
| `AC-OFFICE-AGENT-RECOVERY-002.5` | `service_status_transition_test.go` and `handler_routing_test.go` refuse a stale working row without clearing its owner; API and component tests assert the precondition and response ordering |
| `AC-OFFICE-AGENT-RECOVERY-002.6` | `service_status_transition_test.go` (refused transition, unknown source) and `agent-recovery-control.test.tsx` (a non-2xx response is the failure path) |
| `AC-OFFICE-AGENT-RECOVERY-003.1` | `agents.spec.ts` pauses the seeded agent, opens the agents list, and navigates through its card to the detail surface |
| `AC-OFFICE-AGENT-RECOVERY-003.2` | Architectural: the handler adds no permission check beyond the route's existing workspace-membership gate (verified by reading `handler.go`); not covered by a dedicated test |

## E2E tests

- Chromium, `apps/web/e2e/tests/office/agents.spec.ts`: recover a `paused`
  agent from the `dashboard` sub-route and assert the control disappears;
  assert the control is absent for an `idle` agent and reappears after a
  manual stop; verify a paused agent remains reachable from the list. Covers
  `AC-...-001.1`, `.5`, `.7`, and `003.1`.
- Mobile Chrome, `apps/web/e2e/tests/office/mobile-agent-recovery-control.spec.ts`:
  recover a `paused` agent on a Pixel 5 by tap; assert no document overflow, a
  44px minimum control hitbox (height and width), and the real backend result.
  Covers `AC-...-001.1`.

Coverage remains narrower than first planned for sub-route presence beyond
`dashboard` and keyboard activation. The shared layout and native button
semantics cover those paths; dedicated browser cases remain test-rigor debt,
not a production defect.

## Work orders

- [x] [Task 01: Pin the agent status transition contract](task-01-pin-status-transitions.md)
- [x] [Task 02: Add the Office agent status mutation client](task-02-status-mutation-client.md)
- [x] [Task 03: Present operator recovery on the agent detail surface](task-03-recovery-control.md)

Browser coverage (originally planned as a separate Task 04) was delivered
inside Task 03 as `agents.spec.ts` and `mobile-agent-recovery-control.spec.ts`
rather than a standalone work order; no `task-04-recovery-e2e.md` exists.

## Verification results

Delivered on PR [#3534](https://github.com/kdlbs/kandev/pull/3534). The fixup
adds a compare-and-set recovery path and keeps the contributor's original
transition and UI coverage. Fixup verification passed with 79 backend tests,
23 frontend unit tests, TypeScript typecheck, changed-file ESLint, i18n checks,
and specification lint. The existing PR CI remains the source for the full
browser, lint, build, and i18n matrix.

## Risks

- The office E2E fixture resets the shared CEO agent to `idle` in a
  `beforeEach`. A recovery spec must move the agent to `paused` inside the test
  body, after that hook, or it will find an `idle` agent and no control.
- `OfficeApiClient.updateAgentStatus` sends only the requested status and
  optional pause reason. The browser recovery control also sends its guarded
  `expected_status` precondition through the web API client.
- The identity strip wraps on narrow screens; the mobile spec asserts that the
  document remains within the viewport.
- Five locale catalogs and the pseudo catalog gate the build. Adding English
  copy alone fails `i18n:check`.
- Another operator's already-open view keeps the stale status: the endpoint
  publishes no event. This is a stated exclusion, not a defect, and no test
  should assert cross-client propagation.
