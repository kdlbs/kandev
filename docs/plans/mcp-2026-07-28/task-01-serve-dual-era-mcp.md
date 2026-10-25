---
id: "01-serve-dual-era-mcp"
title: "Serve dual-era MCP"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/mcp-protocol-compatibility.md"
---

# Task 01: Serve Dual-Era MCP

## Inputs

- `REQ-AGENTS-MCP-PROTOCOL-001` and acceptance criteria `.1` through `.5`
  and `.8`.
- `docs/specs/agents/system-design/mcp-protocol-compatibility.md` sections
  `Shared server`, `Protocol selection`, and `Failure behavior`.
- `docs/decisions/2026-08-30-dual-era-mcp-protocol.md`.
- The current shared server in `apps/backend/internal/mcp/server/server.go`.

## Change

1. Check the available mark3labs v1 releases and their protocol support.
   Select the newest compatible v1 release. Use `v1.0.0-beta.1` if no newer
   compatible release exists.
2. Write failing server tests for automatic discovery, direct modern
   requests, legacy initialize, concurrent eras, and invalid modern metadata.
3. Upgrade `github.com/mark3labs/mcp-go` and tidy the backend module.
4. Adapt the shared server to the v1 constructor, transport, and hook APIs.
   The current after-call hook expects `*mcp.CallToolResult`; v1 supplies a
   general result value.
5. Keep one Streamable HTTP handler for every tool profile. Preserve tool
   filtering, handlers, results, annotations, and dynamic plugin tools.
6. Keep modern tool-list cache metadata private with a zero lifetime.

## Acceptance

- A modern automatic client selects `2026-07-28` on `/mcp` and can list and
  call a representative tool.
- A valid direct modern request works without `server/discover`.
- A legacy client can initialize, list, and call a tool on the same handler.
- Modern and legacy requests work concurrently without shared protocol state.
- Invalid modern metadata returns a protocol error and does not enter the
  legacy path.
- Every Kandev tool profile builds with the same dual-era server.
- Tool-list cache metadata is private and has no positive lifetime.

## Verification

```bash
cd apps/backend
go mod tidy
go test -race ./internal/mcp/server
git diff --check -- go.mod go.sum internal/mcp/server
```

Repeat the tests if later edits change the module files or shared server.

## Files likely touched

- `apps/backend/go.mod`
- `apps/backend/go.sum`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- New focused protocol fixture files under
  `apps/backend/internal/mcp/server/`, if needed.

## Dependencies

None. This task establishes the SDK and server API used by later work.

## Output contract

Report the selected SDK version, protocol cases covered, API adaptations,
files changed, commands run, failures, and remaining prerelease risks. Update
this work order and `plan.md` with the result.

## Result

- Selected `github.com/mark3labs/mcp-go v1.0.0-beta.1`; no newer compatible v1 release was available when checked.
- Added tests for modern discovery, direct modern list and call, legacy initialize/list/call, concurrent eras, invalid modern metadata, and modern cache hints.
- Kept one shared Streamable HTTP handler and legacy SSE handlers for every Kandev MCP profile.
- Added private, zero-lifetime cache hints for modern list and discovery results.
- Adapted the v1 general-result call hook and decoded v1 raw JSON tool arguments before schema validation.
- Verification passed: `go test -race ./internal/mcp/server` with the full package suite, plus `git diff --check` for the module and MCP server files.
- Remaining risk: the selected SDK is a beta release.
