---
spec: docs/specs/mcp-session-observability/spec.md
decision: docs/decisions/2026-08-16-session-mcp-tool-catalog.md
created: 2026-08-16
status: done
---

# Implementation Plan: MCP Server Explorer

## Overview

Extend the session-owned MCP attachment report with the Kandev tool catalog.
Then replace the compact status disclosure with a responsive explorer.

The desktop surface is a wide dialog with server navigation and a detail pane.
The phone surface is a full-height drawer with list and detail views.

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

## Verification Results

All task-defined verification passed. Task 01 records the focused Go suite,
Task 02 records the focused Vitest, typecheck, and i18n checks, Task 03 records
the desktop and mobile Playwright results and inspected PR assets, and Task 04
records both public-doc validators. Final branch verification also includes
formatting, diff checks, and the complete changed-file review before commit.

## Implementation Waves And Parallel Candidates

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

## Risks

- Plugin tool descriptions are provider-controlled text. The UI must render
  them as text, not HTML or Markdown.
- A large tool catalog can increase session metadata. The current-only catalog
  and explicit limits bound that growth.
- Some agents load tools lazily. The UI must show "not loaded" until Kandev
  observes `tools/list`.
- Third-party tool catalogs remain unavailable without a new proxy or provider
  contract.
