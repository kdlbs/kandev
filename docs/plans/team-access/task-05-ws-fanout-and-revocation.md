---
id: "05-ws-fanout-and-revocation"
title: "WebSocket fan-out and revocation"
status: todo
wave: 2
depends_on: ["03-reach-and-scope-resolution"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/org-units.md"
---

# Task 05: WebSocket Fan-Out and Revocation

## Acceptance

- `Hub.BroadcastToWorkspace` reaches everyone who can **reach** the workspace —
  for an org-visible workspace that is the whole org minus guests — resolved
  through the same predicate as HTTP, not a parallel membership lookup.
- WS subscription authorization uses the same reach predicate at subscribe time,
  and the dispatch backstop checks the action's scope, so a `viewer` cannot
  drive a `task.state` or `session.prompt` action over the socket.
- `workspace.access.updated` is emitted on visibility change, membership add,
  re-role, remove, and ownership transfer.
- Losing access drops that user's subscriptions for the workspace immediately,
  on every open connection they hold, without disturbing other users'
  connections or that user's other workspaces. This covers all three loss paths:
  removal, visibility narrowed to `private`, and role narrowed to `viewer`.
- A two-client test proves a user who lost access receives no further frames,
  and that a still-current user does.
- A user who never had access never receives a frame, proven by client
  assertion rather than by inspecting broadcast call sites.

## Verification

- `go test -race ./internal/gateway/websocket/...`
- `go test ./internal/gateway/websocket/... -run 'TestReachFanout|TestAccessLossDropsSubscription|TestViewerCannotDispatch'`

## Files Likely Touched

- `apps/backend/internal/gateway/websocket/{hub,dispatch_scope,access}.go`
- `apps/backend/internal/task/service/service_events.go`

## Inputs

- Spec: API surface (WS changes), Failure modes (access lost mid-session).
- Patterns: the `Hub.BroadcastToWorkspace` / `//ws:global` conventions and the
  dispatch-backstop ref-extraction rules in `apps/backend/AGENTS.md` — an action
  that invents a new name silently opts out, so the scope check must key off the
  same extraction rather than a parallel one.

## Output Contract

Report the two-client fan-out and revocation tests covering all three
access-loss paths, the viewer-cannot-dispatch test, the goroutine-leak result,
RED/GREEN commands, and set this task plus its plan checkbox to done.
