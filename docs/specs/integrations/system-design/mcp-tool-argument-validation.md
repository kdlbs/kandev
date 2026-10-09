---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-MCP-TOOL-ARGUMENT-VALIDATION-001
created: 2026-09-16
owners:
  - kandev
---

# MCP tool argument validation system design

## Purpose and boundaries

The integration system owns the argument contract for Kandev MCP tools. The
managed MCP server validates a call against the schema that is active for the
current surface before it invokes a handler.

Handlers and backend services remain the owners of business rules, task state,
provider state, and authorization. This design does not add a second request
protocol, a frontend transport, or persistent validation state.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-MCP-TOOL-ARGUMENT-VALIDATION-001` | [Components and responsibilities](#components-and-responsibilities), [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `apps/backend/internal/mcp/server.Server` owns the active MCP tool registry
  and the in-memory validator registry.
- `registerTools` registers each built-in tool through the shared wrapper. At
  the end of registration, `rebuildToolArgumentValidators` compiles the
  schemas in the active registry.
- `compileToolArgumentSchema` uses the registered raw or typed JSON Schema.
  It closes only the root argument object with `additionalProperties: false`.
  Nested objects keep the behavior declared by their own schemas.
- `validateToolArguments` normalizes omitted arguments and the supported
  `create_task_kandev` compatibility alias, then validates the request while
  holding the mode and validator read barriers.
- `wrapHandlerWithArgumentLogging` converts a validation error to an MCP tool
  error. It invokes the wrapped handler only after validation succeeds.
- MCP handlers and the backend dispatcher keep their existing action names and
  domain validation. They receive no request that failed the shared boundary.

`SetMode`, `SetProviders`, `SetProfile`, and plugin-tool replacement all rebuild
the tool registry. Their assembly path runs the same registration and
validator compilation code, so a replacement catalog cannot use stale schemas.

## Data and contracts

The server keeps one `map[string]toolArgumentValidator` under
`validatorMu`. Each entry contains the compiled schema or a compile error.
Compile errors are retained per tool so a broken schema fails closed without
changing the server constructor contract.

The registered `mcp.Tool` schema is the source of truth for required fields,
types, constraints, arrays, objects, and enums. Calls with no arguments use an
empty object. A parameterless schema therefore accepts omitted arguments or
`{}` and rejects any field at the closed root.

Validation errors use a stable, redacted shape:

```text
invalid arguments for <tool>: validation failed at <path> (keyword: <keyword>; missing: "<name>"; unknown arguments: "<name>" at <path>)
```

The formatter selects a primary failure for the path and keyword. It then
walks every validation cause, groups `additionalProperties` failures by their
JSON instance path, and sorts paths and property names. It never formats the
rejected values. For the two session-bound task change-request tools, an
unknown `task_id` at any rejected path also adds the existing rule that the
tool is bound to the calling task.

`create_task_kandev` advertises `prompt`. Before validation, a call with only
the supported legacy `description` key is copied to `prompt` in a shallow
request copy. A call with both names fails. The handler continues to send the
text through the backend action's existing `description` field.

## Control flow

### Tool registration

1. The server assembles the tool set for its MCP surface and capabilities.
2. Each tool is registered with `wrapHandler` or the sensitive variant.
3. `rebuildToolArgumentValidators` compiles the complete active catalog.
4. A compile failure is logged and stored in that tool's validator entry.

### Tool call

1. An MCP `tools/call` request enters `wrapHandlerWithArgumentLogging`.
2. `validateToolArguments` decodes raw arguments, uses `{}` when omitted, and
   applies only the established create-task alias normalization.
3. The validator for the current tool checks the normalized object.
4. On failure, the wrapper returns an MCP error result and does not call the
   handler or backend dispatcher.
5. On success, the wrapper passes the normalized request to the existing
   handler and preserves the current result and duration logging.

The server read lock prevents a mode or catalog replacement from completing
between validator selection and validation. A replacement waits for an active
validation call, then publishes its new tools and matching validator map.

## Failure and recovery

- A malformed JSON argument, missing field, wrong type, constraint violation,
  or unknown root field returns an MCP tool error before dispatch.
- A nested arbitrary-key map remains open when its registered schema permits
  arbitrary keys. Root closure does not recursively change that contract.
- A schema compile failure logs the tool name and fails calls to that tool
  closed with a generic schema-invalid error.
- Required-property diagnostics include every missing schema name found in the
  selected failure and retain the invalid object path. Unknown-property
  diagnostics include all rejected properties found in the validation tree.
- A validation error does not retry the call and does not echo secrets,
  prompts, identifiers, or other submitted values.
- A mode or provider change uses the replacement catalog on the next call;
  stale tool names remain subject to the existing handler and authorization
  rules.

## Persistence

Validator entries live only in the MCP server process. No database table,
workspace record, task field, or WebSocket event stores validation state.
Argument values continue to follow the existing handler and backend ownership
rules only after validation succeeds.

## Security

Root unknown-field rejection prevents callers from injecting unadvertised task,
session, provider, or identity fields into handlers. The session-bound task
rule is explanatory only; backend principal and workspace checks remain the
authorization boundary.

Validation diagnostics expose schema names and paths, not rejected values.
Sensitive tool calls continue to use the existing logging suppression wrapper.

## Observability

Schema compilation failures are logged with the tool name. Existing MCP call,
error, result, and duration logs remain in the wrapper. Focused server tests
cover the error contract, dispatch suppression, redaction, nested paths, and
mode replacement. The active-catalog test compiles every built-in tool in each
supported MCP mode.

## Related decisions

- [ADR-2026-08-01: Validate MCP tool arguments at the shared server boundary](../../../decisions/2026-08-01-validate-mcp-tool-arguments.md)
