---
status: active
system: agents
created: 2026-08-14
owners:
  - kandev
---
# Agent Rich Output Requirements

## Overview

Agents can explain results in Markdown, but trends and workspace files often need a more direct visual treatment. Users need those results to remain calm, readable, and trustworthy inside the transcript instead of becoming arbitrary agent-designed dashboards.

## Requirements

### REQ-AGENTS-AGENT-RICH-OUTPUT-001: Agent Rich Output

**Intent:** Agents can explain results in Markdown, but trends and workspace files often need a more direct visual treatment. Users need those results to remain calm, readable, and trustworthy inside the transcript instead of becoming arbitrary agent-designed dashboards.

#### Acceptance criteria

- **AC-AGENTS-AGENT-RICH-OUTPUT-001.1:** Task and Office agents can call `show_rich_output_kandev` to add one native, replayable presentation to the conversation.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.2:** A presentation has a plain-text title, an optional plain-text description, and one to four ordered blocks.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.3:** Version 1 supports only file previews, line or bar charts, and metric groups.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.4:** Line and bar charts may receive values inline or select columns from a workspace CSV. CSV-backed results persist as normalized snapshots so they do not depend on the later workspace lifecycle.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.5:** Every chart displays readable x- and y-axis values. Kandev automatically shortens ISO date/time labels and large numbers for the plot while preserving the original x value in the tooltip.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.6:** Every chart identifies its series in a legend. Multi-series legends are local keyboard- and touch-operable filters; changing them never changes the persisted presentation or sends a tool callback.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.7:** Line and bar charts retain their native Recharts entrance animation by default. Kandev defers the expensive plot until its chart is near the viewport in a visible browser tab, then mounts and animates it once.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.8:** Appearance settings provide a per-device option to disable rich-output chart animation. The operating system's reduced-motion preference also disables it, regardless of the saved option.

## Migrated source detail

## Why

Agents can explain results in Markdown, but trends and workspace files often
need a more direct visual treatment. Users need those results to remain calm,
readable, and trustworthy inside the transcript instead of becoming arbitrary
agent-designed dashboards.

## What

- Task and Office agents can call `show_rich_output_kandev` to add one native,
  replayable presentation to the conversation.
- A presentation has a plain-text title, an optional plain-text description,
  and one to four ordered blocks.
- Version 1 supports only file previews, line or bar charts, and metric groups.
- Line and bar charts may receive values inline or select columns from a
  workspace CSV. CSV-backed results persist as normalized snapshots so they do
  not depend on the later workspace lifecycle.
- Every chart displays readable x- and y-axis values. Kandev automatically
  shortens ISO date/time labels and large numbers for the plot while preserving
  the original x value in the tooltip.
- Every chart identifies its series in a legend. Multi-series legends are
  local keyboard- and touch-operable filters; changing them never changes the
  persisted presentation or sends a tool callback.
- Line and bar charts retain their native Recharts entrance animation by
  default. Kandev defers the expensive plot until its chart is near the
  viewport in a visible browser tab, then mounts and animates it once.
- Appearance settings provide a per-device option to disable rich-output chart
  animation. The operating system's reduced-motion preference also disables
  it, regardless of the saved option.
- Unchanged completed presentations reuse their parsed payload and derived
  chart inputs across unrelated transcript rerenders. Local legend filtering
  rerenders only the affected chart, keeps its data/configuration objects
  stable, and does not restart its entrance animation.
- Kandev owns all typography, spacing, color, layout, accessibility, and
  responsive behavior. Agent input never supplies HTML, Markdown, JavaScript,
  CSS, remote URLs, colors, animation, or component names.
- Completed rich-output calls remain standalone transcript items rather than
  being collapsed into the generic tool-activity group.
- Pending calls use the normal compact tool state. Failed calls show the normal
  tool error. A completed call with unsupported or malformed data shows a
  compact localized unavailable state and does not break nearby messages.
- Small textual comparisons continue to use Markdown tables. Version 1 does
  not provide a native data-table block.
- Always-injected Task and Office guidance includes one exact inline-chart,
  CSV-chart, and metric-group recipe. It tells agents to call the native tool
  directly for explicit presentation requests instead of building ASCII, SVG,
  HTML, or another app. The MCP description stays a focused routing summary,
  while its schema retains complete chart examples. Kandev, rather than the
  agent, owns axes, legends, colors, and layout; agents provide only semantic
  blocks, inline chart values, or CSV column mappings.

## API surface

`show_rich_output_kandev` is present only in the Kanban-task and Office-task
MCP profiles. It is absent from configuration and external MCP profiles.

The tool accepts a closed JSON object. Unknown fields fail validation.

```json
{
  "version": 1,
  "title": "Build health",
  "description": "Latest local verification",
  "blocks": [
    {
      "type": "metrics",
      "items": [
        { "label": "Passed", "value": "38" },
        { "label": "Duration", "value": "12.4s", "detail": "Warm cache" }
      ]
    },
    {
      "type": "chart",
      "chart_type": "line",
      "title": "Runtime by run",
      "summary": "Runtime fell across the last five runs.",
      "csv": {
        "path": "reports/build-times.csv",
        "x_column": "started_at",
        "series": [
          { "column": "seconds", "label": "Seconds" }
        ]
      }
    },
    {
      "type": "file",
      "path": "reports/build.json",
      "title": "Raw report",
      "caption": "Generated by the verification run",
      "mime_type": "application/json"
    }
  ]
}
```

Top-level limits:

- `version` is exactly `1`.
- `title` is 1 to 120 characters.
- `description`, when present, is at most 500 characters.
- `blocks` contains 1 to 4 items.
- The complete encoded argument object is at most 64 KiB.

Block contracts:

- `file`: `path` is a non-empty workspace-relative path of at most 1024
  characters. Optional `repo`, `title`, `caption`, and `mime_type` are plain
  strings. Absolute paths, traversal segments, URLs, data URIs, and inline
  file bytes are rejected.
- `chart`: `chart_type` is `line` or `bar`; `title` and `summary` are required.
  Its data is exactly one of:
  - inline `labels` with 1 to 100 plain strings and `series` with 1 to 4
    entries. Every series contains the same number of finite numeric or `null`
    values as `labels`;
  - `csv` with a workspace-relative `.csv` `path`, optional `repo`, an exact
    `x_column`, and 1 to 4 series mappings. Each mapping has an exact numeric
    `column` and optional display `label`.
  Kandev assigns visual colors, dimensions, axis formatting, tooltip behavior,
  and legend controls in both forms. A one-series chart still names that series
  in a non-interactive legend; a multi-series chart lets the user show or hide
  each series locally.
- `metrics`: `items` has 1 to 6 entries. Every item has a plain-text `label`
  and display `value`, plus optional plain-text `detail`. Metrics carry no
  agent-selected semantic color.

Inline-only calls return a short text acknowledgement. Their canonical
presentation payload is the tool input already stored at
`metadata.normalized.generic.input`. Calls containing CSV charts return a
versioned `resolved_charts` result, as both standard MCP structured content and
an equivalent JSON text fallback. Each entry identifies its chart block index
and stores only bounded labels plus numeric or `null` series values.

Decision: [ADR-2026-08-14-kandev-native-agent-rich-output](../../../decisions/2026-08-14-kandev-native-agent-rich-output.md).

## Permissions

The tool inherits the current task session's existing MCP identity and message
authorization. It does not accept a task or session identifier and performs no
separate backend mutation. File expansion uses the current session's existing
workspace-file authorization.

The tool is advertised as a read-only presentation tool. Test and demo agent
profiles may still choose a stricter provider permission policy, so isolated
real-agent evaluation profiles must explicitly enable full access and automatic
approval when the evaluation is intended to run unattended.

## Failure modes

- Schema or semantic validation failure rejects the tool call with a concise
  error and produces no rich presentation.
- CSV resolution rejects missing or binary files, files over 256 KiB, malformed
  rows, duplicate or missing headers, zero or more than 100 data rows, blank or
  overlong x values, and non-finite numeric cells. Errors identify the source
  row and column when possible. Empty numeric cells become chart gaps.
- A pending tool call does not render unvalidated agent data as a presentation.
- A malformed historic completed payload fails closed in the frontend and
  leaves the rest of the transcript usable.
- File content is never fetched automatically. Expansion errors, missing
  files, binary formats without a preview, and expired task workspaces render
  a localized unavailable state while other blocks remain usable.
- Unknown future versions are not guessed. They render the unavailable state.

## Persistence guarantees

The presentation payload follows existing tool-call message retention and
survives WebSocket replay, page reload, and backend restart without a new
database entity. Inline blocks replay from arguments. CSV-backed charts replay
from their normalized result snapshot and never re-read the source. Referenced
file-preview bytes remain owned by the task workspace and can disappear when
that workspace is archived, deleted, reset, or cleaned; the message remains and
its file block degrades explicitly.

Codex ACP may transport MCP calls in an `execute` tool frame whose `rawInput`
contains the MCP server, actual tool name, and `arguments`. The ACP adapter
recognizes that provider envelope and persists a provider-neutral generic tool
call using the actual MCP tool name, arguments, and completed MCP result. The
shell-shaped transport category is never exposed as the presentation's stored
tool identity.

No raw CSV, file bytes, images, or other binary payloads are copied into SQLite
for this feature.

## Mobile design contract

- Desktop and mobile entry point: the completed presentation appears inline at
  its chronological location in the existing task or Office transcript.
- Nearest mobile exemplars: the existing chat transcript supplies the inline
  surface; `MobileFileViewerPanel` supplies focused file inspection.
- Hierarchy: presentation title, optional description, then ordered blocks.
  File open and chart inspection are the primary block actions.
- Presentation remains inline because it is conversation evidence, not a new
  primary destination. Opening a file uses the existing dedicated mobile file
  surface rather than embedding a desktop editor in chat.
- The transcript remains the single vertical scroll owner. Charts stay within
  their block width, metric items reflow without horizontal page scrolling,
  legend controls wrap within the chart, and actions have touch-safe targets.
- Desktop and mobile share parsing, validation, data, and actions. Only block
  composition and sizing vary with `useResponsiveBreakpoint` where needed.

## Scenarios

- **GIVEN** a task or Office agent, **WHEN** it lists MCP tools, **THEN**
  `show_rich_output_kandev` is available with the closed version 1 schema.
- **GIVEN** an agent is asked for an inline line or bar chart, **WHEN** it reads
  Kandev's tool guidance, **THEN** it can copy the canonical `chart_type`,
  `title`, `summary`, `labels`, and labeled `series[].values` shape without
  inventing axis, category, or data fields.
- **GIVEN** an agent is explicitly asked for a chart, file preview, KPI cards,
  or a native metric summary and suitable data exists, **WHEN** it chooses a
  response mechanism, **THEN** it calls `show_rich_output_kandev` directly
  instead of creating an ASCII, SVG, HTML, or app-based substitute.
- **GIVEN** an agent is asked for KPI cards or a native metric summary, **WHEN**
  it reads Kandev's tool guidance, **THEN** it can copy the canonical metric
  `items[].label` and `items[].value` shape without editing a workspace file.
- **GIVEN** a configuration or external MCP client, **WHEN** it lists tools,
  **THEN** `show_rich_output_kandev` is absent.
- **GIVEN** a valid completed presentation, **WHEN** the transcript receives
  the tool update, **THEN** its file, chart, and metric blocks render as one
  standalone native transcript item.
- **GIVEN** the same completed message after a page reload, **WHEN** history is
  replayed, **THEN** the presentation is equivalent to its live rendering.
- **GIVEN** Codex ACP emits an MCP invocation as an `execute` frame with
  `_meta.is_mcp_tool_call`, **WHEN** the initial and terminal updates are
  normalized, **THEN** the transcript stores one generic
  `show_rich_output_kandev` tool call with unwrapped arguments and MCP result.
- **GIVEN** a pending rich-output call, **WHEN** its arguments arrive, **THEN**
  the transcript shows only the normal pending tool state.
- **GIVEN** malformed input, an unknown block, or a payload over 64 KiB,
  **WHEN** the tool is called, **THEN** validation fails closed.
- **GIVEN** a workspace CSV with an x column and numeric columns, **WHEN** a
  line or bar chart names those columns, **THEN** the handler resolves at most
  100 rows and persists a normalized result snapshot.
- **GIVEN** a completed CSV-backed chart whose workspace file is later edited
  or removed, **WHEN** conversation history reloads, **THEN** the chart remains
  identical to the accepted snapshot and performs no workspace read.
- **GIVEN** a valid line or bar chart, **WHEN** it renders, **THEN** both axes
  contain visible tick values, its tooltip exposes the original x value, and
  every series is named in the legend.
- **GIVEN** a multi-series chart, **WHEN** the user toggles a legend item with a
  pointer, touch, or keyboard, **THEN** that series is filtered locally and can
  be restored without changing the persisted payload.
- **GIVEN** a valid line or bar chart below the viewport or in a background
  browser tab, **WHEN** its transcript loads, **THEN** Kandev preserves the
  chart's title, summary, and layout space without mounting or animating the
  expensive plot.
- **GIVEN** that deferred chart, **WHEN** it approaches the viewport in a
  visible tab, **THEN** its plot mounts once and retains the default Recharts
  entrance animation.
- **GIVEN** an unchanged completed rich-output message, **WHEN** unrelated chat
  state rerenders its parent, **THEN** Kandev reuses the parsed presentation,
  chart data, chart configuration, and formatter callbacks instead of
  rebuilding the chart subtree or restarting its entrance animation.
- **GIVEN** the per-device rich-output animation option is disabled, or the
  operating system requests reduced motion, **WHEN** a chart plot mounts,
  **THEN** its complete series geometry appears without animation while axes,
  tooltips, and legend filtering remain available.
- **GIVEN** a valid presentation with a workspace file, **WHEN** the user
  expands the file block, **THEN** Kandev requests the file once and displays
  a bounded preview or explicit unavailable state.
- **GIVEN** a file block in a multi-repository task, **WHEN** the user opens
  it, **THEN** the optional repository discriminator reaches the existing file
  viewer selection path.
- **GIVEN** a Pixel 5-sized viewport, **WHEN** file, chart, and metric blocks
  render, **THEN** content stays inside the viewport, actions remain touchable,
  and file opening reaches the existing mobile viewer.
- **GIVEN** a small row-and-column comparison, **WHEN** an agent reports it,
  **THEN** it uses ordinary Markdown rather than a native rich table block.

## Out of scope

- Portable rendering of every MCP or ACP content block.
- MCP Apps, `ui://` resources, iframes, executable UI, or tool callbacks.
- Plugin-contributed rich renderers or changes to the plugin agent-tool result
  contract.
- Native tables, forms, editors, maps, arbitrary dashboards, or agent-defined
  layout.
- Changing Recharts' default entrance style or duration for users who leave
  rich-output animation enabled.
- Transcript-wide virtualization, a canvas chart renderer, data sampling, or
  lower chart payload limits. Those require separate evidence after deferred
  mounting and referential-stability work is measured.
- Durable raw artifact storage or a new workspace file-preview API.

## Implementation plan

- [Original implementation plan](../../../plans/agent-rich-output/plan.md)
- [Chart rendering performance repair](../../../plans/rich-output-chart-performance/plan.md)
