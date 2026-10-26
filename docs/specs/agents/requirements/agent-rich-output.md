---
status: active
system: agents
created: 2026-08-14
owners:
  - kandev
---
# Agent Rich Output Requirements

## Overview

Task and Office agents present results inside the conversation using approved
native components. Catalog composition lets an agent choose components,
grouping, and order while Kandev controls appearance, responsive behavior, and
interactions. Agents owns the agent-facing contract shared by task and Office
consumers.

Requirements 001 and 002 retain their completed replacement baseline. Requirement
003's component baseline is implemented by the structured evidence package
completed on 2026-10-10. Its authoring-feedback repair is implemented locally by the linked recovery
package completed on 2026-10-12.
The contract retains version 1 historical replay and uses version 2 for new
composition.
The user selected json-render and catalog composition on 2026-10-08. Delivery
is tracked in the [replacement plan](../../../plans/json-render-rich-output/plan.md).

## Terminology

- **Presentation:** One completed rich-output tool call in the transcript.
- **Catalog:** The versioned set of components and properties Kandev permits.
- **Composition:** A rooted hierarchy of catalog elements with ordered children.
- **Snapshot:** Accepted chart data or selected table cells retained with a completed message.

## Requirements

### REQ-AGENTS-AGENT-RICH-OUTPUT-001: Agent rich output

**Intent:** Preserve the observable behavior of existing native rich output.

#### Acceptance criteria

- **AC-AGENTS-AGENT-RICH-OUTPUT-001.1:** Task and Office agents can call `show_rich_output_kandev` to add one native, replayable presentation to the conversation.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.2:** A retained historical version 1 presentation has a plain-text title, an optional plain-text description, and one to four ordered blocks.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.3:** Retained historical version 1 supports only file previews, line or bar charts, and metric groups.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.4:** Line and bar charts may receive values inline or select columns from a workspace CSV. CSV-backed results persist as normalized snapshots so they do not depend on the later workspace lifecycle.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.5:** Every chart displays readable x- and y-axis values. Kandev automatically shortens ISO date/time labels and large numbers for the plot while preserving the original x value in the tooltip.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.6:** Every chart identifies its series in a legend. Multi-series legends are local keyboard- and touch-operable filters; changing them never changes the persisted presentation or sends a tool callback.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.7:** Line and bar charts retain their native Recharts entrance animation by default. Kandev defers the expensive plot until its chart is near the viewport in a visible browser tab, then mounts and animates it once.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.8:** Appearance settings provide a per-device option to disable rich-output chart animation. The operating system's reduced-motion preference also disables it, regardless of the saved option.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.9:** When a file card's session, repository discriminator, or path changes, it shall discard the previous target's content, error, and loading state. Its disclosure shall use the new target's retained choice or default under REQ-AGENTS-IMAGE-PREVIEW-DEFAULTS-001. Non-image replacement shall request no content until explicit expansion; image replacement may automatically request only the new target.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.10:** A pending read for a replaced file target shall never publish its eventual success or failure into the replacement card, including after that card is explicitly expanded.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.11:** When session, repository discriminator, and path stay unchanged, unrelated rerenders and title or caption changes shall preserve the card's disclosure and preview state. Hiding and re-expanding a successful preview shall reuse its content; re-expanding a failed preview shall allow an explicit retry.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.12:** Each file card shall own independent preview and disclosure state, including when two cards reference the same target. Replacing or toggling one card shall not reset or expand another. These state rules shall apply equally on desktop and phone within the existing inline presentation.

- **AC-AGENTS-AGENT-RICH-OUTPUT-001.13:** Pending calls shall show compact tool activity, failed calls shall show the normal tool error, and successfully completed calls shall appear as standalone transcript items. Unsupported or malformed completed presentations shall show a localized unavailable state without breaking adjacent messages.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.14:** Image previews shall use the default and retained-choice behavior in REQ-AGENTS-IMAGE-PREVIEW-DEFAULTS-001; other file previews shall fetch content only after explicit expansion. File cards shall open the existing desktop or mobile file viewer with the current session, path, and repository selection. Unavailable, binary, or expired files shall degrade within their own card while other content remains usable.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.15:** Rich output shall inherit the current session's authorization, accept only workspace-relative file references, and neither accept a caller-selected session nor retain raw file or CSV bytes as presentation data. Configuration and external MCP profiles shall not advertise this tool.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.16:** Charts shall accept 1 to 100 labels and 1 to 4 equally sized series of finite numbers or nulls. Metric groups shall accept 1 to 6 label/value entries. CSV chart sources shall reject binary or invalid UTF-8 data, files over 256 KiB, missing or duplicate headers, malformed rows, invalid numeric cells, and zero or more than 100 data rows; empty numeric cells shall become gaps.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.17:** Completed presentations shall survive reload, restart, and transcript replay using accepted data. Editing or deleting a CSV source shall not change its historical chart; workspace cleanup may make a referenced file preview unavailable without removing the presentation.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.18:** On phones, rich output shall remain inline in the existing transcript, keep chart and text content within the viewport, wrap metric and legend content, and expose file actions and multi-series controls with touch targets of at least 44px. The transcript shall remain the primary vertical scroll owner.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.19:** Unrelated transcript updates shall not restart chart animation or reset file disclosure. Local legend filtering shall update only that chart and shall retain its data, axis formatting, and remaining presentation state.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.20:** Small textual comparisons shall continue to use Markdown tables. Explicit requests for charts, previews, KPIs, or metric presentations with available data shall route to the native rich-output tool instead of generating a separate application or executable artifact.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.21:** Failed rich-output calls shall retain a compact red cross without persistent inline error text. Hover or keyboard focus shall expose a short explanation. Click or keyboard activation shall open the retained error details in a popup. Missing details shall produce a localized fallback.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.22:** On phones or coarse-pointer devices, tapping the cross shall open error details in a dismissible bottom drawer. The trigger shall have a target of at least 44px. Long errors shall wrap and scroll within the surface without page overflow. Dismissal shall return focus to the trigger.
- **AC-AGENTS-AGENT-RICH-OUTPUT-001.23:** Error disclosure shall work live and after reload without changing tool status or issuing a tool call. A failed attempt followed by a successful call shall retain separate outcomes. Opening the error shall not change the successful card or its local state.

### REQ-AGENTS-AGENT-RICH-OUTPUT-002: Catalog composition

**Intent:** Let agents compose richer native presentations from a bounded
catalog, replacing the fixed block-list authoring model for new output.

#### Acceptance criteria

- **AC-AGENTS-AGENT-RICH-OUTPUT-002.1:** Agents shall be able to author version 2 presentations with ordered nesting, labelled groups, plain text, and horizontal or vertical grouping of charts, metric groups, and file previews. Different valid hierarchies shall produce the corresponding grouping and order.
- **AC-AGENTS-AGENT-RICH-OUTPUT-002.2:** Only the advertised catalog's component names and literal properties shall be accepted. Unknown components, properties, versions, actions, expressions, remote resources, executable content, and agent-selected styling shall be rejected before rendering or reading a CSV source.
- **AC-AGENTS-AGENT-RICH-OUTPUT-002.3:** A composition shall contain exactly one rooted tree with unique element identities, valid child references, no cycles or reused children, and no unreachable elements. It shall contain at most 64 elements, at most 8 levels including the root, and at most 4 chart/metric/file/table/status-list content elements in total. Accepted input and chart snapshots shall each remain bounded to 64 KiB.
- **AC-AGENTS-AGENT-RICH-OUTPUT-002.4:** Existing valid version 1 historical presentations shall retain their content, order, chart snapshots, and local interactions through the replacement. Malformed historical data shall fail closed; compatibility shall not cause a repeated tool call, CSV-source read, or transcript rewrite. Image cards may request their preview under REQ-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.
- **AC-AGENTS-AGENT-RICH-OUTPUT-002.5:** A CSV chart snapshot shall identify its exact composition element. Missing, duplicate, unexpected, or mismatched snapshots shall make the presentation unavailable rather than attach data to another chart or re-read a file.
- **AC-AGENTS-AGENT-RICH-OUTPUT-002.6:** A completed version 2 presentation shall show the same accepted hierarchy and values live and after reload or restart. Browsing history shall not re-read CSV sources, execute agent actions, or contact an external service. File previews may use session-authorized workspace reads under requirement 001 and REQ-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.
- **AC-AGENTS-AGENT-RICH-OUTPUT-002.7:** Composition shall preserve all chart, motion, file-preview lifetime, authorization, and unavailable-state behavior in requirement 001. Two distinct file elements referencing the same target shall remain independent.
- **AC-AGENTS-AGENT-RICH-OUTPUT-002.8:** On phones, horizontal groups shall become vertical groups in their declared order. Desktop and phone views shall share content and local state; changing viewport shall not reset an expanded preview or chart filter, or overwrite a desktop preference.
- **AC-AGENTS-AGENT-RICH-OUTPUT-002.9:** New compositions shall work through the existing trusted Kandev tool identity in both task and Office transcripts. A foreign tool with a similar name or an incomplete provider envelope shall not select this presentation.
- **AC-AGENTS-AGENT-RICH-OUTPUT-002.10:** Agent-facing discovery shall identify the catalog, accepted properties, limits, and valid version 2 examples for inline charts, CSV charts, and metrics. Task and Office guidance shall direct agents to the new authoring shape and retain explicit-request routing within the existing instruction budgets.
- **AC-AGENTS-AGENT-RICH-OUTPUT-002.11:** Rendering a composition shall not create task mutations or agent callbacks. File disclosure, opening, and chart legend filtering shall remain host-owned. Images may expand automatically under REQ-AGENTS-IMAGE-PREVIEW-DEFAULTS-001; manual toggles, opening and filtering shall be initiated by the user.

### REQ-AGENTS-AGENT-RICH-OUTPUT-003: Structured evidence components

**Intent:** Let agents present tabular evidence, check results, and prominent
findings through the existing native catalog without user-supplied UI recipes.

#### Acceptance criteria

- **AC-AGENTS-AGENT-RICH-OUTPUT-003.1:** Agents shall be able to compose Table, StatusList, and Callout with the existing catalog components through the same rich-output tool and renderer. These additions shall preserve the meaning and replay of previously accepted version 1 and version 2/catalog 1 presentations.
- **AC-AGENTS-AGENT-RICH-OUTPUT-003.2:** A Table shall show ordered, labelled columns and rows supplied inline or selected from a workspace CSV. It shall accept 1 to 8 uniquely keyed columns, 0 to 100 rows, and exactly one cell per column in each row. Labels and column keys shall contain 1 to 120 Unicode code points; cells shall contain 0 to 500. Cells shall render as plain text and preserve empty values without numeric coercion or markup interpretation.
- **AC-AGENTS-AGENT-RICH-OUTPUT-003.3:** A CSV Table shall retain only the selected columns as an accepted snapshot associated with its exact element identity. Editing, deleting, or losing access to the source after completion shall not change its cells or trigger a read when viewing history or reloading.
- **AC-AGENTS-AGENT-RICH-OUTPUT-003.4:** CSV Tables shall reject non-relative sources, binary or invalid UTF-8 data, files over 256 KiB, missing, blank or duplicate headers, unavailable selected columns, malformed row widths, oversized cells, and more than 100 rows. Header-only files shall produce an empty table. Invalid composition shall fail before any source read; a source failure shall fail the tool without presenting a partially resolved report.
- **AC-AGENTS-AGENT-RICH-OUTPUT-003.5:** Desktop Tables shall associate each cell with its column header. Phone Tables shall present each row as ordered labelled fields, preserving every column and value without page-level horizontal scrolling. Empty tables shall show a localized empty state. Long content shall wrap, and the transcript shall remain the primary vertical scroll owner.
- **AC-AGENTS-AGENT-RICH-OUTPUT-003.6:** A StatusList shall display 0 to 20 ordered items with a plain-text label, optional explanation, and one of passed, failed, running, pending, or skipped. Each status shall have a visible localized label and an icon, so color alone never conveys meaning. Item labels shall contain 1 to 120 code points and explanations at most 500; an empty list shall show a localized empty state.
- **AC-AGENTS-AGENT-RICH-OUTPUT-003.7:** StatusList values shall be static findings supplied by the agent. They shall not claim backend-verified status, poll a live task or execution, mutate workflow state, or expose completion controls. Running and pending shall remain snapshot labels rather than promises of automatic updates.
- **AC-AGENTS-AGENT-RICH-OUTPUT-003.8:** A Callout shall display a plain-text message of 1 to 500 code points, optional title of at most 120, and an info, success, warning, or error kind. The kind shall have a visible localized label and icon. Historical notes shall not be announced as newly occurring live alerts or treated as tool failures.
- **AC-AGENTS-AGENT-RICH-OUTPUT-003.9:** All three additions shall remain inline in task and Office conversations, respect host theme and localization, use no agent-authored actions or styling, and preserve neighboring chart filters and file disclosures across unrelated updates or viewport changes.
- **AC-AGENTS-AGENT-RICH-OUTPUT-003.10:** Table and StatusList shall each count toward the shared four-content-element bound. Callout shall count toward the existing node, depth, and byte bounds as explanatory text. Inputs and combined CSV snapshots shall each remain within 64 KiB. Unknown properties, statuses, kinds, malformed cells, and missing, duplicate, unexpected or mismatched snapshots shall be rejected without silently repairing data.
- **AC-AGENTS-AGENT-RICH-OUTPUT-003.11:** Task and Office discovery shall expose component descriptions, limits, and valid inline-table, CSV-table, status-list and callout examples. Guidance shall explain when structured tables, check results and prominent findings are useful while retaining prose or Markdown for small textual answers. Users shall not need to supply tool names, schema fields or component names.
- **AC-AGENTS-AGENT-RICH-OUTPUT-003.14:** The short tool description shall list every accepted StatusList status: passed, failed, running, pending, and skipped. Discovery shall retain a valid StatusList example using passed. Unknown statuses shall remain invalid.
- **AC-AGENTS-AGENT-RICH-OUTPUT-003.12:** When rich-output arguments violate a component's declared shape, the error shall identify the offending element or field and its expected shape. For a recognized component and an unambiguously selected source form, the error shall not demand properties from other component or source forms, or label accepted properties as unknown. Missing or unknown component names and ambiguous source forms shall receive an explicit selection error or separately labelled alternatives rather than contradictory merged requirements.
- **AC-AGENTS-AGENT-RICH-OUTPUT-003.13:** Rejected rich-output calls shall retain strict validation and perform no workspace read or partial presentation. Feedback shall omit submitted property values and raw file content; schema-declared names, types, allowed values and bounded examples may explain a correction. A corrected valid call shall succeed through the same tool without a relaxed schema, new conversation or user-supplied UI recipe.

## Scope and compatibility

The replacement baseline includes Stack, Group, Text, Chart, Metrics, and
FilePreview. The catalog also includes Table, StatusList, and Callout.
Grouping and Text extend authoring; chart, metric, and file content retain the
existing limits and semantics. Version 1 remains a historical replay input. New tool calls use version 2;
stale version 1 calls receive an actionable error before any source read. Catalog extensions require explicit reviewed
contracts rather than exposing a general-purpose UI library automatically.

## Out of scope

- MCP Apps, iframes, arbitrary HTML/JavaScript/CSS, and external UI resources.
- Progress indicators, forms, agent-authored action buttons, callbacks, editors, maps,
  plugin-contributed components, and live data subscriptions.
- Partial tool-argument rendering, JSONL patch streaming, and post-completion
  agent updates to an existing presentation.
- A new agent transport, AI SDK chat runtime, artifact database, workspace file
  API, transcript virtualization, or a change to chart animation defaults.

## Implementation plans

- [Rich-output failure disclosure](../../../plans/rich-output-failure-disclosure/plan.md)

- [Copilot rich-output identity repair](../../../plans/copilot-rich-output-identity/plan.md)

- [Image preview defaults](image-preview-defaults.md) and its
  [implementation plan](../../../plans/image-preview-defaults/plan.md) define
  the image exception to explicit expansion.

- [CSV header BOM repair](../../../plans/rich-output-csv-header-bom/plan.md)
- [Rich-output authoring recovery](../../../plans/rich-output-authoring-recovery/plan.md)
- [Structured evidence catalog extension](../../../plans/rich-output-evidence-components/plan.md)

- [json-render replacement](../../../plans/json-render-rich-output/plan.md)
- [Original implementation](../../../plans/agent-rich-output/plan.md)
- [Chart performance repair](../../../plans/rich-output-chart-performance/plan.md)
- [File-preview lifetime repair](../../../plans/rich-file-preview-targets/plan.md)
