---
id: "02-host-api"
title: "Expose scoped Host and canvas reads"
status: pending
wave: 2
depends_on:
  - "01-transition-reads"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-WORKFLOW-HISTORY-001
  - REQ-PLUGINS-WORKFLOW-HISTORY-002
acceptance_criteria:
  - AC-PLUGINS-WORKFLOW-HISTORY-001.1
  - AC-PLUGINS-WORKFLOW-HISTORY-001.2
  - AC-PLUGINS-WORKFLOW-HISTORY-001.4
  - AC-PLUGINS-WORKFLOW-HISTORY-001.5
  - AC-PLUGINS-WORKFLOW-HISTORY-002.1
  - AC-PLUGINS-WORKFLOW-HISTORY-002.2
  - AC-PLUGINS-WORKFLOW-HISTORY-002.3
system_design:
  - ../../specs/plugins/system-design/workflow-transition-history.md
---

# Task 02: Expose scoped Host and canvas reads

## Summary

Add typed plugin Host reads and equivalent canvas browser routes for task
trails and workflow route groups. Enforce grants and scope before a ledger
query, then document the additive public contract.

## In scope

- Extend the v1 proto, generated stubs, SDK interfaces and mappings, Plugin
  Host adapter, and browser JSON routing.
- Reuse `api_read:tasks` for a scoped task trail. Require both task and
  workflow read grants plus workspace canvas scope for route groups.
- Test forbidden task/workflow IDs, task-scope summary denial, pagination,
  missing steps, null endpoints, and omission of actor/session identifiers.
- Update the embedded canvas authoring API reference and public plugin/canvas
  reference pages.

## Out of scope

- Canvas layout and publication, task writes, and new permission kinds.

## Acceptance

1. gRPC/SDK and browser reads expose the same ordered facts and bounded groups,
   with no unscoped or unpublished identifier leaked through an error.
2. Effective grants and current canvas scope gate every browser request;
   task scope can read only its own trail and cannot read route groups.
3. The authoring bundle and public docs name the new routes, fields, limits,
   permission requirements, and workspace-promotion boundary.

## Verification

```bash
make -C apps/backend proto
(cd apps/backend && go test ./internal/plugins ./pkg/pluginsdk ./internal/mcp/canvasskill)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/proto/kandev/plugin/v1/plugin.proto` and generated Go stubs
- `apps/backend/pkg/pluginsdk/host.go` and tests
- `apps/backend/internal/plugins/host_data.go`
- `apps/backend/internal/plugins/webapp_protocol.go`
- `apps/backend/internal/plugins/webapp_protocol_data.go` and tests
- `apps/backend/internal/mcp/canvasskill/files/references/browser-api.md`
- `docs/public/plugins-authoring.md`
- `docs/public/canvases.md`

## Dependencies

Task 01's task-service read methods.

## Risks

The Host gRPC API is additive-only in protocol v1. A browser route that checks
only a resource grant but misses canvas scope could expose other tasks' moves.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/workflow-transition-history.md)
- [System design](../../specs/plugins/system-design/workflow-transition-history.md)
- [ADR 0043](../../decisions/0043-plugin-host-data-api.md)

## Results

Pending.
