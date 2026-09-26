---
id: "02-backend-mcp-contract"
title: "Backend MCP contract"
status: done
wave: 2
depends_on: ["01-record-contract"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-rich-output.md"
---

# Task 02: Backend MCP contract

## Acceptance

1. Task and Office MCP profiles expose a strictly validated
   `show_rich_output_kandev`; configuration and external profiles do not.
2. Version, union shape, count, size, numeric, and relative-path limits fail
   closed with clear tool errors.
3. Task and Office prompts list the exact tool and instruct agents to prefer
   plain text or Markdown when rich output adds no value.

## Verification

```sh
cd apps/backend && go test ./internal/mcp/server/...
```

## Files likely touched

- `apps/backend/internal/mcp/server/rich_output.go`
- `apps/backend/internal/mcp/server/rich_output_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/config/prompts/kandev-context.md`
- `apps/backend/config/prompts/office-context.md`

## Dependencies

Task 01.

## Parallelism

Sequential. This creates the schema consumed by frontend tasks.

## Inputs

- Spec API surface, permissions, and failure modes.
- Existing `profileToolGroups`, `wrapHandler`, and Draft 7 validator patterns.

## Output contract

Report RED and GREEN evidence, files changed, exact test result, trust-boundary
notes, and task/plan status update.

## Results

- RED: task/Office registration, semantic chart lengths, unsafe paths, and the
  64 KiB cap each failed before its implementation. The full package then
  exposed stale tool-count and prompt inventories.
- GREEN: `go test ./internal/mcp/server/...` passed.
- Added a closed Draft 7 schema, semantic validation, task/Office-only profile
  registration, exact prompt entries, and fail-closed contract coverage.
- File references accept only task-workspace-relative paths. The handler stores
  no bytes and performs no file or backend mutation.
