---
spec: docs/specs/integrations/requirements/mcp-tool-argument-validation.md
created: 2026-08-01
status: implemented
---

# Implementation Plan: MCP Tool Argument Validation

## Overview

Compile every registered Kandev MCP input schema into a server-owned validator, close only its root argument object, and run validation in the shared `wrapHandler` boundary before any tool-specific code. Converge `create_task_kandev` on the advertised `prompt` name while retaining unadvertised `description` compatibility, and document the fail-closed caller behavior. This covers all current modes and automatically protects tools registered in the future through the standard server path.

## Confirmed root cause

`mcp-go` v0.43.2 unmarshals `tools/call` and invokes the selected handler without validating `CallToolRequest.Params.Arguments` against the tool's registered JSON Schema. Kandev's handlers then use permissive getters such as `GetString`, which return defaults for absent or mistyped values and never inspect unknown keys. The issue #2123 reproduction therefore returned success after discarding the detailed `prompt` supplied to `create_task_kandev`.

All 65 built-in tool registrations use `Server.wrapHandler`, which is the shared insertion point across task, configuration, external, and Office modes. Tool sets can be replaced by `SetMode`, so validator compilation must run after every registration pass rather than only in the constructor.

## Backend

### Shared schema compilation and validation

- Add `github.com/santhosh-tekuri/jsonschema/v6` v6.0.2 as a direct backend dependency and use its compiled-schema API rather than implementing a partial JSON Schema interpreter.
- Add `apps/backend/internal/mcp/server/tool_argument_validation.go` to marshal either `Tool.RawInputSchema` or `Tool.InputSchema`, inject `additionalProperties: false` only at the root object, compile one validator per active tool, and retain a fail-closed compile error for any invalid schema.
- Extend `Server` with the active compiled validator set. Rebuild it at the end of `registerTools`, which covers initial construction and `SetMode` replacement.
- Normalize the create-task legacy alias when needed and validate arguments in `wrapHandler` before invoking the wrapped handler. Treat omitted arguments as an empty object, return a sanitized MCP tool error on failure, and preserve existing call-duration/error logging.
- Keep nested object schemas unchanged so open configuration maps continue accepting arbitrary keys.

### Create-task parameter convergence

- Advertise `prompt` as the single create-task field for text delivered to the started agent.
- Add a small pre-validation normalization path for `create_task_kandev`: shallow-copy unadvertised legacy `description` to `prompt` and remove `description` before schema validation. Avoid copying ordinary calls.
- Reject calls that supply both keys before validation. Do not advertise both names.
- Forward canonical `prompt` through the existing backend `description` payload without changing stored models or WebSocket actions.

## Public documentation

- Update `docs/public/automation-and-mcp.md` with the cross-cutting rule that Kandev validates required fields, types, constraints, and unknown top-level fields before a tool runs.
- Document `prompt` as the canonical create-task input and explain legacy `description` migration outside the token-bearing tool schema.

## Tests

- **What:** every built-in tool schema in every MCP mode compiles after root closure.
  - **File:** `apps/backend/internal/mcp/server/tool_argument_validation_test.go`
  - **How:** construct task, config, external, and Office servers, enumerate their active tools, and assert each has a compiled validator without an error.
- **What:** unknown, missing-required, wrong-type, enum/constraint, and parameterless-tool calls fail before handler invocation, while valid calls and intentionally open nested maps pass.
  - **File:** `apps/backend/internal/mcp/server/tool_argument_validation_test.go`
  - **How:** register focused test tools through the same wrapper, capture handler invocation, and exercise representative typed and raw schemas.
- **What:** `SetMode` rebuilds validators for the replacement tool set.
  - **File:** `apps/backend/internal/mcp/server/tool_argument_validation_test.go`
  - **How:** change a server mode and validate calls against a tool schema available only in the replacement mode.
- **What:** `create_task_kandev` advertises canonical `prompt`, accepts hidden legacy `description`, rejects both, and rejects every other unknown key without backend dispatch.
  - **File:** `apps/backend/internal/mcp/server/handlers_test.go`
  - **How:** focused handler-through-wrapper calls with `testBackend` payload capture.

No browser E2E is needed because this changes the MCP protocol boundary without changing UI behavior.

## Implementation Waves And Parallel Candidates

Execution is sequential in the primary conversation because the tasks share the MCP wrapper and validation contract.

- [x] [Task 01: Enforce registered schemas at the MCP boundary](task-01-enforce-registered-schemas.md) — done
- [x] [Task 02: Converge create-task on prompt](task-02-create-task-prompt-compatibility.md) — done
- [x] [Task 03: Document fail-closed MCP arguments](task-03-document-mcp-validation.md) — done

## Validation commands

- `make -C apps/backend test`
- `make -C apps/backend lint`
- `make -C apps/backend build`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`

## Risks and boundaries

- Existing schemas have historically been descriptive rather than enforced. The all-schema compilation test and focused valid-call cases must expose accidental mismatches before rollout.
- Only the root argument object is closed. Recursively closing nested objects would break intentional arbitrary-key configuration inputs.
- Validator error text must be sanitized and stable enough to guide callers without echoing sensitive values.
- `registerTools` is the validator rebuild boundary; future dynamic registration outside that path must deliberately rebuild or use a new shared registration helper.

## Validation results

- `make -C apps/backend test` — passed across all backend packages.
- `make -C apps/backend lint` — passed with no issues.
- `make -C apps/backend build` — passed for the backend and local/remote helper binaries; Darwin helpers emitted the expected local no-codesigner warnings.
- Focused MCP server tests — 167 passed under the race detector; the deterministic mode-change test passed 100 race-enabled repetitions.
- CI-style changed-file lint against `bb525f1894803d150fb1bfb6a98f116a38ef3d3b` — no issues.
- `node --test scripts/validate-public-docs.test.mjs` — 58 passed.
- `node scripts/validate-public-docs.mjs` — validated 41 published pages.
