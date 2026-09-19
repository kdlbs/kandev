---
id: "05-mcp-publish-tool"
title: "publish_review_findings_kandev MCP tool"
status: done
wave: 3
depends_on: ["03-review-service-and-events"]
plan: "plan.md"
spec: "../../specs/agents/requirements/native-code-review.md"
---

# Task 05: `publish_review_findings_kandev` MCP tool

Let an agent session publish findings into the same store the built-in pass writes to.

## Inputs

- Spec **API surface** → Task MCP tool, and the malformed-entry rule in **Failure modes** (an MCP call rejects the whole batch).
- Pattern: `show_walkthrough_kandev` — `internal/mcp/server/server.go` `registerWalkthroughTools` + the steps schema helper, `internal/mcp/server/handlers.go` `showWalkthroughHandler`, `internal/mcp/handlers/handlers.go` `handleShowWalkthrough`, and `pkg/websocket/actions.go` `ActionMCPShowWalkthrough`.

## Work

1. `pkg/websocket/actions.go` — `ActionMCPPublishReviewFindings = "mcp.review.publish_findings"`.
2. `internal/mcp/server/server.go` — `registerReviewTools()` declaring `publish_review_findings_kandev` with the exact parameter schema from the spec, registered alongside the walkthrough tools.
3. `internal/mcp/server/handlers.go` — `publishReviewFindingsHandler()` forwarding the payload and returning a text result naming the stored count and run id; a validation error is returned as a tool error so the agent can retry.
4. `internal/mcp/handlers/handlers.go` — `handlePublishReviewFindings` calling `ReviewService.PublishFindings` with `Trigger = agent`; map `ErrInvalidReviewFinding` to `ws.ErrorCodeInvalidPayload` and anything else to `ws.ErrorCodeInternalError`. Register in the dispatcher next to `ActionMCPShowWalkthrough`, and add the `reviewService` dependency to `NewHandlers`.
5. Update the tool inventory in `docs/public/automation-and-mcp.md` if that file enumerates task-MCP tools (check before editing).

## Acceptance

- Two valid findings produce a `completed` run with `trigger = agent` and both findings, and the WS `task.review.findings_published` event fires.
- A batch with one entry missing `file` returns a tool error and stores nothing.
- `anchor_text` is absent from the tool schema — an agent-published finding stores an empty `anchor_text` and is therefore never re-anchored, only marked stale.

## Verification

```
cd apps/backend && go test ./internal/mcp/...
```

## Files likely touched

`pkg/websocket/actions.go`, `internal/mcp/server/{server.go,handlers.go,server_test.go}`, `internal/mcp/handlers/handlers.go`, `internal/backendapp/main.go`.

## Output contract

Summary, files changed, tests run with results, blockers, risks, `status: done`, plan checkbox.
