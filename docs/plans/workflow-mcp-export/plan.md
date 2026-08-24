---
spec: docs/specs/integrations/requirements/external-mcp.md
created: 2026-08-19
status: done
---

# Implementation Plan: Workflow MCP Export

## Overview

Add one read-only MCP tool for a single workflow. Reuse the existing workflow export service and the current MCP JSON-text bridge.

The change adds no new export format or persistence path. A second task updates the public MCP reference after the contract is complete.

## Confirmed implementation path

`workflow.Service.ExportWorkflow` already builds the complete `WorkflowExport` document. The backend wires its provider to the scoped task service.

The MCP server already converts backend response objects to indented JSON text. `import_workflow_kandev` accepts that JSON text as its `document` argument.

This path gives a direct round trip for documents within the existing 1 MiB import limit. No new serializer is necessary.

## Backend

### WebSocket action and dispatcher

- `apps/backend/pkg/websocket/actions.go`: add `ActionMCPExportWorkflow = "mcp.export_workflow"` beside the workflow configuration actions.
- `apps/backend/internal/mcp/handlers/handlers.go`: register the action when the workflow service is available.

### Backend export handler

- `apps/backend/internal/mcp/handlers/config_workflow_handlers.go`: add `handleExportWorkflow`.
- Decode and require `workflow_id`.
- Call `h.workflowSvc.ExportWorkflow(ctx, workflowID)`.
- Return the `WorkflowExport` object through `ws.NewResponse`.
- Return a generic tool error for service errors. Do not include a partial document or internal error details.

### MCP tool and bridge

- `apps/backend/internal/mcp/server/config_handlers.go`: register `export_workflow_kandev` in the workflow configuration group.
- Define one required string argument named `workflow_id`.
- Add `exportWorkflowHandler` and forward the request through `ActionMCPExportWorkflow`.
- Keep the shared `forwardToBackend` conversion. It returns the export object as indented JSON MCP text.

### Agent configuration context

- `apps/backend/config/prompts/config-context.md`: list the export tool, its required argument, and its JSON result.
- State that the result can be the `document` input for `import_workflow_kandev`.

## Frontend

No frontend code changes are in scope. The existing static tool preview is not the live MCP contract.

## Public documentation

- `docs/public/automation-and-mcp.md`: add workflow export to the external MCP group and update the live tool count from 35 to 36.
- Keep the static-preview caveat. Add `export_workflow_kandev` to its list of omitted tools.
- `docs/public/coverage.json`: add `export_workflow_kandev` to the workflow MCP tool coverage list.

Primary content type: reference.

## Tests

- **What:** The tool is registered on configuration and external surfaces, but not task or Office surfaces.
  **File:** `apps/backend/internal/mcp/server/server_test.go`.
  **How:** Inspect the registered tool names and update the exact surface counts.
- **What:** The server requires `workflow_id`, sends `mcp.export_workflow`, and returns the backend envelope as JSON text.
  **File:** `apps/backend/internal/mcp/server/config_handlers_test.go`.
  **How:** Call the registered tool against the existing backend stub. Decode its MCP text as `WorkflowExport`.
- **What:** The backend exports one complete portable workflow and does not change source state.
  **File:** `apps/backend/internal/mcp/handlers/config_workflow_handlers_test.go`.
  **How:** Seed a workflow and steps through the existing test service. Call the backend handler and compare the portable fields.
- **What:** The exported MCP response can enter the existing import path unchanged.
  **File:** `apps/backend/internal/mcp/handlers/config_workflow_handlers_test.go`.
  **How:** Export one workflow, use its JSON payload as the import document for another workspace, and compare imported workflow behavior.
- **What:** Missing identifiers and export service errors return tool errors without documents.
  **Files:** `apps/backend/internal/mcp/server/config_handlers_test.go` and `apps/backend/internal/mcp/handlers/config_workflow_handlers_test.go`.
  **How:** Use missing arguments and an unknown workflow ID.
- **What:** The configuration context references a registered tool.
  **File:** `apps/backend/internal/mcp/server/sysprompt_sync_test.go`.
  **How:** Run the existing exact-name synchronization test after the context update.
- **What:** Public documentation remains structurally valid.
  **Files:** `docs/public/automation-and-mcp.md` and `docs/public/coverage.json`.
  **How:** Run both public-document validation scripts.

No browser E2E test is necessary. The feature changes an MCP contract and agent context, not rendered UI behavior.

## Verification Results

- `go test ./internal/mcp/handlers ./internal/mcp/server ./pkg/websocket`: passed, 696 tests across 3 packages.
- PR fixup: the annotation and whitespace-ID regression tests failed before the fix and passed after it.
- `go test ./internal/mcp/handlers ./internal/mcp/server ./pkg/websocket`: passed, 698 tests across 3 packages after fixup.
- `go test ./internal/sysprompt`: passed.
- `make -C apps/backend lint`: passed with 0 issues after fixup.
- `node --test scripts/validate-public-docs.test.mjs`: passed 61 tests with 0 failures.
- `node scripts/validate-public-docs.mjs`: validated 41 published docs pages.
- `git diff --check`: passed.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Expose workflow export through MCP](task-01-expose-workflow-export.md) - done.

Wave 2:

- [x] [Task 02: Document MCP workflow export](task-02-document-workflow-export.md) - done.

Execution is sequential in the primary conversation. No subagents are authorized.

## Risks and out of scope

- The import handler rejects documents larger than 1 MiB. This plan does not change that limit.
- The existing exporter owns field fidelity. The MCP handler must not duplicate export assembly or add a second serializer.
- The existing workflow provider owns authorization. The MCP handler must pass the scoped request context unchanged.
- A workspace-level batch export tool is out of scope.
- The static External MCP settings preview remains incomplete and non-authoritative.
