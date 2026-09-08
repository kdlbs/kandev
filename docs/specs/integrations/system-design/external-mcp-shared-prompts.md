---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-EXTERNAL-MCP-002
---

# External MCP Saved Prompt Reads System Design

## Purpose and boundaries

The integration system owns the tool contract for configuration and external
MCP clients. The prompts package remains the source of saved prompt data and
reference expansion.

This design adds read access to the existing saved prompt collection. It does
not add storage, mutation tools, or automatic reference expansion.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-EXTERNAL-MCP-002` | [Tool contract](#tool-contract), [Control flow](#control-flow), [Security](#security) |

## Components and responsibilities

- `internal/prompts/service` reads saved prompts through the current repository.
  A name lookup trims the input and maps a missing row to
  `ErrPromptNotFound`.
- `internal/mcp/handlers` owns the backend actions. A narrow prompt-reader
  dependency supplies list and name lookup operations.
- `internal/mcp/server` registers the tools for configuration and external MCP
  surfaces. Its handlers forward requests through the existing backend bridge.
- `pkg/websocket/actions.go` owns the two action names used by both MCP
  transports.
- `config/prompts/config-context.md` tells the configuration agent when and how
  to use the tools.

## Tool contract

### `list_shared_prompts_kandev`

The tool has no input fields. It returns this JSON text shape:

```json
{
  "shared_prompts": [
    {
      "name": "code-review",
      "builtin": true,
      "content_bytes": 1234
    }
  ],
  "total": 1
}
```

The result keeps the repository order. Built-in prompts come first. Names are
then in ascending order. The summary does not include prompt content.

### `get_shared_prompt_kandev`

The tool requires one non-empty `name` string. The handler trims surrounding
space before lookup. Names remain exact and case-sensitive.

The result contains `name`, `content`, `builtin`, `content_bytes`,
`created_at`, and `updated_at`. The tool does not return a prompt ID because
the read contract uses the prompt name.

Both tools use the MCP read-only, non-destructive, idempotent, and closed-world
annotations.

## Control flow

1. The MCP server validates the tool input.
2. The server sends `mcp.list_shared_prompts` or `mcp.get_shared_prompt` to the
   backend dispatcher.
3. The backend handler calls the prompt-reader dependency with the same
   request context.
4. The list handler maps models to summaries and omits content.
5. The get handler maps one model to the full read result.
6. The MCP bridge returns the backend object as indented JSON text.

The existing prompt resolver does not participate in this flow. Workflow-step
list output also remains unchanged.

## Failure behavior

The MCP server rejects a missing or blank `name` before backend lookup. The
backend returns a tool error for an unknown name or a repository error. Error
results do not include prompt content or internal database details.

## Security

The tools exist only on configuration and external MCP surfaces. Existing MCP
transport authentication and request-context propagation remain the security
boundary.

Saved prompts are instance-wide configuration today. The tools return the same
collection that the authenticated caller sees in Settings > Prompts. This
change does not add workspace-level or user-level prompt ownership.

## Compatibility

No database migration is necessary. Existing HTTP prompt routes, Settings UI,
and `@name` expansion keep their current contracts.

The public MCP reference adds the two tools. The external tool count changes
from 40 to 42.

## Verification strategy

- Prompt-service tests cover trimmed lookup and missing-name translation.
- Backend-handler tests cover summaries, full reads, missing names, and errors.
- MCP-server tests cover schemas, annotations, forwarding, and surface rules.
- Prompt-context tests keep documented tool names synchronized with the live
  catalog.
- Public documentation validation covers the updated MCP reference and coverage
  map.
