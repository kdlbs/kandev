---
id: "01-sdk-extension-seam"
title: "SDK extension seam for cursor/task"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/cursor-subagent-metadata.md"
---

# Task 01: SDK extension seam for cursor/task

Open a seam so the ACP client can accept the non-`_`-prefixed `cursor/task`
request instead of returning `-32601`, and expose the parsed request to the
adapter.

Upstream ACP rejects non-`_` inbound methods on the client side. The
implementation that landed opens the seam in the pinned ACP fork instead: the
fork routes any unrecognized inbound client request to the client's
`ExtensionMethodHandler`, and Kandev passes its client to
`acp.NewClientSideConnection` with `WithCursorTaskHandler` (no repo-local
connection helper is involved).

## Acceptance
- The pinned ACP fork routes inbound vendor request methods (including
  `cursor/task`) to `Client.HandleExtensionMethod`; the outbound `_`-prefix
  contract is unchanged (inbound-accept only).
- `apps/backend/internal/agentctl/server/acp/client.go` `Client` implements the
  extension handler: `cursor/task` returns an empty success result; any other
  unrecognized method returns `acp.NewMethodNotFound(method)`.
- A `WithCursorTaskHandler` client option (mirroring `WithUpdateHandler`) hands
  the raw params to a callback the adapter can set.

## Verification
`cd apps/backend && go test ./internal/agentctl/server/acp/...`

## Files likely touched
- `apps/backend/internal/agentctl/server/acp/client.go` — add
  `HandleExtensionMethod`, `WithCursorTaskHandler`, and the handler field.
- `apps/backend/internal/agentctl/server/acp/client_test.go` — dispatch tests.
- The pinned ACP fork (`kdlbs/acp-go-sdk` PR #3) — widen the client extension
  seam; consumed via the `go.mod` `replace`.
- `apps/backend/internal/agentctl/server/acp/connection_test.go` (new) — live
  wire-path test for `cursor/task` and unknown vendor requests.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go` and
  `apps/backend/internal/agentctl/server/utility/acp_executor.go` — use the new
  helper instead of constructing the ACP SDK connection directly.

## Dependencies
None. ADR-2026-08-20 (Option A) is accepted; this task is unblocked.

## Parallelism
sequential.

## Inputs
- Spec: "API surface", "Failure modes".
- Plan: "Resolved decision and implemented seam", "Area 1".
- Code refs: `client.go` option pattern (`WithUpdateHandler`), ACP
  `ClientSideConnection` wire framing, and the adapter/utility connection
  call-sites.

## Output contract
Summary, files changed, tests run, blockers, risks; update this task's status
and the `plan.md` Wave 1 checkbox.

## Results
- Added `CursorTaskHandler`, `WithCursorTaskHandler`, `SetCursorTaskHandler`,
  and `HandleExtensionMethod` to `apps/backend/internal/agentctl/server/acp/client.go` so `cursor/task` returns a success result and unknown vendor methods still return `-32601`.
- Widened the client extension seam in the pinned ACP fork (`kdlbs/acp-go-sdk` PR #3) so any unrecognized inbound client request routes to `HandleExtensionMethod`; consumed via the `go.mod` `replace`. No repo-local connection proxy was added; the adapter passes its client to `acp.NewClientSideConnection` with `WithCursorTaskHandler`.
- `cd apps/backend && go test ./internal/agentctl/server/acp/...` passed.
