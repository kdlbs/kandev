---
spec: docs/specs/platform/requirements/mcp-session-observability.md
decision: docs/decisions/2026-08-18-session-mcp-tool-definition-details.md
created: 2026-08-16
updated: 2026-08-18
status: implemented
---

# Implementation Plan: MCP Server Explorer

## Overview

The first implementation added a session-owned Kandev tool catalog and a
responsive explorer. This follow-up restores the rich status tooltip and fixes
the explorer layout.

The revised explorer uses three levels: servers, tools, and one tool. The tool
list gets most of the available space. A tool page shows its description,
arguments, and deterministic token estimate.

Third-party servers keep their current safe status details. Kandev does not
connect to those servers to collect tool metadata.

## Existing constraints

- `useSessionMcp` already selects the active session's attachment history.
- `MCPServerAttachment` already carries status, transport, target, summary,
  and tool count through persistence, boot hydration, and WebSocket updates.
- `mcp.Server` observes the exact `tools/list` result in
  `hooks.AddAfterListTools`.
- Third-party MCP connections bypass Kandev after agent configuration delivery.
- The existing desktop control uses a tooltip. The existing touch control uses
  a drawer and a 44px trigger.

## Completed baseline

## Backend

### Safe tool catalog contract

Update `apps/backend/internal/agentctl/types/streams/mcp_attachment.go` with:

- `MCPToolSummary` fields `name` and `description`.
- `tools` and `tool_catalog_truncated` on `MCPServerAttachment`.
- `tools` on `MCPAttachmentEvidence` for a Kandev `tools_list_observed` event.
- limits of 128 entries and 1,024 UTF-8 bytes per description.

Normalize catalog entries before publication. Sort entries by name. Remove
empty names, bound descriptions on a valid UTF-8 boundary, and set the
truncation marker from the full `tool_count`.

When `StartAttempt` moves the current attempt into `Previous`, remove each
server catalog from that historical copy. Keep `tool_count` for diagnostics.
Keep schema version 1 because all new fields are optional and old reports stay
valid.

### Kandev `tools/list` observation

Update `apps/backend/internal/mcp/server/server.go`. Convert
`mcp.ListToolsResult.Tools` to the bounded summary inside
`AddAfterListTools`. Send that catalog with the existing attachment evidence.

Only the local Kandev MCP server can publish catalog entries. Do not add an
agentctl HTTP endpoint. Do not inspect third-party configurations or connect to
third-party MCP servers.

The existing lifecycle, orchestrator, boot, and WebSocket paths serialize the
new optional fields without a new event type. Extend their focused rehydration
and reducer fixtures to prove that the catalog survives reload for the current
attempt.

## Frontend

### Shared types and view model

Add the catalog fields to:

- `apps/web/lib/types/session-runtime-payloads.ts`.
- `apps/web/lib/state/slices/session-runtime/types.ts`.

Create a pure explorer view model under
`apps/web/components/task/chat/mcp-explorer/`. It owns:

- deterministic selection of `kandev`, then the first server.
- selection fallback when a live update removes the selected server.
- localized status labels and catalog availability states.
- the stored and total tool counts.
- plain-text tool names and descriptions.

### Desktop dialog

Extract `McpIndicator` from
`apps/web/components/task/chat/chat-input-toolbar-primitives.tsx` into the new
explorer folder.

Keep a short hover and focus tooltip for the trigger label. On click, open a
controlled `Dialog` with `enterConfirms={false}`. Use a bounded wide surface,
such as `sm:max-w-4xl`, with one internal body height.

Use a two-column body on desktop:

- a fixed-width server list with status dots and status labels.
- a flexible detail pane with server metadata and the tool catalog.

The detail pane owns vertical scrolling. Long names wrap or truncate inside
their pane. The document does not gain horizontal overflow.

### Phone and tablet drawer

Use `useResponsiveBreakpoint` and the existing `Drawer` primitive. Phones use
a full-height `100dvh` surface. Tablets with a coarse pointer use a bounded
drawer.

The first view lists servers. A 44px server row opens one focused detail view.
A visible 44px Back control returns to the list. The header stays fixed, and
the body is the only vertical scroll owner. Bottom content clears the safe-area
inset.

Desktop and touch surfaces share the same server selection, view model, status
metadata, and tool list components.

### Copy and localization

Move the existing hard-coded MCP status labels into the `task` namespace. Add
all new labels, empty states, catalog limits, and third-party explanations to
each task locale. Do not translate server names or tool names.

## Tests

- **Catalog normalization:** table-driven Go tests cover sorting, empty names,
  UTF-8 description bounds, entry bounds, and truncation.
- **Attempt history:** a Go test proves that the current catalog persists and
  a superseded attempt keeps only its count.
- **MCP hook:** a Go test proves that `tools/list` publishes names and
  descriptions without schemas or other tool data.
- **Wire types:** existing lifecycle and orchestrator fixtures include a
  current Kandev catalog and survive JSON rehydration.
- **View model:** Vitest covers default selection, live fallback, unavailable
  catalogs, third-party limits, and truncated counts.
- **Responsive components:** Testing Library covers dialog open and close,
  server selection, tool descriptions, phone list-to-detail navigation, Back,
  focus return, and localized empty states.

## E2E Tests

- **Scenario:** A desktop user clicks the MCP trigger and selects Kandev.
  **File:** `apps/web/e2e/tests/chat/mcp-status.spec.ts`.
  **Outcome:** A wide dialog shows the active Kandev tools and descriptions.
- **Scenario:** A desktop user selects a third-party server.
  **File:** `apps/web/e2e/tests/chat/mcp-status.spec.ts`.
  **Outcome:** The detail pane shows safe status and the catalog limitation.
- **Scenario:** A phone user opens the explorer and selects Kandev.
  **File:** `apps/web/e2e/tests/chat/mobile-mcp-status.spec.ts`.
  **Outcome:** A full-height drawer shows a focused tool list and a Back control.
- **Scenario:** Tool content exceeds the phone viewport.
  **File:** `apps/web/e2e/tests/chat/mobile-mcp-status.spec.ts`.
  **Outcome:** The drawer scrolls internally and the document has no horizontal
  overflow.

## Public documentation

Update these explanation pages after the UI lands:

- `docs/public/automation-and-mcp.md` explains how to open the explorer and
  what Kandev can show for each server type.
- `docs/public/agents-and-profiles.md` updates the missing-tool troubleshooting
  step for the new dialog and drawer.

## Follow-up implementation

### Tool definition contract

Extend `MCPToolSummary` with these optional fields:

- `input_schema`, stored as valid JSON for the current Kandev attempt.
- `input_schema_truncated`, set when the schema exceeds a storage limit.
- `estimated_tokens`, calculated from the complete MCP tool JSON.

Add `tool_token_estimator` to `MCPServerAttachment`. Its value is
`o200k_base:mcp-tool-json-v1` when estimates are present.

Keep the existing 128-tool and 1,024-byte description limits. Add a 64 KiB
limit for one input schema and a 512 KiB combined schema limit. If a schema is
too large, omit the complete schema instead of storing invalid partial JSON.

Calculate the estimate before the schema projection removes fields. Use the
compact JSON that `mcp.Tool.MarshalJSON` sends for one tool. This includes the
complete MCP definition but excludes the surrounding `tools/list` response.

Use `github.com/tiktoken-go/tokenizer` with `o200k_base`. Keep tokenization in a
small backend package with a reusable codec. The package must work offline and
must not download vocabulary data at runtime. Do not use a character-count
fallback.

Record the binary-size change in Task 05. If the dependency adds an
unacceptable release cost, stop and revise the decision before replacement
with a heuristic.

### Explorer state and navigation

Replace the current server-detail composition with these page states:

1. The server page shows all session servers.
2. The tools page shows one server summary and its scrollable tool list.
3. The tool page shows one tool description and input schema.

Desktop keeps the server rail visible. Selecting a server opens its tools page
in the main pane. Selecting a tool replaces that pane with the tool page.

Phones use one page at a time. Back returns from the tool page to the tools
page. A second Back returns to the server page. Tablets use the same focused
navigation in the bounded drawer.

Store the selected server, selected tool, tool-list scroll offset, and return
focus target while the explorer is open. If a live catalog removes the selected
tool, return to the tools page and use the deterministic server fallback.

### Status tooltip and compact metadata

Restore the previous precise-pointer tooltip. It lists each current server with
its status dot, name, localized status label, and bounded summary. An Active
Kandev server uses the existing green status color.

Touch users get the same status data on the server and tools pages. No action
or status depends on hover.

Move transport, target, connection ID, and observation time into a compact
connection-details disclosure. Keep the server name, status, tool count, and
catalog notices visible. The tool list uses the remaining height.

Pass `showCloseButton={false}` to the desktop `DialogContent` when the custom
44px close control remains. The accessible dialog contains one close control.

Use a fixed explorer header and one page-body scroll owner. On desktop, add an
explicit constrained grid row with `min-h-0`. The tool list owns the main-pane
scroll area. The dialog and drawer must not scroll the document.

### Tool list and tool page

Each tool row shows its name and `~N tokens` when an estimate exists. It does
not show the full description. The selected row opens the tool page.

The tool page shows the name, estimate, full plain-text description, and input
schema. Render common object properties as rows with name, type, required
state, and description. Add a plain JSON view for nested schemas, references,
and composition keywords.

If the schema is absent because the tool has no arguments, show **No
arguments**. If storage limits removed it, show a distinct **Schema too large
to display** state.

Explain the estimate near the first token value: it uses `o200k_base`, and the
actual agent cost varies by model and provider. Do not repeat this explanation
on every row.

### Follow-up tests

- Backend tests cover structured and raw input schemas, schema limits, combined
  limits, historical removal, estimator metadata, known tokenizer vectors, and
  tokenizer errors.
- Component tests cover the rich tooltip, one close control, three-level
  navigation, schema states, scroll restoration, focus restoration, and live
  catalog removal.
- Desktop E2E proves internal tool-list scrolling and opens a tool page with
  arguments and an estimate.
- Mobile E2E proves server-to-tools-to-tool navigation, both Back actions,
  44px controls, safe-area clearance, and no document overflow.
- Public docs explain arguments, estimate limits, and third-party catalog
  limits.

## Verification Results

All baseline and follow-up verification passed. Tasks 01 through 08 contain
the exact commands, RED evidence, final results, and inspected assets.

The backend focused suites, known `o200k_base` vectors, and release build pass.
The exact offline tokenizer adds about 3.0 MB to the compressed `kandev`
binary and 2.9 MB to the compressed `agentctl` binary.

The frontend focused suite passes 31 tests. Type checking, localization checks,
the localization ratchet, and focused lint pass. Fresh production E2E runs pass
on desktop Chromium and mobile Chrome. The inspected assets show the compact
tool list, desktop tool detail, and mobile tool detail.

The public documentation test suite passes 61 tests, and the validator accepts
all 41 published pages.

## Implementation Waves And Parallel Candidates

Completed baseline:

Wave 1:

- [x] [Task 01: Capture the Kandev tool catalog](task-01-capture-tool-catalog.md)

Wave 2:

- [x] [Task 02: Build the responsive explorer](task-02-build-responsive-explorer.md)

Wave 3:

- [x] [Task 03: Prove browser flows](task-03-prove-browser-flows.md)
- [x] [Task 04: Document the explorer](task-04-document-explorer.md)

Tasks 03 and 04 are parallel-safe because they own disjoint E2E and public-doc
files. The primary session executes all tasks in sequence unless the user asks
for subagents.

Follow-up wave 1:

- [x] [Task 05: Capture tool definitions and estimates](task-05-capture-tool-definitions.md)

Follow-up wave 2:

- [x] [Task 06: Refine explorer navigation and layout](task-06-refine-explorer-ux.md)

Follow-up wave 3:

- [x] [Task 07: Prove the revised browser flows](task-07-prove-revised-browser-flows.md)
- [x] [Task 08: Update explorer documentation](task-08-update-explorer-docs.md)

Tasks 07 and 08 are parallel-safe after Task 06. They own separate E2E and
public-doc files.

## Risks

- Plugin tool descriptions are provider-controlled text. The UI must render
  them as text, not HTML or Markdown.
- A large tool catalog can increase session metadata. The current-only catalog
  and explicit limits bound that growth.
- Some agents load tools lazily. The UI must show "not loaded" until Kandev
  observes `tools/list`.
- Third-party tool catalogs remain unavailable without a new proxy or provider
  contract.
- Token estimates differ from provider context and billing counts. The UI must
  always identify them as estimates.
- The offline `o200k_base` vocabulary increases the backend binary. Task 05
  records this change before the implementation continues.
- Input schemas can contain provider-controlled text. The UI must render schema
  values as data and never as HTML.
