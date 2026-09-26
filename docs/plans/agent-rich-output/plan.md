---
spec: docs/specs/agents/requirements/agent-rich-output.md
created: 2026-08-14
status: complete
---

# Implementation Plan: Agent Rich Output

## Overview

Record the host-native trust and persistence boundary first. Add a strict
task/Office MCP tool next, then render its persisted inline arguments or
normalized CSV result as a standalone chat presentation. Finish with native
file, chart, and metric blocks, mobile parity, focused E2E proof, and public
MCP reference docs.

## Backend

### MCP schema and profile

- `apps/backend/internal/mcp/server/rich_output.go`: define the closed tool
  schema, decoded contracts, semantic limits, path validation,
  `validateRichOutput`, `showRichOutputHandler`, and
  `registerRichOutputTool`.
- `apps/backend/internal/mcp/server/server.go`: add one profile group enabled
  only for `SurfaceKanbanTask` and `SurfaceOfficeTask`.
- `apps/backend/config/prompts/kandev-context.md` and `office-context.md`: list
  the exact tool and plain-text-first authoring rubric.

No database, service, or repository change is required. Inline presentations
replay from normalized tool-call input. CSV charts reuse the existing
`workspace.file.get` action once and replay from their normalized tool result.

## Frontend

### Contract and transcript dispatch

- Add a closed discriminated union and fail-closed parser under
  `apps/web/components/task/chat/messages/kandev/rich-output/`.
- Add `isRichOutputMessage` to
  `apps/web/components/task/chat/types.ts` and exempt it in
  `apps/web/hooks/use-processed-messages.ts` so completed presentations remain
  standalone.
- Register `show_rich_output` in the existing Kandev renderer registry and
  pass session/file-opening context through `KandevToolMessage`.

### Native blocks

- Compose one quiet outer presentation with existing `KandevRow` and
  `KandevBody` patterns.
- Render metrics with tabular numbers and responsive wrapping.
- Render line/bar charts through `@kandev/ui` chart primitives and host theme
  tokens. Render direct Recharts axis and tooltip children, use host-owned tick
  formatting, and provide a visible series legend with local multi-series
  filtering. Use no new dependency or agent-selected colors.
- Render file metadata without fetching. On explicit expansion, call
  `requestFileContent`; clip text previews, outline images neutrally, and route
  open through the existing desktop/mobile viewer.
- Widen the central file-open callback to accept optional repository identity
  while preserving one-argument callers.
- Add translated host copy in the task namespace and regenerate pseudo locale.

### Mobile design contract

- Outcome and entry point: inspect the same chronological presentation in the
  existing mobile transcript.
- Exemplar: inline chat message composition plus `MobileFileViewerPanel` for
  focused file content.
- Hierarchy: title, description, ordered blocks; the transcript owns vertical
  scrolling.
- Surface rationale: results remain inline conversation evidence; file content
  moves to the existing full-height viewer because dense content should not be
  squeezed into chat.
- Geometry: 44px actions, bounded chart width/aspect, wrapping metric grid,
  zero document horizontal overflow, and no hover-only action.

## Tests

- **MCP contract and limits:**
  `apps/backend/internal/mcp/server/rich_output_test.go`; table-driven valid and
  invalid payloads plus profile presence.
- **Profile inventories and prompt parity:** extend
  `server_test.go` and keep `sysprompt_sync_test.go` exact.
- **Frontend parser and message predicate:** add pure TypeScript tests beside
  the parser and in `components/task/chat/types.test.ts`.
- **Transcript grouping:** extend `apps/web/hooks/use-processed-messages.test.ts`
  to preserve ordinary activity grouping and exempt rich output.
- Pure UI markup uses focused React DOM coverage in
  `rich-output-renderer.test.tsx` and `chart-block.test.tsx`; Playwright is
  reserved for persistence, browser chart geometry, and file-viewer behavior.

## E2E Tests

- **Desktop:** `apps/web/e2e/tests/chat/rich-output.spec.ts` calls the real
  inline mock-agent MCP directive, observes metrics, two charts, and a file,
  reloads, expands and opens the seeded file, and proves the result is not
  activity-grouped.
- **Mobile:** `apps/web/e2e/tests/chat/mobile-rich-output.spec.ts` proves chart
  containment, metric reflow, touch targets, no horizontal overflow, and file
  opening in the existing mobile viewer.

## Verification Results

- `cd apps/backend && go test ./internal/mcp/server/...`: passed.
- Focused Vitest parser, predicate, grouping, and renderer suite: 4 files and
  95 tests passed.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet`: passed. The i18n check retained 961 existing
  advisory real-locale parity notices; pseudo is synchronized.
- Desktop Chromium rich-output E2E: 1 test passed, including reload, lazy
  preview, and file opening.
- Mobile Chrome rich-output E2E: 1 test passed, including containment, 44px
  actions, preview, and the native file viewer.
- Focused E2E sleep lint, 61 public-doc validator tests, validation of all 41
  published pages, and `git diff --check`: passed.
- CSV extension: `go test ./internal/mcp/server/... -count=1` and focused
  `golangci-lint` passed with zero issues; focused rich-output Vitest passed 3
  files and 73 tests.
- CSV extension: typecheck, full web lint, i18n check/ratchet, desktop Chromium,
  and mobile Chrome passed. Desktop replay remained rendered after deleting the
  source CSV; mobile rendered both charts without horizontal overflow.
- Axis/legend repair: 62 focused frontend tests, backend MCP tests, typecheck,
  full web lint, i18n checks, desktop Chromium, and mobile Chrome passed. Both
  browsers proved visible x/y ticks, raw-value tooltip content, single-series
  labels, multi-series filtering, replay, touch targets, and containment.
- Public reference validation passed all 61 validator tests and 41 published
  pages after documenting automatic axes, tooltips, legend filtering, and
  unit-bearing series labels.
- Real-agent ACP delivery: the broad agentctl/MCP Go suite passed; focused
  rich-output/settings Vitest passed 10 files and 86 tests; typecheck and i18n
  checks passed; desktop Chromium passed 2 tests and mobile Chrome passed 1.
  A fresh full-access, auto-approved Luna session produced the CSV chart in one
  successful tool call. Canonical persistence, axes, legend, and reload were
  verified in the live isolated UI with no permission prompt or console error.

## Implementation Waves

1. [x] [Record contract and decision](task-01-record-contract.md)
2. [x] [Backend MCP contract](task-02-backend-mcp-contract.md)
3. [x] [Frontend contract and dispatch](task-03-frontend-contract-dispatch.md)
4. [x] [Native blocks and mobile parity](task-04-native-blocks-mobile.md)
5. [x] [E2E and public documentation](task-05-e2e-public-docs.md)
6. [x] [CSV-backed chart sources](task-06-csv-chart-sources.md)
7. [x] [Chart axes and legends](task-07-chart-axes-legends.md)
8. [x] [Real-agent ACP delivery](task-08-real-agent-acp-delivery.md)
9. [x] [Compact MCP guidance](task-09-compact-mcp-guidance.md)
10. [x] [Agent routing regression](task-10-agent-routing-regression.md)

Execution is sequential in the primary conversation. No subagents are
authorized.

## Risks

- Historic malformed metadata must not crash transcript rendering.
- Tool arguments arrive before server success; rich UI must wait for a
  successful terminal state.
- Workspace files can outlive sessions but not task workspace cleanup.
- Existing file reads can return up to 10 MiB. File previews never auto-fetch;
  CSV sources use a bounded one-time read capped at 256 KiB.
- Multi-repository callback widening must preserve current desktop and mobile
  file-opening behavior.
- CSV-backed charts must resolve once, reject source files over 256 KiB, and
  persist only a bounded normalized snapshot. Replay must never depend on the
  workspace file still existing or still containing the same rows.
- Recharts discovers axes and tooltips from direct chart children. A wrapper
  component around those primitives silently removes tick values, grid lines,
  and hover/touch tooltips.
- Codex ACP reports MCP invocations with the broad `execute` kind. Provider
  envelope recognition must be gated by explicit MCP metadata and must retain
  the actual tool name, arguments, and result without reclassifying ordinary
  shell execution.
- JSON Schema examples alone may not be visible enough to weaker agents. Keep
  the system prompt recipe compact, exact, and synchronized across Task and
  Office surfaces.

## Axis and legend repair

Status: complete on 2026-08-15.

- Add browser assertions that line and bar charts render x/y tick values,
  preserve raw x values in tooltips, show a legend for every series count, and
  locally filter multi-series plots.
- Replace the opaque `ChartAxes` component boundary in
  `apps/web/components/task/chat/messages/kandev/rich-output/chart-block.tsx`
  with direct Recharts children so its discovery pass registers axes, grid,
  and tooltip behavior.
- Add pure host-formatting helpers for compact numeric ticks, ISO date/time
  ticks, and bounded category labels. The underlying labels and values remain
  unchanged for replay and tooltip inspection.
- Add an accessible legend that is informational for one series and uses
  pressed-state buttons for multiple series. Controls wrap inside the chart and
  retain 44px mobile hit targets.
- Verify the focused desktop and mobile production-build E2E flows, web
  typecheck/lint, and the running isolated Tailscale gallery without touching
  the main `:9998` instance.

Result: Recharts now receives axes, grid, tooltip, and legend as direct chart
children. Host formatting keeps raw replay data intact while showing compact
dates, categories, and numbers. Every chart names its series; multi-series
legends are 44px pressed-state controls that filter locally. The isolated
desktop and Pixel 5 gallery rendered four axes and four legend controls with no
horizontal overflow, while the main `:9998` process retained its original PID.

## Real-agent ACP delivery

Status: complete on 2026-08-16.

- Recognize only explicitly marked Codex MCP `execute` frames, preserve trusted
  provenance across the marker-less completion update, and persist canonical
  arguments plus the unwrapped MCP result.
- Keep the frontend compatible with historic nested Codex payloads while new
  messages use the provider-neutral shape.
- Give agents compact, exact inline and CSV chart recipes in Task and Office
  guidance plus schema examples; keep the MCP description focused on routing.
- Validate selection, restraint, CSV authoring, automatic approval, canonical
  replay, axes, and legends with fresh real Luna sessions in an isolated
  Tailscale-accessible instance.

Result: Luna chose rich output for requested inline and CSV charts, retained
Markdown for a small textual comparison, and completed the final CSV case on
its first tool attempt. The isolated profile was full-access and auto-approved;
no permission or clarification request occurred. The rendered line chart
survived reload with five x-axis ticks, five y-axis ticks, and one legend.

## Instruction budget hardening

Status: complete on 2026-08-17.

- Cap the rich-output MCP description at 650 runes and each always-injected
  Task/Office guidance line at 750 runes.
- Keep the exact inline and CSV recipes once in each prompt and in schema
  examples, but remove those payloads from the routing description.
- Protect both budgets and the content split with focused backend tests.

Result: the tool description fell from 845 to 439 runes. Task and Office lines
fell from 1,205 and 1,198 runes to 740 each without weakening mandatory
explicit-request routing or removing either proven Luna recipe.

## Agent routing regression

Status: complete on 2026-08-19.

- Route explicit chart, preview, KPI, and metric-summary requests directly to
  `show_rich_output_kandev`; reject ASCII, SVG, HTML, and alternate-app
  presentation substitutes in the agent guidance.
- Add one exact metric-group recipe while retaining the inline-chart, CSV-chart,
  Markdown-restraint, and host-ownership guidance under the existing budgets.
- Re-run inline bar, CSV time series, file preview, metric summary, and small
  Markdown comparison prompts as fresh Luna Worker subtasks in separate
  worktree executor environments.

Result: the prompt lines are 737 of 750 runes and the MCP description is 454 of
650 runes. All four native requests produced exactly one valid rich-output call;
the textual comparison produced only Markdown. The CSV task created only its
requested CSV source, and all five tasks completed without permission or input
requests.
