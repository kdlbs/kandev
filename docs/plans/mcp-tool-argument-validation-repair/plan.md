---
created: 2026-09-16
status: complete
requirements:
  - REQ-INTEGRATIONS-MCP-TOOL-ARGUMENT-VALIDATION-001
system_design:
  - ../../specs/integrations/system-design/mcp-tool-argument-validation.md
legacy_specs: []
---

# Implementation plan: MCP validation diagnostic repair

## Overview

Keep the shared MCP validation boundary, but make its diagnostic include all
unknown and missing properties when more than one schema failure occurs. This
repair keeps the existing redaction, task binding, schema, handler, and
backend contracts. The package records the maintainer change that repairs the
contributor PR.

## Scope

### In scope

- Traverse the complete JSON Schema validation failure tree.
- Group and sort unknown and required properties by their JSON instance path.
- Keep the session-task binding explanation when a rejected `task_id` appears.
- Add regressions for combined required and unknown-property failures in a
  closed nested object, multiple required-property branches, and a root
  unknown property paired with a nested primary failure.
- Prove that invalid calls do not dispatch and do not echo submitted values.

### Out of scope

- New MCP tools, schema fields, or transport messages.
- Changes to task-change-request authorization or backend actions.
- Frontend behavior, persistence, retries, or provider automation.

## Technical approach

`sanitizedToolArgumentError` keeps the first failure for the primary path and
keyword. Separate traversals visit every `ValidationError.Causes` node and
collect `kind.Required` and `kind.AdditionalProperties` entries by instance
path. The formatter sorts both dimensions and keeps an explicit `$` path when
the primary failure is nested. It adds one binding explanation for the two
session-bound task-change-request tools when a rejected property is `task_id`.

The existing `wrapHandler` path remains the only dispatch boundary. The
backend is not called for any validation error.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-INTEGRATIONS-MCP-TOOL-ARGUMENT-VALIDATION-001.1` and `.2` | `apps/backend/internal/mcp/server/tool_argument_validation_test.go` and `session_bound_task_tools_test.go` prove active schemas, combined required/unknown failures, and no backend dispatch. |
| `AC-INTEGRATIONS-MCP-TOOL-ARGUMENT-VALIDATION-001.3` | `tool_argument_validation_test.go` proves required and unknown names, root and nested paths, task binding, deterministic ordering, redaction, and multi-branch failures. |
| `AC-INTEGRATIONS-MCP-TOOL-ARGUMENT-VALIDATION-001.4` through `.6` | Existing parameterless, mode-change, and open-nested-map tests remain in `tool_argument_validation_test.go`. |
| `AC-INTEGRATIONS-MCP-TOOL-ARGUMENT-VALIDATION-001.7` and `.8` | Existing create-task normalization and handler tests retain the prompt alias and backend description contract. |

No browser E2E is needed because this repair changes MCP diagnostics and does
not change a rendered surface.

## Work orders

- [x] [Task 01: Preserve unknown arguments across validation failures](task-01-preserve-unknown-arguments.md) (`done`)

## Verification results

Completed on 2026-09-16:

- `go test ./internal/mcp/server -count=1` passed.
- `go test -race ./internal/mcp/server -count=1` passed.
- `go test ./internal/mcp/...` passed.
- Commit hooks passed, including Go formatting, changed-code lint, harness,
  architecture, commit-message, and copy checks.
- The maintainer commit was pushed over SSH to the contributor branch.
- The local documentation-coverage evaluator returned `covered` for the
  changed backend paths and this work order.
- `node --test .github/scripts/pr-docs.test.cjs` passed all 75 tests.
- `python3 scripts/list-docs.py validate` and both specification lint commands
  passed.
- `git diff --check` passed.

GitHub will rerun the documentation-coverage status for the new head. The
package now provides the changed work order and all linked contracts required
by that evaluator.

## Risks

- A validator library change can alter the order or shape of its failure tree.
  Stable sorting and focused regressions keep the caller-facing diagnostic
  deterministic.
- Traversal must inspect typed error nodes only. Generic validator text must
  not enter the response because it can contain rejected values.
