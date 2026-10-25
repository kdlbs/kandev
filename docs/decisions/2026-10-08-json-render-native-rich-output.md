# ADR-2026-10-08-json-render-native-rich-output: Use json-render for native agent rich output

**Status:** accepted
**Date:** 2026-10-08
**Area:** frontend, backend, protocol

## Context

Kandev's native rich-output contract currently accepts a fixed list of chart,
metric, and file blocks. Agents cannot compose labelled groups or layouts, and
the frontend owns a custom block dispatcher and duplicated schema checks.
The user selected json-render as the replacement and explicitly chose catalog
composition rather than a renderer-only migration.

The original [native rich-output decision](2026-08-14-kandev-native-agent-rich-output.md)
established session authorization, native rendering, bounded data, and durable
chart snapshots. Those boundaries remain useful when composition expands.

## Decision

1. Use json-render's catalog and React Renderer as the sole rich-output
   composition engine. Kandev defines the approved native catalog and its
   properties; agents author a versioned composition through the existing tool.
2. Initially expose grouping and plain text with existing charts, metric groups,
   and workspace file previews. Kandev owns responsive geometry, theme,
   accessibility, localization, and local controls. Library features are not
   automatically public agent capabilities.
3. Keep the agent tool read-only. The initial catalog exposes no agent-authored
   actions, dynamic expressions, executable resources, or data subscriptions.
   MCP Apps remains a separate future capability.
4. Generate the backend component schema from the TypeScript catalog and check
   it for drift. Backend and frontend enforce the same bounded literal-tree
   contract before resolving or rendering data.
5. Convert historical version 1 input and stored snapshots to the same library rendering
   path. Retain only compatibility decoding, not a parallel custom renderer,
   and preserve existing message metadata without rewriting history. New tool
   authoring uses version 2 at completed delivery; remove the old writer,
   legacy styling mode, unused MIME hint, and block-shaped native interfaces.

The replacement and its reviewed Table, StatusList and Callout extension are
implemented locally, with verification recorded in their
[replacement](../plans/json-render-rich-output/plan.md) and
[evidence-component](../plans/rich-output-evidence-components/plan.md) work orders.
The owning requirements and system design are active/current. This records
completed implementation, not release publication.

## Consequences

- Agents gain bounded composition without selecting arbitrary browser code or
  styling. Native components can retain proven chart and file behavior.
- Kandev adopts a library dependency and must verify updates against its catalog,
  schema export, stable component identities, and persistence fixtures.
- A new authoring version requires prompt, schema, snapshot, and compatibility
  work. Keeping one rendering path avoids two presentation engines.
- The Go backend remains a validator and data resolver. It does not require a
  JavaScript runtime or a new agent transport.

## Alternatives considered

### Keep the custom block renderer

Retains the smallest current dependency surface but does not meet the selected
catalog-composition goal and leaves Kandev maintaining the composition engine.

### Use json-render internally with the old authoring contract

Changes rendering infrastructure without letting agents compose interfaces.
The user explicitly selected the broader catalog-composition outcome.

### Adopt A2UI

Offers a portable declarative protocol, but portability across renderer
frameworks is not required for this replacement. The user selected json-render
for Kandev's current React surface.

### Host MCP Apps

Supports independently supplied interactive applications, with resource,
sandbox, permission, and bridge lifecycles. That addresses a different outcome
from replacing Kandev's own native rich-output composition.

## Contract owners

Agents owns the [requirements](../specs/agents/requirements/agent-rich-output.md)
and [system design](../specs/agents/system-design/agent-rich-output.md).
Delivery is defined in the [plan](../plans/json-render-rich-output/plan.md).
