---
status: current
system: agents
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
  - REQ-AGENTS-AGENT-RICH-OUTPUT-003
---
# Agent rich-output system design

## Purpose and boundaries

The replacement baseline and structured evidence extension are implemented.
The 2026-10-10 evidence package adds Table, StatusList and Callout while
preserving existing component and historical contracts.

Replace the fixed rich-output composition engine with json-render and expose
catalog composition to agents through `show_rich_output_kandev`. Agents owns
the versioned authoring, validation, and replay contract. Task and Office
transcripts consume the same native renderer. The implementation uses one json-render path for new compositions and retained
version 1 history.

The [UI MCP tool-results design](../../ui/system-design/kandev-mcp-tool-results.md)
continues to own shared identity matching, result selection, and transcript
renderer dispatch. Existing workspace-file authorization and lifecycle remain
with Workspaces. This design does not introduce MCP Apps, new transport,
agent callbacks, or raw artifact retention.

Decision: [Use json-render for native agent rich output](../../../decisions/2026-10-08-json-render-native-rich-output.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-AGENT-RICH-OUTPUT-001` | Native components, Legacy compatibility, Persistence and CSV snapshots, Provider identity, Responsive composition |
| `REQ-AGENTS-AGENT-RICH-OUTPUT-002` | Catalog and dependencies, Version 2 contract, Validation and generated schemas, Single rendering path, Failure and recovery |
| `REQ-AGENTS-AGENT-RICH-OUTPUT-003` | Structured evidence extension, CSV table snapshots, Structured evidence presentation and discovery, Evolution and authoring feedback |

## Components

- `apps/backend/internal/mcp/server/rich_output.go` embeds the generated closed
  version 2 schema, validates tree/path/size semantics, and handles authoring.
- `rich_output_csv.go` resolves bounded CSV sources through
  `workspace.file.get` using the server's current session. Results identify
  `resolved_charts[].element_id` and carry input/catalog versions.
- `tool_argument_validation.go` compiles schemas through jsonschema/v6 and
  provides a discovery-refresh error for stale v1 authoring. The shared
  `toolschema.CheckPortableRoot` rejects root schema combinators.
- `apps/web/lib/rich-output/catalog.ts` owns the Zod component contracts;
  `validate.ts` guards literal props and graph semantics; `normalize.ts` binds
  exact snapshots or converts historical inputs using `legacy.ts`.
- `RichOutputRenderer` mounts the real library Renderer with one stable native
  registry. It owns translated terminal states and session/file host context.
- Native `Chart`, `Metrics`, and `FilePreview` accept catalog-derived props.
  `useChartPlotVisibility`, `useRichOutputChartAnimations`, and
  `useWorkspaceFilePreview` retain existing local behavior.
- `KANDEV_RENDERERS.show_rich_output` and `isRichOutputMessage` select the trusted
  native tool and keep its completed result outside ordinary activity groups.

## Catalog and dependencies

Pin `@json-render/core` and `@json-render/react` to 0.21.0 for the initial
implementation, with a direct compatible Zod 4 dependency (4.3.6). The published
React peer range is `^19.2.3`, matching this worktree's React 19.2.3. Changing these pins requires rerunning the catalog,
schema, and full-consumer compatibility tests. Library versions are not public
catalog versions.

Place the pure catalog definition and payload types under
`apps/web/lib/rich-output/`. Define it with `defineCatalog` from
`@json-render/core` and `schema` from `@json-render/react/schema`. Use typed,
closed Zod property schemas. It must not import React components or app state.

| Component | Literal properties | Children |
| --- | --- | --- |
| Stack | `direction`: `vertical` or `horizontal` | 1 or more ordered children |
| Group | optional plain-text `title` (up to 120 code points) | 1 or more ordered children |
| Text | `text` (1 to 500 code points) | None |
| Chart | Catalog chart properties, with inline data or a CSV source | None |
| Metrics | Current `items` property | None |
| FilePreview | `path`, optional `repo`, `title`, `caption` | None |

The catalog has no agent-callable actions, functions, styling properties,
remote resources, forms, or dynamic bindings. Local preview and chart controls
are implemented by their native components and are not catalog events. Do not
register the library's stock component catalog wholesale.

Agent strings are product data and render as plain text, including text that
contains markup characters. Host controls and unavailable/loading copy use
existing translations. Catalog descriptions are English agent-facing metadata,
not browser copy; new source must declare justified i18n exemptions rather than
calling translation at module scope. If host copy changes, update all required
locale catalogs and generate Traditional Chinese with the repository helper.

## Version 2 contract

Keep the tool name, read-only annotation, and task/Office profile visibility.
New calls require this closed envelope:

```json
{
  "version": 2,
  "catalog_version": 1,
  "title": "Build health",
  "description": "Current workspace verification",
  "spec": {
    "root": "report",
    "elements": {
      "report": {
        "type": "Stack",
        "props": { "direction": "vertical" },
        "children": ["overview", "evidence"]
      },
      "overview": {
        "type": "Group",
        "props": { "title": "Checks" },
        "children": ["counts", "interpretation"]
      },
      "counts": {
        "type": "Metrics",
        "props": { "items": [{ "label": "Passed", "value": "38" }] },
        "children": []
      },
      "interpretation": {
        "type": "Text",
        "props": { "text": "All focused checks passed." },
        "children": []
      },
      "evidence": {
        "type": "FilePreview",
        "props": { "path": "reports/build.json" },
        "children": []
      }
    }
  }
}
```

The envelope permits only `version`, `catalog_version`, `title`, `description`,
and `spec`. Version 2 requires catalog version 1. The spec permits only `root`
and `elements`. Each element permits only `type`, `props`, and `children`;
`children` is required and is empty on leaves. Named slots, `on`, `watch`,
`repeat`, `visible`, initial state, and dynamic expression objects are rejected,
even when the underlying library supports them.

Element IDs use `[A-Za-z][A-Za-z0-9_-]{0,63}`. Root and child references use the
same rule; reserved prototype names (`constructor`, `prototype`, `__proto__`)
are forbidden. Require a tree: one root, every node reachable exactly once,
every child present, no cycle, and no reused child reference. Bound the graph to
64 nodes and 8 levels including root, and count at most 4 Chart/Metrics/FilePreview
nodes across the baseline graph. With the extension, Table and StatusList also
count toward that same four-node limit; Callout is explanatory text like Text.
Text and container nodes do not bypass the global
node/depth/payload bounds.

Retain the current 64 KiB input bound, title length 1 to 120, and optional
500-character description. Text limits count Unicode code points consistently
at both boundaries. Preserve the version 1 content limits: paths up to 1024,
repo up to 255, file title up to 120, caption up to 500;
chart title up to 120, summary up to 500, 1 to 100 nonempty labels up to 120,
1 to 4 series with labels up to 120 and matching finite numeric/null arrays;
metrics 1 to 6 entries with label up to 120, value up to 200, detail up to 500.
CSV descriptors retain the exact path/repo/column mapping and label limits.

## Validation and generated schemas

The TypeScript catalog is the source of component names and property contracts.
The deterministic generator under `apps/web/scripts/` exports property schemas
from the catalog components and applies the closed Kandev envelope and literal
restrictions. Its `spec` declares only `root` and `elements`; library exports
cannot widen that boundary. It writes the embedded backend schema and examples.
Check artifacts for drift in the frontend build/CI path. `--check` does not write.

Do not change the shared portable-schema contract to accommodate this feature.
The MCP root remains `type: object` with no root combinators. The completed
backend advertises only version 2 and catalog version 1. The compatibility period
is the implementation sequence: the real renderer lands before new authoring;
at completed delivery, reconnecting clients discover the new schema. Stale
version 1 calls fail before any read with guidance to refresh discovery and use
version 2. Historical decoding remains independent of tool acceptance. Nested
schemas may contain combinators.

Use the existing Go schema validator for shape and property validation. Add only
host-specific semantic checks for version selection, byte/code-point limits,
graph eligibility, relative paths, finite numbers, series lengths, and leaf
count. The frontend repeats the closed envelope/graph checks and uses
`catalog.validate` for the catalog shape. Library validation or rendering must
never be treated as authorization or as graph-cycle protection. Reject rather
than strip unknown keys or auto-repair malformed trees.

A shared conformance corpus under the backend server's `testdata/` covers
accepted/rejected inputs. Both Go and Vitest consume it; snapshot binding is
covered by producer and full-consumer tests.
Cases include every catalog entry, unknown fields, cross-version fields,
Unicode boundaries, invalid graphs, payload bounds, forbidden state/events,
CSV path rules, and mixed inline/CSV charts. Generated examples must pass both
validators and compile through Kandev's portable-root validator.

## Persistence and CSV snapshots

Retain existing message metadata as the durable source: input at
`metadata.normalized.generic.input` and result at
`metadata.normalized.generic.output`. There is no new table or message rewrite.
Inline version 2 presentations replay from accepted arguments and return the
existing short text acknowledgement.

Before reading any source, validate the entire composition. For each CSV Chart,
resolve once through the existing session-authorized workspace read. Reuse the
existing per-call `(repo, path)` cache and CSV parsing rules, but associate the
version 2 result with the element ID, not traversal position:

```json
{
  "version": 2,
  "catalog_version": 1,
  "resolved_charts": [
    {
      "element_id": "latency",
      "labels": ["2026-10-08"],
      "series": [{ "label": "p95 (ms)", "values": [29.4] }]
    }
  ]
}
```

The result contains exactly one entry for each CSV Chart and no entries for
other nodes. Preserve expected series order and display labels, including the
column-name fallback when a CSV mapping omits `label`. Reject wrong versions,
missing or duplicate IDs, unexpected IDs, incorrect lengths, or mismatched
series labels. Validate the complete snapshot against the 64 KiB bound before
success. Do not persist raw CSV bytes. Publish the same snapshot as MCP
structured content and its equivalent JSON text fallback; verify both through
the real result-extraction path.

Frontend normalization replaces each CSV descriptor with its accepted inline
labels/series in a derived immutable spec, without mutating stored input. A
missing result never triggers a CSV read during replay. File previews remain workspace references;
[image defaults](image-preview-defaults.md) amend their disclosure lifecycle.

## Legacy compatibility

Keep a small, closed version 1 decoder and its validation tests. It checks the
old input and block-indexed snapshots, then converts their resolved blocks to a
json-render spec. Use deterministic IDs such as `legacy-block-0` and preserve
order. Convert the old title/description into the host-owned outer presentation
and blocks into a vertical Stack using the same spacing as new compositions.
There is no legacy presentation mode.

Keep version 1 schema/types only inside the historical decoder. Remove the
backend v1 authoring schema and block-indexed result writer at completed delivery.
The decoder translates block-indexed stored snapshots to element-addressed
resolved specs. It may not render JSX or call a parallel composition engine.
Do not retain `RichOutputBlockView` as a fallback renderer.

## Single rendering path

`RichOutputRenderer` remains the registry entry, outer title/description shell,
terminal-state gate, and localized fallback. Memoize normalization by persisted
arguments and result. On success, pass the validated, resolved spec to
`Renderer` from `@json-render/react` using a stable `defineRegistry` registry.
Both input versions must pass through this actual library component.

Keep catalog/types below UI in `lib/rich-output/`; keep concrete registry
components beside the existing rich-output components. A host React context
supplies session identity and the existing file-open callback. Do not rebuild
registry component identities when session or transcript state changes, and do
not pass the entire Zustand store as json-render state. Provide only the library
contexts the pinned Renderer requires, with no model-authored handlers,
navigation function, or shared writable state.

The Stack, Group, and Text components render validated composition primitives.
Chart, Metrics, and FilePreview use catalog-derived props directly. Preserve
native chart and workspace-file behavior but remove block-shaped component
interfaces and the unused MIME hint. Old block types are decoder-private.
Keep props-to-domain adaptation memoized so unchanged charts preserve derived
objects and their visibility/motion state. Put an error boundary around the
presentation; malformed component failures show the localized unavailable state.

## Native components and local state

The native Chart component retains direct Recharts axes, grid, and tooltip children, local
legend filtering, formatting helpers, deferred plot mounting, and motion
preferences. Library tree composition must not insert wrappers inside the
Recharts plot's discovery boundary.

Within the FilePreview registry adapter, key the native FilePreview component by
the serialized `(sessionId, repo, path)` tuple. Its element identity separates
two cards referencing the same target. A tuple change unmounts only that card;
`useWorkspaceFilePreview` invalidates the old request's settlement. Stable
identity/target preserves disclosure, success caching, and explicit failed-read
retry through title changes, unrelated rerenders, and viewport changes.

Do not key the whole presentation or registry by viewport/session/file target.
That would reset unrelated chart/filter/file state. Native controls keep their
existing translated accessible names and callbacks; json-render event wiring is
not needed for these local controls.

## Provider identity

Preserve existing normalized MCP identity and arguments. Codex uses explicitly
marked MCP envelopes with `{server, tool, arguments}`; Cursor and Grok use
complete `{providerIdentifier, toolName, args}` envelopes. The generic name
retains `<provider>/<tool>`. Historic `kandev: <tool>_kandev` titles and nested
`raw_input.args`/`arguments` retain their existing compatibility helpers.

| Evidence shape | Intended behavior | Verification |
| --- | --- | --- |
| Canonical trusted Kandev tool input/result | v1 and v2 share the library renderer | Full-consumer renderer fixtures and MCP E2E |
| Codex marked envelope | Preserve actual tool identity and composition input | Existing adapter tests plus v2 renderer envelope fixtures |
| Cursor/Grok complete envelope or historic Kandev title | Resolve stored arguments and result without rewriting history | Existing normalization tests plus v1/v2 renderer fixtures |
| Claude/bare Kandev tool title accepted by existing matcher | Use existing matcher; no new provider inference | Existing identity tests and renderer fixture |
| Foreign provider, incomplete envelope, or unknown tool | Generic activity; never native composition | Matcher and full-consumer negative controls |

Do not claim a live provider run from fixture coverage. Any new provider shape
requires evidence and a separately reviewed normalization change.

## Responsive composition

Entry point is the completed presentation in task or Office chat. Reuse the
existing inline transcript and `MobileFileViewerPanel` as shipped exemplars.
The presentation is conversation evidence; opening a file continues to use the
focused mobile viewer rather than a new embedded desktop workbench.

Hierarchy is outer title, description, then ordered catalog groups. A horizontal
Stack uses equal flexible children on desktop and becomes vertical below the
canonical 768px phone boundary. Use `useResponsiveBreakpoint` for presentation
choice and preserve the same mounted elements and state; do not mount two
viewport trees. Order never changes. Container children use `min-w-0` and chart
width remains bounded to its container. Metrics and legends keep wrapping.

The transcript is the primary vertical scroll owner. The existing bounded
expanded-file preview may scroll locally. File open and preview controls, and
interactive legend buttons, use 48px nominal phone/coarse-pointer targets so
pressed scaling still leaves at least 44px, without enlarging ordinary
fine-pointer desktop controls. Do not add viewport
fixed overlays for successful cards. Failed-call disclosure follows the
[failure design](agent-rich-output-failures.md). Keyboard focus, semantic
headings, plain-text chart summaries, and translated host states remain available.

## Failure and recovery

Validation fails before CSV access. Pending calls never render partial specs.
The [failure design](agent-rich-output-failures.md) owns failed-call disclosure
and enum discovery. Malformed completed presentations remain unavailable.
File-read errors remain local and explicitly retryable. History is never repaired
or regenerated automatically.

## Verification and delivery

The [plan](../../../plans/json-render-rich-output/plan.md) maps each AC to
conformance, full-consumer, backend, and browser evidence. It includes distinct
phone composition and legacy replay controls. Rendering tests must use the real
catalog, normalizer, registry, and json-render Renderer, with only external
workspace transport mocked. Keep the existing file-target, chart scheduling,
motion, result-extraction, and trusted-identity controls.

Publish public guidance only with implementation. The completed replacement baseline
removes the fixed block dispatcher and advertises version 2 composition;
retained version 1 code must be named and scoped as historical compatibility. Documentation
and scoped AGENTS guidance must distinguish contract adaptation from rendering.
No runtime feature flag or new operational metric is introduced by this package.

## Evolution and authoring feedback

Catalog version is independent of the dependency version. Additive optional
properties/components may extend catalog version 1 only when all historical
fixtures retain their meaning. Renames, removals, changed required properties,
and changed interpretation require a new catalog version and a narrow reader
migration. Never silently reinterpret stored reports or run old actions on replay.
Every dependency update runs generated-schema drift checks, shared conformance,
real-renderer state tests, and historical fixtures.

Shape errors use the compiled schema and validation tree to select relevant
diagnostics before formatting. For discriminated element unions, match the
schema's constant `type`; for source unions, use uniquely present required
source properties such as `rows` or `csv`. Never merge missing/unknown fields
from incompatible alternatives. If selection is unresolved, label alternatives
or report the schema's allowed discriminator values. Both/neither source forms
remain invalid; overlapping `oneOf` branches remain an ambiguity, not success.
Preserve independent sibling and root failures. Report nested type errors at
their actual pointer, with schema-derived expected types/required object fields.
Do not echo rejected values or use raw validator error strings. Keep existing
non-union diagnostics and task-scope guidance. The shared MCP wrapper owns
this diagnostic projection; the full compiled schema remains authoritative.
Do not create a second rich-output validator, choose an arbitrary closest
branch, accept aliases, repair arguments or change the portable-root contract.
Host graph/source errors identify the element or path and the violated bound.
Do not log raw report or file contents. Structured evidence components are the
reviewed extension described below. Any future authorized action contract needs
separate design; these additions do not introduce actions.

## Structured evidence extension

Keep version 2/catalog version 1: this is an additive extension and existing
fixtures retain exactly their meaning. Do not edit the historical decoder,
renumber old components, install a stock catalog, or add a second renderer.
The existing json-render catalog remains the schema source and its registry the
only composition path. Add native adapters for Table, StatusList, and Callout;
reuse `@kandev/ui/table`, `@kandev/ui/badge`, and `@kandev/ui/alert` where suitable.
The published stock json-render definitions were assessed: Table and Alert
provide useful precedents, but their bundled UI primitives and unconstrained
props do not replace Kandev's bounds or phone row composition. No dependency
addition is necessary. This extends the existing ADR rather than creating a new
architecture decision.

| Component | Closed literal props | Semantics |
| --- | --- | --- |
| Table | optional `title` (0..120); `columns` (1..8) of `{key, label}` (each 1..120); either `rows` (0..100 arrays of 0..500-code-point strings) or `csv: {path, repo?}` | Column keys unique; row width exactly matches columns; `key` selects CSV headers; no children |
| StatusList | optional `title` (0..120); `items` (0..20) of `{label, status, detail?}` | Status enum passed/failed/running/pending/skipped; label 1..120, detail 0..500; no children |
| Callout | `kind`: info/success/warning/error; `text` (1..500); optional `title` (0..120) | Static plain-text finding; no children |

Table's inline and CSV branches are exclusive, including in stored input. A
strict object forbids arbitrary presentation/formatting properties. CSV paths
reuse the current path/repo rules, including `.csv`, 1024-code-point path and
255-code-point repo limits. CSV header matching reuses the existing normalized
header index (trimmed headers and BOM handling); selected keys must match that
index exactly and cannot be repeated. Duplicate display labels are permitted
because distinct keys identify columns. Rows are ordered arrays to avoid
arbitrary object keys and backend/frontend number formatting differences.
Numeric-looking text, spaces, quotes, formulas, empty cells, and markup remain
literal string data. A table does not provide export or formula execution.

Extend `validate.ts`, generated schema, Go semantic guards and the shared
conformance corpus together. The generator's portable object root, graph rules,
Unicode counts and byte bounds are unchanged. Table and StatusList join the
existing content-count predicate; Callout remains text-like. Ensure a Table's
row widths and column uniqueness are checked on producer and consumer, not
only in the renderer. Unknown statuses/kinds are shape errors, not fallback
variants. Keep the old 39-case corpus and historical controls in the suite.

## CSV table snapshots

Extend the existing resolver under `internal/mcp/server/` with a table-specific
parser, using the same per-call `(repo, path)` file cache and authorized
`workspace.file.get`. After bounded UTF-8 admission, both parsers share reader
construction that removes exactly one file-leading UTF-8 BOM before strict CSV
tokenization, including a quoted first header. Keep existing header indexing
and row-width checks; never enable `LazyQuotes`, strip embedded U+FEFF, or repair
malformed quotes. Retain chart-only finite numbers, null gaps and minimum rows.
Tables allow header-only sources and preserve selected cells without conversion.

Validate the whole composition before any file access, then resolve every CSV
Chart and Table. File content remains ephemeral. A failed read/parse or oversized
combined snapshot fails the entire tool; it must never return success with a
subset of resolved elements. All reads are read-only; no rollback is needed.

Preserve the chart-only result envelope byte structure and add an optional
`resolved_tables` collection for CSV Tables. Each entry is:

```json
{
  "element_id": "services",
  "columns": [
    {"key": "service", "label": "Service"},
    {"key": "monthly_cost_usd", "label": "Monthly cost (USD)"}
  ],
  "rows": [["API", "420"], ["Payments", "680"]]
}
```

The containing result retains `version: 2` and `catalog_version: 1`. A new result
omits a collection when it has no corresponding CSV elements. Chart-only stored
results remain accepted unchanged; table-only results need no chart collection.
Combined results contain both. A result with no CSV sources remains the existing
short acknowledgement. Limit each collection to the applicable source count
and the combined result to 64 KiB; do not increase limits or silently truncate.
Publish identical accepted data via structured content and JSON text fallback.

Normalize against the exact expected CSV element sets for each type. Require
unique element IDs, no extra entries, all requested sources present, matching
input/result versions, and the exact Table column keys, labels and order.
Reject wrong-type binding, invalid cells, row widths or oversized results.
Normalize immutably into inline Table props after removing `csv`; revalidate
resolved props before handing them to the real Renderer. A non-CSV element
cannot acquire source data from an unsolicited snapshot. No replay read or
backend migration is allowed. New table snapshot normalization should be a
small helper, not another general rendering/transport abstraction.

## Structured evidence presentation and discovery

Desktop Table uses semantic headers/cells, wraps text and may scroll locally
horizontally if eight columns cannot fit. Phone composition uses ordered row
cards with labelled definition-list fields and every cell visible. Reuse
`useResponsiveBreakpoint` and shared validated columns/rows; replace only the
stateless Table presentation. Do not remount the whole report or neighboring
stateful charts/files within the rich-output renderer. Task-layout switches
can remount chat; retain up to 32 report view stores per app-store owner,
scoped by authentication identity, session and persisted message ID. Use
json-render's external StateStore for host-only chart filters and file
expansion preferences, keyed by exact element and series/file target. Host
identities are derived after validation; agent bindings remain prohibited.
This bounded memory contains no file bytes and is discarded with the app
store. Restored file disclosures use the existing authorized preview read;
CSV snapshots never reread their source. Keeping only preferences avoids
retaining potentially large private files or changing global chat layouts. The closest mobile exemplar is
`apps/web/components/task/simple/task-properties.tsx`, contributing grouped
label/value presentation; the existing inline rich-output Metrics contributes
wrapping and transcript placement. This is evidence read in chat, so it needs
no drawer, navigation, fixed control, or extra safe-area ownership. The
transcript remains the vertical scroll owner, and there is no phone horizontal
scroll requirement. Empty tables and lists show localized host states. Blank
cells keep their empty value and receive a localized visible empty-cell label
on both viewports so emptiness is distinguishable from missing data.

StatusList colors badge/icon pairs green for passed, red for failed, blue for
running, amber for pending and neutral for skipped, with light/dark contrast.
StatusList uses a semantic list and stable ordered items with icon, translated
status label, agent-supplied label and optional detail. It is a snapshot, not
live task authority. Running is a static icon/label, not an indeterminate
spinner. Callout adapts the native Alert styling with a visible translated kind
label and icon, optional agent title and message, and `role="note"` instead of
an assertive live-alert role. Agent strings render as text; color is additional
information. These components have no controls, external links, callbacks,
animations, polling, or agent-selected styles. Local interactions in existing
components retain their authorization and identity semantics.

Schema examples in `lib/rich-output/examples.ts` cover inline and CSV Table,
StatusList and Callout, including a composed report. Descriptions tell agents:
use Table for multi-row/structured comparisons, StatusList for reported checks,
and Callout for a main finding, limitation or recommended investigation. Update
the tool description and both task/Office guidance without exceeding their
650-/750-rune budgets. Generate compact element/property guidance from the
catalog schema and validated examples with `generate-rich-output-catalog.ts`;
embed its checked artifact alongside the schema in the backend. Keep routing
policy separate from generated shape metadata. Put component-local examples
on the corresponding schema variants so discovery does not rely solely on
root examples. Generation must fail on drift or budget overflow, rather than
silently truncating guidance. Remove the handwritten duplicate shape recipe.
Prose and small Markdown tables remain available; do not
require every answer to become a dashboard. Discovery teaches shape and limits;
user prompts describe the question, not components or JSON.

Add deterministic integration/E2E evidence for composition, empty states,
source deletion plus reload, invalid snapshot binding, phone row labels,
no document horizontal overflow, and state retention across viewport changes.
Test real `Renderer`, registry and result selection, including both structured
and text-only provider results and existing foreign/incomplete identities.
One isolated real-agent smoke prompt asks only for a multi-service comparison,
reported verification results and the main finding; record actual tool choice,
model and rendering. One successful run is evidence, not a guarantee across
all providers. If the agent chooses prose or another subset, record the result
and improve discovery within this approved boundary; do not hide component
recipes in the user request. Use existing isolation and real-Codex authorization
from this session, preserve the main :9998 instance, and never use current task
MCP creation APIs as implementation workers. The authoring-recovery
[plan](../../../plans/rich-output-authoring-recovery/plan.md) adds deterministic
malformed-call/correction tests and repeated natural-prompt plus controlled
recovery trials on Sol and Luna. Record all attempts, model routing and rejected
calls; backend advertisement is not proof of what a client exposed to its model.
Failed trials leave agent usability open even when renderer checks pass.

## Primary dependency evidence

Checked 2026-10-08 against published package metadata and 0.21.0 declarations:
[core](https://registry.npmjs.org/@json-render/core/0.21.0),
[React](https://registry.npmjs.org/@json-render/react/0.21.0),
[catalog](https://json-render.dev/docs/catalog),
[registry](https://json-render.dev/docs/registry), and
[specs](https://json-render.dev/docs/specs).
The published APIs include `defineCatalog`, `catalog.validate`,
`catalog.jsonSchema`, `defineRegistry`, and `Renderer`.

## Related decisions

- [json-render replacement](../../../decisions/2026-10-08-json-render-native-rich-output.md)
- [Original native trust and persistence boundary](../../../decisions/2026-08-14-kandev-native-agent-rich-output.md)

## Implementation records

- [Copilot rich-output identity repair](../../../plans/copilot-rich-output-identity/plan.md)

- [CSV header BOM repair](../../../plans/rich-output-csv-header-bom/plan.md)
- [Structured evidence extension](../../../plans/rich-output-evidence-components/plan.md)

- [Replacement package](../../../plans/json-render-rich-output/plan.md)
- [File-preview lifetime repair](../../../plans/rich-file-preview-targets/plan.md)
