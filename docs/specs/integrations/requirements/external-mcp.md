---
status: draft
system: integrations
created: 2026-04-28
owners:
  - tbd
---
# External MCP Endpoint Requirements

## Overview

Users want to operate on Kandev workspaces, workflows, agents, and tasks from coding agents (Claude Code, Cursor, Codex, …) running outside Kandev. Today the Kandev MCP is embedded inside each `agentctl` instance, scoped to a single session, and reachable only on a container-internal `localhost` port — there is no way to add Kandev as an MCP server in an external agent's global config.

## Requirements

### REQ-INTEGRATIONS-EXTERNAL-MCP-001: External MCP Endpoint

**Intent:** Users want to operate on Kandev workspaces, workflows, agents, and tasks from coding agents (Claude Code, Cursor, Codex, …) running outside Kandev. Today the Kandev MCP is embedded inside each `agentctl` instance, scoped to a single session, and reachable only on a container-internal `localhost` port — there is no way to add Kandev as an MCP server in an external agent's global config.

#### Acceptance criteria

- **AC-INTEGRATIONS-EXTERNAL-MCP-001.1:** The Kandev backend exposes an MCP server on its existing HTTP port (default `38429`). The MCP routes are reachable from any network that can reach the Kandev backend.
- **AC-INTEGRATIONS-EXTERNAL-MCP-001.2:** Users register Kandev as an MCP server in their external coding agent using one of:
- **AC-INTEGRATIONS-EXTERNAL-MCP-001.3:** Streamable HTTP: `http://localhost:38429/mcp`
- **AC-INTEGRATIONS-EXTERNAL-MCP-001.4:** SSE: `http://localhost:38429/mcp/sse`
- **AC-INTEGRATIONS-EXTERNAL-MCP-001.5:** The external endpoint exposes the **config-mode tool surface** plus `create_task_kandev` (no plan tools, no `ask_user_question_kandev`).
- **AC-INTEGRATIONS-EXTERNAL-MCP-001.6:** The endpoint requires a personal access token when Kandev authentication is enabled. It remains open when authentication is disabled.
- **AC-INTEGRATIONS-EXTERNAL-MCP-001.7:** The Settings UI shows the URL and ready-to-paste config snippets for popular agents.
- **AC-INTEGRATIONS-EXTERNAL-MCP-001.8:** The existing per-`agentctl` MCP behavior is unchanged — the same tool definitions back both endpoints.

### REQ-INTEGRATIONS-EXTERNAL-MCP-002: Saved prompt reads

**Intent:** Configuration agents need the saved prompt content that workflow
steps reference. This content lets an agent audit the complete effective prompt
without operator copy and paste.

#### Acceptance criteria

- **AC-INTEGRATIONS-EXTERNAL-MCP-002.1:** When a configuration or external MCP
  client lists tools, the catalog shall include `list_shared_prompts_kandev`
  and `get_shared_prompt_kandev`.
- **AC-INTEGRATIONS-EXTERNAL-MCP-002.2:** When a client calls
  `list_shared_prompts_kandev`, the result shall enumerate the saved prompts
  without their content.
- **AC-INTEGRATIONS-EXTERNAL-MCP-002.3:** Each prompt summary shall include the
  exact name, built-in status, and content size in bytes.
- **AC-INTEGRATIONS-EXTERNAL-MCP-002.4:** When a client calls
  `get_shared_prompt_kandev` with an exact saved prompt name, the result shall
  include the full content and prompt metadata.
- **AC-INTEGRATIONS-EXTERNAL-MCP-002.5:** When the name is empty or unknown, the
  tool shall return an error without prompt content.
- **AC-INTEGRATIONS-EXTERNAL-MCP-002.6:** The tools shall use the authenticated
  MCP request context and the saved prompt collection for that Kandev instance.
- **AC-INTEGRATIONS-EXTERNAL-MCP-002.7:** Task, Office, and automation MCP
  catalogs shall not include the saved prompt tools.
- **AC-INTEGRATIONS-EXTERNAL-MCP-002.8:** Saved prompt reads shall not change
  prompt content or the current `@name` expansion behavior.

## Migrated source detail

## Why

Users want to operate on Kandev workspaces, workflows, agents, and tasks from coding agents (Claude Code, Cursor, Codex, …) running outside Kandev. Today the Kandev MCP is embedded inside each `agentctl` instance, scoped to a single session, and reachable only on a container-internal `localhost` port — there is no way to add Kandev as an MCP server in an external agent's global config.

## What

- The Kandev backend exposes an MCP server on its existing HTTP port (default `38429`). The MCP routes are reachable from any network that can reach the Kandev backend.
- Users register Kandev as an MCP server in their external coding agent using one of:
  - Streamable HTTP: `http://localhost:38429/mcp`
  - SSE: `http://localhost:38429/mcp/sse`
- The external endpoint exposes the **config-mode tool surface** plus `create_task_kandev` (no plan tools, no `ask_user_question_kandev`).
- The endpoint requires a personal access token when Kandev authentication is enabled. It remains open when authentication is disabled.
- The Settings UI shows the URL and ready-to-paste config snippets for popular agents.
- The existing per-`agentctl` MCP behavior is unchanged — the same tool definitions back both endpoints.
- The configuration and external tool surfaces expose `export_workflow_kandev`.
- `export_workflow_kandev` requires one `workflow_id` and returns a JSON `WorkflowExport` document as MCP text content.
- The document uses `version: 1` and `type: kandev_workflow`. It contains exactly one portable workflow and all of its steps.
- The document omits instance IDs and timestamps. It keeps portable profile data and position-based step references from the existing export service.
- A caller can pass the returned text unchanged as `document` to `import_workflow_kandev` when the document is within the import limit.
- Workflow export is read-only. It does not change the workflow, its steps, or its workspace.

## API surface

### `export_workflow_kandev`

Available surfaces: configuration MCP and external MCP.

Input:

```json
{"workflow_id":"workflow-uuid"}
```

Success result: MCP text content that contains an indented JSON `WorkflowExport` document.

The output uses the existing portable workflow contract. The tool does not define a second export format.

## Permissions

The tool uses the caller identity that Kandev attaches to the MCP request. The existing workflow service authorizes the workflow through its workspace.

The tool does not return a workflow from another user's workspace. Authentication-disabled installations keep their current unscoped, single-user behavior.

## Failure modes

- A missing or empty `workflow_id` returns a tool error before backend export work starts.
- An unknown or inaccessible workflow returns a tool error and no partial document.
- A workflow or step read error returns a tool error and no partial document.
- `import_workflow_kandev` keeps its existing 1 MiB document limit. This feature does not add an export limit or change that import limit.

## Scenarios

- **GIVEN** a running Kandev backend, **WHEN** a user adds `http://localhost:38429/mcp` to their Claude Code MCP config and asks "list my Kandev workspaces", **THEN** Claude Code calls `list_workspaces_kandev` and returns the workspaces that appear in the Kandev UI.
- **GIVEN** the user is in a Cursor session outside Kandev, **WHEN** they ask Cursor to "create a task to fix the login bug in workspace X", **THEN** the task appears in the Kandev kanban board.
- **GIVEN** the user opens Kandev Settings → External MCP, **WHEN** they click "Copy Claude Code config", **THEN** they receive a JSON snippet they can paste into `~/.claude.json`.
- **GIVEN** an external agent calls a session-scoped tool that does not exist on this endpoint, **THEN** the call returns a tool-not-found error and the rest of the session is unaffected.
- **GIVEN** a caller can read a workflow, **WHEN** it calls `export_workflow_kandev` with that workflow ID, **THEN** it receives one valid portable workflow document with all workflow steps.
- **GIVEN** an exported document is within 1 MiB, **WHEN** the caller passes the result unchanged to `import_workflow_kandev`, **THEN** import accepts the document and applies existing deduplication rules.
- **GIVEN** a caller cannot read a workflow, **WHEN** it calls `export_workflow_kandev` with that workflow ID, **THEN** the call returns an error without workflow data.
- **GIVEN** a task or Office MCP surface, **WHEN** the client lists tools, **THEN** `export_workflow_kandev` is absent.

## Out of scope

- New authentication methods or changes to personal access tokens.
- Session-scoped tools (`create_task_plan_kandev`, `ask_user_question_kandev`, plan get/update/delete).
- Per-workspace scoping of the endpoint.
- Exposing `agentctl`'s per-session MCP externally — architectural mismatch (per-session, ephemeral, no auth boundary).
- A workspace-level `export_workflows_kandev` batch tool.
- Changes to the portable workflow format or the 1 MiB import limit.
- Changes to the existing HTTP export endpoints or workflow export UI.
- Saved prompt create, update, or delete tools over MCP.
- Automatic expansion of saved prompt references in workflow-step list results.
- Changes to saved prompt persistence, reference matching, or expansion depth.

## Implementation plan

See [Workflow MCP export](../../../plans/workflow-mcp-export/plan.md).
