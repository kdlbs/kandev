---
created: 2026-09-08
status: complete
requirements:
  - REQ-AGENTS-MCP-DISCOVERY-001
  - REQ-AGENTS-MCP-DISCOVERY-002
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-009
system_design:
  - ../../specs/agents/system-design/mcp-tool-discovery-guidance.md
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
legacy_specs: []
---

# Implementation plan: MCP discovery and canvas prompts

## Overview

Make Kandev tool discovery explicit, reduce repeated tool descriptions, and
direct canvas requests through draft creation and publication.

The failed task `efb43c76-a2ab-44cf-9169-b38fb5e3da9f` created a standalone app.
Its conversation contained no canvas calls. Its original MCP server exposed
seven canvas tools, but its injected task prompt omitted all canvas guidance.
The canvas list was empty. These observations motivate the prompt changes.

Agents owns the shared discovery contract because it spans task operations and
clients. Canvases owns its creation preset and authoring outcome. The paired
specifications link those independent contracts without duplicating ownership.

## Scope

### In scope

- Conditional client-native discovery with an available-catalog fallback.
- A compact task prompt with essential workflow and authorization rules.
- Canvas instructions derived from the resolved session MCP profile.
- A stronger localized Create canvas preset in the existing task dialog.
- Targeted prompt, producer, catalog, desktop, and mobile verification.

### Out of scope

- MCP transport, permission, scope, promotion, or runtime flag changes.
- A new tool-search API or provider-specific discovery adapter.
- Office/configuration prompt compaction or a new canvas editor.
- Repairing the previous task, publishing its files, or contacting its agent.

## Technical approach

Task 01 changes `config/prompts/kandev-context.md` and `internal/sysprompt`.
It preserves critical dynamic sections and replaces routine tool listings with
discovery guidance. It wires the canvas option through all existing task
prompt producers using the executor's resolved MCP profile.

Task 02 changes the localized canvas preset. `CanvasTaskCreateLauncher` keeps
the normal task dialog and scratch/local defaults. It adds tests against real
catalog text, because the current component test substitutes a mock sentence.
The mobile E2E currently replaces the preset with explicit MCP commands, so
that test alone cannot detect weak authoring guidance.

The system designs contain the proposed discovery and canvas wording. The
implementation wires the capability projection through task launch, prepared
session launch, workflow context reset, and the first direct message path.

## Tests

| Criteria | Evidence |
| --- | --- |
| `AC-AGENTS-MCP-DISCOVERY-001.1` through `.6` | New focused sysprompt discovery tests. |
| `AC-AGENTS-MCP-DISCOVERY-002.1`, `.2`, `.5` | Compact size and preserved workflow assertions. |
| `AC-AGENTS-MCP-DISCOVERY-002.3`, `.4` | Producer tests for start, prepared start, and reset; MCP inventory tests. |
| `AC-CANVASES-AGENT-WEB-APPS-001.10` through `.12` | Gated authoring instruction tests and negative profile cases. |
| `AC-CANVASES-AGENT-WEB-APPS-009.6`, `.8` | Real locale preset contract tests with exact tool identifiers. |
| `AC-CANVASES-AGENT-WEB-APPS-009.7` | Desktop/mobile dialog and submitted-description tests. |

## E2E tests

Task 02 extends the guided creation case in
`apps/web/e2e/tests/canvas/mobile-plugin-canvas.spec.ts` and adds the equivalent
focused case in `plugin-canvas.spec.ts`. The projects are `mobile-chrome` and
`chromium`. Assert the actual preset before editing it, then assert that the
submitted task retains the edited description.

Use isolated managed runners. Existing mock lifecycle coverage proves canvas
registration and publication. It does not prove an external model's compliance
with the prompt. Do not report that stronger claim from deterministic tests.

## Work orders

- [x] [Task 01: Compact MCP discovery guidance](task-01-mcp-discovery-guidance.md) — done
- [x] [Task 02: Canvas creation preset](task-02-canvas-creation-preset.md) — done

Both work orders are sequential. Implementation requires the later explicit
request specified by the repository workflow.

## Verification results

- `rtk python3 scripts/lint-spec-files.test.py`: 30 tests passed.
- `rtk python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `rtk git diff --check`: passed.

Implementation verification is complete. The new prompt producers and locale
contract tests pass, and focused desktop and mobile E2E coverage confirms the
editable preset reaches the task creation flow and retains user edits.

The raw template is 2,743 UTF-8 bytes. Rendered context is 4,758 bytes for an
ordinary task and 5,179 bytes when canvas authoring guidance is enabled.

## Risks

- Removing tool descriptions can erase a mandatory workflow rule. Retain its
  current condition and test it independently from prose formatting.
- A global canvas flag alone can advertise tools to an excluded profile.
  Reuse the existing profile resolver and cover negative surfaces.
- Exact text snapshots can hide missing behavior behind frequent rewrites.
  Assert required instructions and invariant tool names instead.
- Existing models can still ignore instructions. Prompt delivery tests do not
  guarantee model compliance.

## Documentation impact

Internal requirements, designs, and work orders change in this package.
Public canvas usage and navigation remain the same. The implementation must
check existing public canvas guidance before its final handoff.
