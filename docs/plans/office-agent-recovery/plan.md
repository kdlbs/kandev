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

The backend transition already exists and is not changed. The delivery order is
therefore: pin the backend contract the control depends on, add the web API
client that drives it, render the control, then prove the operator path in a
browser. The transition pin and the API client touch disjoint trees and run in
parallel; the control depends on the client, and the end-to-end evidence depends
on the control.

## Scope

### In scope

- A regression pin over the agent status transition table for the transitions
  this capability consumes, including the same-status no-op.
- A web API client function for `PATCH /api/v1/office/agents/:id/status`.
- A recovery control and pause-reason text on the agent detail identity strip,
  present on every agent sub-route.
- In-flight, failure, and store-patch behavior for the recovery request.
- Recovery copy in the five complete locale catalogs and the pseudo catalog.
- Desktop and mobile browser coverage for the recovery path and for the agent
  list remaining reachable for out-of-service agents.

### Out of scope

- Any change to the status handler, service, repository, or transition table.
- An event publisher or activity entry for the status endpoint.
- Pausing or stopping an agent from the UI.
- Resetting the consecutive-failure counter, dismissing inbox entries, or
  re-queueing runs. Those stay with the existing auto-pause recovery flow.
- The `pending_approval -> idle` transition.
- Bulk or multi-agent recovery.

## Technical approach

### Backend contract pin (no production change)

`apps/backend/internal/office/agents/service.go` owns `allowedTransitions` and
`validateStatusTransition`. Neither has a direct test today, so the idempotence
and refusal behavior that `REQ-OFFICE-AGENT-RECOVERY-002` asserts is currently
unpinned. Add a table test over `validateStatusTransition` covering
`paused -> idle`, `stopped -> idle`, `idle -> idle`, a refused transition, and
an unknown source status. No production Go file changes.

### Web API client

`apps/web/lib/api/domains/office-api.ts` has no caller of the status endpoint.
`updateAgentProfile` cannot serve: `agentPayload` never emits `status`, and the
general `PATCH /agents/:id` endpoint assigns status without consulting the
transition table. Add `updateAgentStatus(id, status, options?)` alongside
`updateAgentProfile`, posting `{ status }` to `${BASE}/agents/${id}/status` and
returning `normalizeAgent(res.agent)`. The target is a caller-supplied constant,
never derived from a previously rendered status.

### Recovery control

`apps/web/app/office/agents/[id]/layout.tsx` renders the identity strip
(`data-testid="agent-identity-strip"`) above the tab nav, so the control and the
pause reason live there and are present on every sub-route without repetition.
Extract them into `apps/web/app/office/agents/[id]/components/agent-recovery-control.tsx`
to keep the layout within the repo's component-size limits.

The control renders only when `agent.status` is `paused` or `stopped`; every
other value, including an empty or unrecognized one, renders nothing. On
activation it sets an in-flight flag that disables further activation, calls
`updateAgentStatus(id, "idle")`, and on success patches the store through the
existing `updateOfficeAgentProfile(workspaceId, id, patch)` action using the
`status` and `pauseReason` from the response body. No optimistic write precedes
the response. On failure the store is untouched, `toast` surfaces the error, and
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
| `AC-OFFICE-AGENT-RECOVERY-001.3` | `office-agent-status-api.test.ts` asserts method, URL, and an exact `{"status":"idle"}` body |
| `AC-OFFICE-AGENT-RECOVERY-001.4` | `agent-recovery-control.test.tsx` with a deferred response: no status change is rendered before it resolves |
| `AC-OFFICE-AGENT-RECOVERY-001.5` | `agent-recovery-control.test.tsx` (control unmounts after the response) and `agents.spec.ts` (control hidden after recovery, no reload) |
| `AC-OFFICE-AGENT-RECOVERY-001.6` | `agent-recovery-control.test.tsx`, non-empty and empty `pauseReason` |
| `AC-OFFICE-AGENT-RECOVERY-001.7` | `agent-recovery-control.test.tsx` with an empty `pause_reason` in the response; also `agents.spec.ts` |
| `AC-OFFICE-AGENT-RECOVERY-001.8` | `office-agent-status-api.test.ts` asserts exactly one request is issued and that it targets the status endpoint; no dismissal or run-queue call is made |
| `AC-OFFICE-AGENT-RECOVERY-001.9` | `agent-recovery-control.test.tsx` queries by role and accessible name; keyboard operability relies on native `<button>` semantics (no custom key handling) and is not separately E2E-tested |
| `AC-OFFICE-AGENT-RECOVERY-002.1` | `agent-recovery-control.test.tsx` with a deferred response: a second activation issues no second request and the control reports progress |
| `AC-OFFICE-AGENT-RECOVERY-002.2` | `agent-recovery-control.test.tsx` with a rejected response: status and pause reason unchanged, error surfaced, control re-enabled, `toast.error` called |
| `AC-OFFICE-AGENT-RECOVERY-002.3` | `service_status_transition_test.go`, `idle -> idle` accepted |
| `AC-OFFICE-AGENT-RECOVERY-002.4` | `service_status_transition_test.go`, same-status acceptance is what makes the second writer converge |
| `AC-OFFICE-AGENT-RECOVERY-002.5` | `office-agent-status-api.test.ts` (target is the caller's constant, never a rendered status) and `agent-recovery-control.test.tsx` (a response for a since-superseded status is not applied to the store) |
| `AC-OFFICE-AGENT-RECOVERY-002.6` | `service_status_transition_test.go` (refused transition, unknown source) and `agent-recovery-control.test.tsx` (a `400` is the failure path) |
| `AC-OFFICE-AGENT-RECOVERY-003.1` | Architectural: no status filter in `listAgents`/`agents-page-client.tsx`/`selectOfficeAgentProfiles` (verified by reading the code); not covered by a dedicated navigation test |
| `AC-OFFICE-AGENT-RECOVERY-003.2` | Architectural: the handler adds no permission check beyond the route's existing workspace-membership gate (verified by reading `handler.go`); not covered by a dedicated test |

## E2E tests

- Chromium, `apps/web/e2e/tests/office/agents.spec.ts`: recover a `paused`
  agent from the `dashboard` sub-route and assert the control disappears;
  assert the control is absent for an `idle` agent and reappears after a
  manual stop. Covers `AC-...-001.1`, `.5`, `.7`.
- Mobile Chrome, `apps/web/e2e/tests/office/mobile-agent-recovery-control.spec.ts`:
  recover a `paused` agent on a Pixel 5 by tap; assert a 44px minimum control
  hitbox (height and width) and the real backend result. Covers `AC-...-001.1`.

Coverage narrower than first planned: sub-route presence beyond `dashboard`,
list-to-detail navigation, and keyboard activation are architectural
guarantees confirmed by reading the code (see the Tests table) rather than
dedicated E2E cases; tracked as test-rigor debt, not a production defect.

## Work orders

- [x] [Task 01: Pin the agent status transition contract](task-01-pin-status-transitions.md)
- [x] [Task 02: Add the Office agent status mutation client](task-02-status-mutation-client.md)
- [x] [Task 03: Present operator recovery on the agent detail surface](task-03-recovery-control.md)

Browser coverage (originally planned as a separate Task 04) was delivered
inside Task 03 as `agents.spec.ts` and `mobile-agent-recovery-control.spec.ts`
rather than a standalone work order; no `task-04-recovery-e2e.md` exists.

## Verification results

Delivered on PR [#3534](https://github.com/kdlbs/kandev/pull/3534). Backend
pin: `go test -tags fts5 ./internal/office/agents/...` (5/5). Web unit:
`agent-recovery-control.test.tsx` + `office-agent-status-api.test.ts`
(vitest, all green). E2E: `agents.spec.ts` (chromium) and
`mobile-agent-recovery-control.spec.ts` (mobile-chrome), both green against a
real backend. `eslint --max-warnings 0`, `tsc --noEmit`, and `i18n:check`
clean.

## Risks

- The office E2E fixture resets the shared CEO agent to `idle` in a
  `beforeEach`. A recovery spec must move the agent to `paused` inside the test
  body, after that hook, or it will find an `idle` agent and no control.
- `OfficeApiClient.updateAgentStatus` sends only `status`. Seeding a
  pause-reason case needs an optional `pause_reason` argument on that helper.
- The identity strip is a single non-wrapping `flex` row. Two new children can
  push it past a phone viewport; the mobile spec is the guard.
- Five locale catalogs and the pseudo catalog gate the build. Adding English
  copy alone fails `i18n:check`.
- Another operator's already-open view keeps the stale status: the endpoint
  publishes no event. This is a stated exclusion, not a defect, and no test
  should assert cross-client propagation.
