# ADR-2026-08-14-kandev-native-agent-rich-output: Keep Agent Rich Output Host Native

**Status:** accepted (amended 2026-08-15)
**Date:** 2026-08-14
**Area:** backend, frontend, protocol, security

## Context

Agents need to present charts, metrics, and workspace files more directly than
plain prose allows. Kandev could render its own closed semantic blocks, repair
the generic MCP/ACP content pipeline, or implement MCP Apps and host arbitrary
sandboxed HTML. Those choices have materially different portability, visual
consistency, persistence, and security costs.

## Decision

1. The first rich-output surface is a built-in task and Office MCP tool,
   `show_rich_output_kandev`, with a versioned, closed semantic schema.
2. Kandev owns rendering. Agent input contains data and workspace-relative
   references only; it cannot provide HTML, JavaScript, CSS, remote resources,
   colors, animations, or arbitrary layout.
3. Version 1 supports file, chart, and metrics blocks. Ordinary Markdown owns
   small tables and prose.
4. Existing tool-call arguments at `metadata.normalized.generic.input` are the
   canonical persisted presentation for inline blocks. A CSV-backed chart is
   the narrow exception: the handler reads the workspace file once and returns
   a bounded, versioned snapshot of normalized labels and numeric or `null`
   values. Existing tool-result persistence stores that snapshot at
   `metadata.normalized.generic.output`; replay never re-reads the CSV.
5. Heavy bytes remain workspace-owned. File blocks and CSV chart sources accept
   only relative paths and optional repository identity. File previews still
   fetch content only after explicit user action. CSV parsing persists no raw
   bytes, but its bounded normalized snapshot remains after workspace cleanup.
6. The frontend repeats validation at its trust boundary and renders completed
   rich-output calls outside generic tool-activity grouping.
7. Portable MCP content and MCP Apps remain separate future capabilities. They
   must not weaken or become implicit escape hatches in this native contract.

## Consequences

- Kandev can guarantee its visual language, responsive behavior, localization,
  and accessibility for the supported blocks.
- Inline payloads and normalized CSV snapshots remain small and replayable
  without a schema migration.
- File content retains task-workspace lifecycle rather than gaining an implied
  artifact-retention guarantee.
- Other MCP hosts cannot reproduce the Kandev-native presentation from this
  tool unless they adopt the same contract; they still receive a text result.
- New block types require an explicit schema version-compatible addition,
  native desktop/mobile composition, and validation coverage.
- Generic MCP images, audio, resources, and MCP Apps still require independent
  transport, persistence, capability, and security work.

## Alternatives Considered

### Repair portable MCP content first

Rejected for the first slice because standard content blocks carry media and
resources but do not define a tasteful chart or metric composition. Kandev
also currently drops non-text content before durable transcript persistence,
which would enlarge the initial change substantially.

### Implement MCP Apps first

Rejected because `ui://` resources and sandboxed applications add resource
capabilities, iframe lifecycle, CSP, permissions, origins, message bridging,
accessibility, and theming policy. That flexibility is justified for true
mini-apps, not for the three common display primitives in version 1.

### Allow arbitrary HTML or a generic component schema

Rejected because sanitization does not create visual consistency, mobile
quality, accessibility, or bounded interaction. A generic component escape
hatch would turn agent output into an unreviewed UI framework.

### Store file bytes in message metadata

Rejected because large base64 values would bloat SQLite and message replay,
duplicate workspace ownership, and create an unclear cleanup contract.

### Resolve CSV again on every replay

Rejected because task cleanup, edits, or branch changes could make conversation
history disappear or silently change. Resolution therefore happens during the
tool call, and replay consumes the persisted normalized snapshot.

### Require agents to inline CSV rows

Rejected because it spends model context on mechanical extraction, increases
schema-call failures, and discourages agents from using charts for existing
workspace data. A closed path-and-column descriptor keeps agent input compact
while preserving the same bounded native renderer.
