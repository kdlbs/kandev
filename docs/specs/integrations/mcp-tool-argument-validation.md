---
status: shipped
created: 2026-08-01
owner: kandev
---

# MCP Tool Argument Validation

Decision: [ADR-2026-08-01-validate-mcp-tool-arguments](../../decisions/2026-08-01-validate-mcp-tool-arguments.md)

## Why

Agents and external MCP clients need a failed tool call to be distinguishable from a successful operation and specific enough to repair safely. Silently ignoring an incorrect argument can create tasks, start agents, or mutate configuration without the caller's intended data, while a generic constraint failure can leave an agent repeating the same malformed call.

## What

- Every Kandev MCP tool validates its arguments against the schema registered for its current MCP mode before running its handler.
- Missing required arguments, values of the wrong declared type, violated declared constraints, and unknown top-level arguments return a tool error and cause no handler or backend side effect.
- A missing-required-property error names the missing schema property or properties as well as the invalid object location and `required` constraint, without returning argument values.
- Parameterless tools accept omitted arguments or an empty object and reject any supplied field.
- Validation follows mode changes: after the server changes its registered tool set, calls use the replacement tools' schemas.
- Nested configuration objects remain open when their schema intentionally permits arbitrary keys.
- `create_task_kandev` advertises `prompt` for the text delivered to a newly started agent. It accepts the former `description` name as an unadvertised compatibility alias, without adding a second field or explanation to the tool schema. A call containing both names fails rather than choosing one silently.
- The backend task action continues receiving the text in its existing `description` field; this naming convergence changes only the MCP boundary.
- Task-mode agent context lists the walkthrough tools and states that every `show_walkthrough_kandev.steps` item requires `file`, `line`, and `text`. The built-in `changes-walkthrough` request repeats that load-bearing call shape so callers do not depend on a client rendering nested JSON Schema correctly.
- On startup, a stored built-in `changes-walkthrough` prompt that exactly matches the loader-normalized content of an untouched shipped revision is refreshed to the current embedded prompt. User-edited built-ins, unrecognized stored content, and user-owned prompts remain unchanged.

## API surface

The advertised MCP `tools/list` schemas do not gain repeated unknown-argument metadata or the legacy `create_task_kandev.description` compatibility alias. The server applies these rules when handling `tools/call`:

1. Normalize an explicitly supported compatibility alias, if any.
2. Validate the resulting arguments against the currently registered tool schema with the root object closed to unknown fields.
3. Invoke the handler only when validation succeeds.

Validation failures are returned as MCP tool error results. They identify the invalid argument location and violated constraint without returning sensitive argument values. For the `required` constraint, the result also identifies each missing property by its schema-defined name.

## Failure modes

- If tool arguments fail validation, Kandev returns an error and does not invoke the handler.
- If a registered tool schema cannot compile, calls to that tool fail closed and Kandev logs the schema defect; test coverage prevents shipping an uncompilable built-in schema.
- If both `prompt` and compatibility alias `description` are supplied to `create_task_kandev`, Kandev returns an error and creates no task.
- Missing-property diagnostics expose only names declared by the registered schema, never sibling values or rejected argument contents.
- Prompt refresh recognizes only exact stored content whose hash matches a loader-normalized historical built-in revision and that has never been saved by a user; unknown or edited content is left intact.
- Validation errors do not echo complete prompts, credentials, configuration values, or other argument contents.

## Scenarios

- **GIVEN** any registered Kandev MCP tool, **WHEN** a caller omits one or more schema-required properties, **THEN** the call returns a tool error naming the invalid object location, the `required` constraint, and every missing schema property, and its handler is not invoked.
- **GIVEN** any registered Kandev MCP tool, **WHEN** a caller supplies a value with the wrong schema type, **THEN** the call returns a tool error and its handler is not invoked.
- **GIVEN** any registered Kandev MCP tool, **WHEN** a caller supplies an unknown top-level argument, **THEN** the call returns a tool error naming its location and its handler is not invoked.
- **GIVEN** a parameterless tool, **WHEN** a caller omits arguments or supplies `{}`, **THEN** the handler runs normally; **WHEN** it supplies any field, **THEN** the call fails.
- **GIVEN** a tool with an intentionally open nested configuration map, **WHEN** a caller supplies an arbitrary key inside that map, **THEN** validation accepts the nested key.
- **GIVEN** the server changes MCP mode, **WHEN** a caller invokes a tool in the replacement set, **THEN** validation uses that tool's replacement schema.
- **GIVEN** a caller supplies advertised `prompt` to `create_task_kandev`, **WHEN** validation runs, **THEN** the prompt is forwarded unchanged through the existing backend `description` field.
- **GIVEN** a caller supplies only legacy `description` to `create_task_kandev`, **WHEN** validation runs, **THEN** the value is accepted as `prompt` and forwarded unchanged.
- **GIVEN** a caller supplies both `prompt` and `description` to `create_task_kandev`, **WHEN** validation runs, **THEN** the call returns an error and creates no task.
- **GIVEN** a task-mode agent receives Kandev's first-turn context or the built-in `changes-walkthrough` request, **WHEN** it prepares `show_walkthrough_kandev`, **THEN** the instructions identify `steps` as an ordered array whose items require `file`, `line`, and `text`; the first-turn context also names the show, get, and delete walkthrough tools.
- **GIVEN** an existing installation with an untouched historical built-in `changes-walkthrough` prompt stored through the embedded loader, **WHEN** Kandev starts with a newer embedded prompt, **THEN** the stored built-in is refreshed; unrecognized content, a user-edited built-in, or a user-owned prompt is preserved.

## Out of scope

- Advertising the legacy `description` compatibility alias alongside `prompt`.
- Changing the semantic business rules enforced inside individual handlers and services.
- Closing arbitrary-key nested configuration maps that are intentionally open.
- Changing MCP transport, authorization, tool availability by mode, or backend action payloads.
- Adding automatic tool-call retries or agent-client-specific schema rewriting.
- Duplicating every registered tool schema in the first-turn task context.
