---
created: 2026-10-08
status: implemented
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
legacy_specs: []
---
# Implementation Plan: json-render rich output

## Overview

Replace Kandev's fixed rich-output composition engine with json-render and let
agents author approved native compositions. First establish the real library
renderer and version 1 replay compatibility, then deliver the version 2 tool
contract and browser flows, then remove obsolete composition code and publish
the final contract. Execute sequentially in the primary session with TDD after
an explicit implementation request. No delegation is authorized.

The user selected catalog composition on 2026-10-08. The user authorized implementation in the primary
session. All three work orders are done; requirements are active and the design
is current. This package records local delivery and its verification evidence.

## Scope

### In scope

- One native json-render rendering path for version 1 compatibility and version
  2 compositions in task and Office transcripts.
- Stack, Group, Text, Chart, Metrics, and FilePreview with closed literal props.
- Catalog-derived backend schema, bounded graph validation, element-addressed
  CSV snapshots, and exact agent guidance/examples.
- Existing native chart/motion/file behavior, desktop/phone composition,
  trusted tool identity, historical replay, and localized failures.
- Public MCP reference and scoped engineering guidance at implementation time.

### Out of scope

MCP Apps, A2UI, new agent transport, streaming patches, forms, agent-authored
callbacks, native tables, plugin catalogs, new artifact storage, data
subscriptions, broad performance rewrites, and release flags.

## Technical approach

### Catalog and renderer

Pin the initial core/react packages at 0.21.0 and a compatible direct Zod 4
version. Read the published declarations before coding; do not substitute an
old example API. Add the pure catalog and normalization modules under
`apps/web/lib/rich-output/`. Keep concrete native registry components under
`components/task/chat/messages/kandev/rich-output/`.

Use `defineCatalog`, `catalog.validate`, `defineRegistry`, and the real
`Renderer`. The outer `RichOutputRenderer` keeps terminal-state handling,
metadata, translated failures, and trusted session/file context. Reuse existing
native `Chart`, `Metrics`, and `FilePreview`, visibility/motion hooks, and
workspace-preview hook as native content implementations. Retain their local
state and memoized domain inputs. Remove `RichOutputBlockView`; never retain a
legacy JSX dispatcher behind the new engine.

### Agent contract and validation

Keep `show_rich_output_kandev`, its read-only annotation, and profile placement.
Prefer `version: 2`, `catalog_version: 1`, and the native json-render
`spec.root`/`spec.elements` model. Keep version 1 only at the historical decoding boundary. New calls use version
2 after the renderer-first implementation transition; stale callers get clear
discovery-refresh guidance without reading CSV files.

The generator under `apps/web/scripts/generate-rich-output-catalog.ts` emits
`apps/backend/internal/mcp/server/rich_output_catalog_v1.schema.json` from the
catalog's component property schemas. Close the Kandev-owned spec fields and
exclude dynamic/interaction fields. Add a deterministic `--check` mode and wire
it into build/CI through the owning web package scripts. The MCP root remains a
portable object without root combinators. The handler enforces branch-exclusive
fields and semantic limits before any workspace read.

Both Go and Vitest consume
`apps/backend/internal/mcp/server/testdata/rich_output_composition_cases.json`.
Do not add a hand-maintained second component/property catalog in Go. Shape
validation uses the existing schema validator; only host graph/path/size/data
semantics need explicit checks. Export and frontend validation must agree on
Unicode code-point boundaries. Reject unknown keys rather than silently
stripping them.

### CSV, replay, and old inputs

Preserve accepted input/result metadata and the current file authorization.
New CSV results carry `element_id`; only historical decoding understands `block_index`.
Validate an exact, version-matched snapshot set and label mapping before
normalization. Do not re-read CSV on replay or mutate stored input.

Keep the version 1 decoder as compatibility only, then normalize both versions
to one resolved library spec. Freeze deterministic legacy element IDs and retain
old order and content. File instance identity is the element
plus the existing session/repo/path tuple; unrelated data or viewport changes
must not reset it.

### Provider compatibility

The shared result-selection/identity design remains authoritative. No provider
normalization changes are planned. The matrix in the owning system design
covers canonical, marked Codex, complete Cursor/Grok, compatible historic titles,
and foreign/incomplete fallback shapes. Extend full-consumer input/result
fixtures for version 2 and retain all version 1 controls. Those fixtures do not
constitute live provider smoke evidence.

### Guidance and documentation

Task guidance lives in `apps/backend/internal/sysprompt/sysprompt.go` through
`richOutputSection`; the template uses `{rich_output_section}`. Office guidance
lives in `apps/backend/config/prompts/office-context.md`. Replace version 1
instructions with concise composition guidance and exact version 2 recipes in
tool schema examples. Retain the existing 650-rune tool description and 750-rune
injected-line budgets. Do not inject the library's default JSONL-patch prompt
into a tool-call workflow.

Publish the updated Native rich output reference in
`docs/public/automation-and-mcp.md` with working composition examples and version
1 compatibility. Reuse current host strings when possible; new host copy must
be complete in every required locale. Update the rich-output engineering
boundary in `apps/web/AGENTS.md`, preserving the root guide's existing guidance.
Apply `/docs-maintainer` to the actual implementation diff.

## ASCII UI preview

Illustrative titles and values are agent data; host controls are translated.
These sketches specify hierarchy, order, responsive grouping, and actions, not
pixels or new chart styles. AC-002.1/.7/.8 and AC-001.5/.6/.14/.18 apply.

### UI-01: Composed report, desktop, completed

Entry: a completed `show_rich_output_kandev` call in task or Office chat.

```text
+------------------------------------------------------+
| Build health                                         |
| Current workspace verification                       |
| +---------------------+ +--------------------------+ |
| | Checks              | | Interpretation           | |
| | Passed 38  Failed 0 | | All focused checks passed | |
| +---------------------+ +--------------------------+ |
| Latency over time                                    |
| [native chart with axes, summary, and legend]         |
| Evidence                                             |
| reports/build.json               [Preview] [Open]    |
+------------------------------------------------------+
```

The two top groups are ordered children of a horizontal Stack. Chart and file
are later siblings in a vertical Stack. The transcript owns primary scrolling.
File preview remains collapsed until explicitly opened; opening the file uses
the existing viewer. Ordinary desktop control density is retained.

### UI-02: Same report, phone, completed

```text
+-------------------------------+
| Build health                  |
| Current workspace verification|
| Checks                        |
| Passed 38                     |
| Failed 0                      |
| Interpretation                |
| All focused checks passed     |
| Latency over time             |
| [bounded native chart]        |
| [wrapping legend controls]    |
| Evidence                      |
| reports/build.json            |
| [Preview]       [Open]        |
+-------------------------------+
```

The horizontal group becomes vertical below 768px without mounting a second
component tree or resetting local state. DOM order is unchanged. Controls use
44px phone/coarse-pointer targets, content stays inside the viewport, and the
Open action reaches `MobileFileViewerPanel`. No new drawer or fixed toolbar is
introduced.

### UI-03: Pending, failed, and unavailable

```text
Pending:     [compact native tool activity]
Tool failed: [existing tool error]
Bad history: [localized presentation unavailable]
File failed: [report intact] [file preview unavailable; explicit retry]
```

The first three states never mount a chart or fetch a file. File failure remains
local. These structures cover AC-001.13/.14 and AC-002.2/.4/.5/.6.

## Tests

Planned names below are implementation targets; existing suites must remain
meaningful and pass. Test the real library boundary rather than mock Renderer.

| Criteria | Evidence |
| --- | --- |
| AC-002.1/.2/.3 | `lib/rich-output/catalog.test.ts`: approved literal props and generated schema; `normalize.test.ts`: invalid graph and limits; `rich-output-renderer.test.tsx`: actual registry, grouping, and responsive state |
| AC-002.4/.5/.6 | `normalize.test.ts`: legacy conversion and exact CSV snapshot binding; full `rich-output-renderer.test.tsx`: v1/v2 replay and invalid stored inputs/results |
| AC-001.1/.2/.3/.4/.15/.16 and AC-002.2/.3/.5/.9/.10 | `rich_output_test.go` and new `rich_output_composition_test.go`: profiles, generated-schema compilation, shared corpus, source read guards, snapshots, compatibility; `sysprompt_sync_test.go` and `tool_metadata_test.go`: recipes/budgets |
| AC-001.5/.6/.7/.8/.19 and AC-002.7 | Existing `chart-format.test.ts`, `chart-block.test.tsx`, `chart-motion.test.tsx`; full-consumer renderer controls keep animations, data, and filtering stable |
| AC-001.9/.10/.11/.12/.14 and AC-002.7/.8/.11 | Real-catalog `rich-output-renderer-file-targets.test.tsx`: replacement races, stable targets, independent same-file cards, viewport change, and no initial read |
| AC-001.13/.17/.20 and AC-002.9/.10 | `kandev-tool-message-rich-output.test.tsx`, `components/task/chat/messages/kandev/types.test.ts`, and `hooks/use-processed-messages-rich-output.test.ts`: trusted identity, result extraction, standalone rendering, prompt routing |
| Generated source of truth | `scripts/generate-rich-output-catalog.test.ts`: deterministic output and check-mode drift rejection; shared Go/TS corpus validates examples and Unicode boundaries |

## E2E tests

- `tests/chat/rich-output.spec.ts`, Chromium: retain v1 legacy controls and add
  v2 composed inline/CSV reports. Check group order, axes/legends, local filtering,
  deferred chart/motion behavior, lazy preview, native file opening, and reload
  after source deletion. Covers AC-001.1/.4/.5/.6/.7/.8/.13/.14/.17/.19 and
  AC-002.1/.4/.5/.6/.7/.9/.11.
- `tests/chat/mobile-rich-output.spec.ts`, mobile-chrome: the same v2 payload
  follows UI-02, stays contained, wraps controls, and opens the native viewer.
  Retain v1 controls. Full-consumer tests additionally cover stable preview state across the 767/768px
  fine-pointer boundary. Browser flows verify nested grouping and phone controls, including a narrow
  fine-pointer Chromium case with measured 44px file controls. Covers AC-001.6/.14/.18 and
  AC-002.1/.7/.8/.11.
- Shared full-consumer renderer/profile tests prove Office uses the same
  contract. The browser scenarios exercise Task delivery; do not claim a live
  Office/browser/provider run from shared coverage.
- Unavailable historical inputs and identity negatives use full-consumer fixtures;
  invalid new tool calls use real MCP error assertions. Do not weaken backend
  validation merely to manufacture corrupt history for a browser test.

Use the guarded E2E runner without overlapping suites or overriding worker
budgets. Its first invocation builds current artifacts; only the immediately
following mobile invocation may use `--no-build`. Every new wait follows the
causal-wait helpers and E2E sleep lint.

## Work orders

1. [Task 01: Library renderer and legacy replay](task-01-library-renderer.md)
2. [Task 02: Agent composition and CSV contract](task-02-agent-composition.md)
3. [Task 03: Retire obsolete composition and publish](task-03-retire-and-document.md)

Each task depends on the previous one. W1 can pass against the existing tool;
W2 is the new end-to-end authoring slice; W3 closes removal and documentation.

## Verification results

Implementation receipts (2026-10-08):

- All three work orders are done. Their Results sections record RED/GREEN
  evidence, final commands, and the source/runtime boundaries.
- Frontend: 14 files / 155 tests passed; typecheck, full lint plus final changed-
  file lint, i18n checks/ratchet, generator drift, and frozen install passed.
- Backend: MCP server and sysprompt suites passed; scoped CI-style golangci-lint
  reported zero issues. The shared corpus contains 39 acceptance/rejection cases.
- Browser: four Chromium and two mobile-chrome tests passed against current
  production artifacts, including CSV deletion/reload, v1 history, native file
  opening, chart controls/motion, phone containment, and narrow fine-pointer
  layout. Phone controls remain at least 44px under pressed scaling.
- Docs: specification catalog/lint, 36 validator tests, 62 public-doc validator
  tests, and 47 public pages passed. Public examples validate against the
  catalog. Actual-diff documentation coverage is covered with all three work
  orders and zero errors. All 31 criteria remain linked.
- Desktop/phone captures were inspected against UI-01/UI-02.
- No commit, push, PR, delegation, or deployment was performed.

Design-package receipts (2026-10-08):

- `python3 scripts/list-docs.py validate`: passed; 361 decisions and 1433
  specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed; 36 validator tests.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: the actual
  eight-file documentation diff is exempt; a separately labelled projected
  renderer change passes all three work orders' contract references. This is
  design traceability evidence, not an implemented-runtime coverage claim.
- Local reference audit: all 31 declared acceptance criteria appear in work-order
  frontmatter, no nonexistent criteria are referenced, and local Markdown
  document links resolve.
- `git diff --check`: passed.

## Risks

- Published json-render schemas support more features than this catalog permits;
  the closed literal guard is necessary even when library validation succeeds.
- A root union would fail Kandev's provider-portable schema contract. A strict
  library export would close the dynamic element map. Test both hazards.
- Recreated registry/adapted-prop identities could reset previews or animate
  settled charts. Full-consumer state tests are required.
- Traversal-index CSV results can bind to the wrong node after composition;
  element-addressed version 2 snapshots and exact-set validation prevent that.
- Old Office examples and cached tool descriptions can produce version 1 input;
  reject stale authoring with actionable guidance while retaining historical replay.
- This package intentionally preserves native chart/file implementations. The
  removed custom code is the composition dispatcher and duplicated v2 catalog,
  not host authorization, content primitives, or compatibility decoding.

## Approved implementation refinements

The user authorized implementation on 2026-10-08 including shedding unnecessary
old functionality. Use catalog-derived native props, a historical-only v1
decoder, no legacy styling mode, no unused MIME hint, and one CSV data resolver.
At completed delivery the backend accepts only v2 and returns actionable errors
for stale v1 authoring. Record catalog evolution rules and retain CSV snapshots.
Future Tables/Status/Notice and action capabilities remain outside this package.
