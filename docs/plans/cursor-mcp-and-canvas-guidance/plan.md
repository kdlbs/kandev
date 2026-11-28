---
created: 2026-11-26
status: implemented
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
  - REQ-AGENTS-MCP-DISCOVERY-002
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
  - ../../specs/agents/system-design/mcp-tool-discovery-guidance.md
legacy_specs: []
---

# Restore Cursor rich output and canvas guidance

## Evidence

The supplied two-frame reproduction at fd9c766e85 emitted generic.name = other, nested raw_input and a successful result. The install had seven rich-output and seven walkthrough calls with this shape. A local Cursor ACP run (CLI 2026.10.19-6bbf2c4, default model, test MCP server) confirmed the split. Cursor opened a pending call titled `MCP: tool` with empty rawInput. A statusless update supplied the qualified title and complete provider/tool/args envelope. A separate in-progress update preceded the completed update carrying only rawOutput.success = true. All four frames used the same call ID. The installed account could not run the incident's Opus model. This probe tested the wire protocol only. At d76f11a23 the opening frame calls parseMCPToolCall. The result path reads activeToolCalls and sends late arguments through UpdatePayloadInput. The historical frontend fallback rejects the package provider. The canonical provider-qualified name already selects the renderer.

For the second incident the session began without canvas guidance. After restart the user prompt contained no canvas instructions. Message preparation gates the full block on session creation or title ownership. wrapCreatedSessionPrompt serves created sessions. An ordinary existing-session dispatch only rebuilds initial context for first-submission replay. Resume package-binding text lists packages and does not teach canvas authoring. Existing resume catalog tests cover startup guidance and inventory.

## Delivery order

1. [Repair late MCP identity](task-01-late-mcp-input.md) under AC-AGENTS-AGENT-RICH-OUTPUT-002.9. Preserve renderer trust and existing card layouts.
2. [Repair guidance delivery](task-02-canvas-guidance.md) under AC-AGENTS-MCP-DISCOVERY-002.6. The tasks have independent technical outcomes. Deliver the primary incident first.

## Scope and verification

Each work order contains its red test and exact focused command. The second work order extends the existing recorded/dispatched agreement in AC-AGENTS-MCP-DISCOVERY-002.4. Existing package rows remain historical. Rendering those rows requires a separate product decision. Canvas creation still requires an explicit user request. A lost acceptance receipt can cause one redundant reminder. Restrict delivery tracking to canvas until another optional section needs it.

## Results

Both work orders implement the repair. The second uses an execution-local attachment receipt at the provider dispatch boundary. No database migration is required.

The focused Go command in each linked work order passed.

Repository checks passed: `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.

The Chromium rich-output and walkthrough command in work order 01 passed both cases. The slash-command regression in work order 02 passed five runs.
