---
id: "02-plugin-catalog-invocation"
title: "Plugin catalog and invocation"
status: done
wave: 2
depends_on: ["01-manifest-sdk-contract"]
plan: "plan.md"
spec: "../../specs/plugins/requirements/agent-tools.md"
adr: "../../decisions/2026-08-11-plugin-tools-through-kandev-mcp.md"
---

# Task 02: Plugin catalog and invocation

## Acceptance

- The plugin service produces sorted, active-only, generation/revision catalog
  snapshots and rejects install/upgrade collisions without replacing the prior
  installation.
- `InvokeAgentTool` revalidates current state, declaration, surface, input, and
  result; enforces deadline/size bounds; calls the plugin once; and logs only
  safe metadata.
- Every effective lifecycle change emits a nonblocking catalog-change signal;
  equivalent changes do not advance the revision or emit duplicate work.

## Verification

```bash
cd apps/backend && go test -race ./internal/plugins/...
```

## Files likely touched

- `apps/backend/internal/plugins/agent_tools.go`
- `apps/backend/internal/plugins/agent_tools_test.go`
- `apps/backend/internal/plugins/invoke.go`
- `apps/backend/internal/plugins/service.go`
- `apps/backend/internal/plugins/service_test.go`
- `apps/backend/internal/plugins/types.go`
- `apps/backend/internal/plugins/provider.go`

## Dependencies

Task 01.

## Parallelism

Parallel-safe with Task 03 after Task 01. This task owns plugin service/catalog
files; Task 03 owns MCP server and agentctl files.

## Inputs

- Spec sections `Runtime Catalog`, `Permissions`, `Failure Modes`, and
  `Persistence Guarantees`
- Existing `InvokeWebhook`, install transaction, status transition, runtime
  supervision callback, and `notifyDeliverer` patterns

## Risks

- Runtime health callbacks must not block while catalog refresh reaches remote
  agentctl instances.
- Tool calls are not retry-safe; no existing event-delivery retry helper may be
  reused for invocation.

## Output contract

Report catalog ordering/revision rules, collision transaction behavior,
invocation bounds, safe logging fields, commands and results, and task/plan
status updates.

## Results

- Added the active-only catalog builder with stable generation/revision and
  readable plugin-ID-derived exposed names, with bounded hash fallback for long
  IDs.
- Added install collision checks, lifecycle refresh signaling, argument and
  result schema validation, 30-second deadlines, and the 1 MiB result cap.
- Verification: `go test -race ./internal/plugins/...` passed.
