---
created: 2026-10-10
status: implemented
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
  - REQ-AGENTS-AGENT-RICH-OUTPUT-003
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
legacy_specs: []
---
# Implementation Plan: Rich-output evidence components

## Overview

Extend the existing json-render catalog with Table, StatusList and Callout.
Deliver Table first because it owns CSV resolution and snapshot changes; then
StatusList; then Callout with final discovery, documentation and real-agent
proof. Each work order is an end-to-end slice that can pass before the next.
Execute sequentially in the primary session with TDD after the design-package
handoff and a later implementation request. No delegation is authorized.

Agents owns the durable rich-output authoring and replay contract shared by
Tasks and Office. This extends the already implemented
[replacement package](../json-render-rich-output/plan.md). Its three completed
work orders and historical results remain accurate records; do not reopen or
copy them. Additive component details belong to requirement 003 and the owning
design, which are now active/current after this package passed.

Confirmed: the user chose all three recommended additions, preserving CSV
support and agent discoverability. Progress remains deferred. Routine bounded
schema and phone-presentation choices are documented in the design; there is
no material unresolved question or new ADR. At planning, source had six catalog
components, a real Renderer/registry, chart-only CSV snapshots and four-content
limits. The earlier natural-prompt 6 Luna run showed agent-led discovery for
that baseline; extension evidence is recorded in Task 03.

## Scope

### In scope

- Three bounded literal leaves in the existing version 2/catalog 1 contract.
- Inline and CSV Tables, selected-cell snapshots, shared file-read cache,
  immutable exact-identity normalization and replay without source access.
- Snapshot StatusList findings and static Callout notes, with accessible labels,
  localized host copy and no new task authority or agent actions.
- Desktop tables and phone labelled-row composition, empty states, wrapping,
  state retention and task/Office consumer compatibility.
- Generated schema/examples, guidance, targeted TDD/browser checks, public
  reference and one isolated real-Codex natural-prompt trial.

### Out of scope

Progress, additional chart types, filtering/sorting/pagination/export, rich cell
renderers, forms, action callbacks, links, live data, plugin components, catalog
version renumbering, library upgrades, new storage, new transport and rollout
flags. Do not modify the user's main :9998 instance. No commit, push or PR is
part of this package.

## Technical approach

### Table vertical slice

Extend `apps/web/lib/rich-output/catalog.ts`, `schema.ts`, `examples.ts` and
`validate.ts`; regenerate the existing embedded catalog schema. Add semantic
unique-column/row-width and shared content-count checks in Go and TypeScript.
Add `rich_output_table_csv.go` beside the existing chart parser, reusing bounded
UTF-8 reads/header validation and the per-call file cache. Preserve chart-only
snapshot output and add optional `resolved_tables` entries by exact element ID.
Keep all source resolution atomic and bound the combined result to 64 KiB.

Extend `normalize.ts` with a small table-snapshot binding helper, including
structured-result and JSON-text fallback consumers. Register `Table` through
`defineRegistry`; its native implementation under the existing rich-output
component directory uses the current UI Table on desktop and labelled row cards
on phone. It never fetches CSV or bypasses the library Renderer.

### StatusList and Callout vertical slices

Each slice extends catalog/schema/corpus, backend shape/semantic validation,
registry, native component tests and targeted browser cases. StatusList joins
the four-content-node bound and uses ordered icon + translated label + agent
text. Callout is explanatory text under node/depth/byte bounds, adapts native
Alert with `role="note"`, and does not make history an assertive live alert.
Their schemas and meanings are in the owning design, not separate UI protocols.

Existing Table/Badge/Alert primitives reduce dependencies and keep Kandev's
appearance. Do not install or expose the stock json-render/shadcn catalog.
All new host strings use `t()` and seven required locale catalogs; generate
Traditional Chinese via the existing tool. Agent content remains plain data.

### Discovery, compatibility and persistence

Extend generated examples as each component lands. Final task/Office guidance
and public reference explain selection without requiring component names in
user prompts. Keep tool-description/injected-line budgets and existing profile
visibility. The trusted identity/result-selection boundary remains unchanged.

| Provider/transport evidence | Intended behavior | Evidence and fallback |
| --- | --- | --- |
| Canonical Kandev result with structured content or equivalent JSON text | Render additive catalog through the existing registry | Full-consumer fixtures and real MCP E2E |
| Marked Codex ACP MCP envelope | Preserve arguments, element IDs and accepted snapshots | Existing adapter fixtures plus isolated 6 Luna smoke after final build |
| Complete Cursor/Grok envelopes, accepted Claude/historic titles | Use existing matching and extraction | Full-consumer fixtures; not a claim of live provider runs |
| Foreign provider or incomplete envelope | Existing generic activity fallback | Negative controls; no newly inferred identity |

Historical v1 and catalog-1 chart-only fixtures retain their input and meaning.
No database migration, message rewrite, legacy dispatcher or lazy replay source
read is allowed. Old implemented plans remain linked for their regression
commands; this plan's final work order runs the relevant ones together with new
suites. Promote the owning requirement/design to active/current and this plan
to implemented only after all linked work orders pass.

## ASCII UI preview

Titles, status values and dataset entries below are illustrative agent data.
Host states/kind/status labels are localized. Geometry is illustrative; field
association, order, plain data, scroll ownership and phone containment are
required. Entry point: a completed report in Task or Office chat.

### UI-01: Evidence report, desktop, completed

```text
+----------------------------------------------------+
| Service review                                     |
| Services                                           |
| Service       Monthly cost (USD)       p95 (ms)     |
| API           420                      165         |
| Payments      680                      240         |
| Verification                                       |
| [check] Passed  Unit tests        38 checks         |
| [cross] Failed  Load budget       p95 over target   |
| [clock] Pending  Dependency audit Not checked       |
| +------------------------------------------------+ |
| | Warning: Investigate Payments                  | |
| | Highest cost and slowest service in this sample | |
| +------------------------------------------------+ |
+----------------------------------------------------+
```

Semantic table headers and cells; wrap long content. Optional local horizontal
scroll is confined to the desktop Table. Status/kind have text and icons.
No completion button, polling indicator or generated action is introduced.
Covers AC-003.1/.2/.5/.6/.7/.8/.9.

### UI-02: Same evidence report, phone, completed

```text
+----------------------------------+
| Service review                   |
| Services                         |
| +------------------------------+ |
| | Service: API                 | |
| | Monthly cost (USD): 420       | |
| | p95 (ms): 165                 | |
| +------------------------------+ |
| +------------------------------+ |
| | Service: Payments            | |
| | Monthly cost (USD): 680       | |
| | p95 (ms): 240                 | |
| +------------------------------+ |
| Verification                     |
| [check] Passed                   |
| Unit tests                       |
| 38 checks                        |
| [cross] Failed                   |
| Load budget                      |
| p95 over target                  |
| [clock] Pending                  |
| Dependency audit                 |
| Not checked                      |
| Warning: Investigate Payments    |
| Highest cost and slowest service |
| in this sample                   |
+----------------------------------+
```

Below 768px, Table changes to ordered labelled fields; no column is hidden.
Use shared columns/rows and the existing responsive breakpoint. Preserve the
same report and neighboring stateful component identities. The inline rich
output Metrics and shipped simple-task property rows are field/wrapping
exemplars; use their presentation patterns, not their business controls.
Transcript owns vertical scrolling. No new fixed elements or safe-area layer.
Existing file/legend controls retain phone/coarse 44px minimum hit areas and
ordinary fine-pointer desktop density. Covers AC-003.5/.6/.8/.9 and AC-002.8.

### UI-03: Empty, literal data and failure states

```text
Empty Table:     Services -> [No rows]
Blank cell:      Service: API -> Optional note: [Empty]
Empty StatusList: Verification -> [No checks]
Literal content: <script>... -> visible text, no executable markup
Invalid call:    [existing tool error; no partial report]
Bad snapshot:    [localized report unavailable; no file read]
```

Empty data is valid; malformed data is not repaired. Copy here is illustrative
and must use existing/new locale keys. Covers AC-003.2/.4/.5/.6/.8/.10.

## Tests

Evidence below uses the real catalog, normalizer, registry and Renderer;
only external workspace transport is mocked in consumer tests. Browser cases
cover native markup rather than adding tests that duplicate component structure.

| Criteria | Planned evidence |
| --- | --- |
| AC-003.1/.2/.4/.10, AC-002.2/.3 | `catalog.test.ts`, `conformance.test.ts`, Go shared corpus: columns/row limits and width, unique keys, Unicode/byte bounds, exclusive sources, unknown values and four-content counting |
| AC-003.3/.4/.10, AC-002.5/.6 | `rich_output_table_csv_test.go`: selected headers/cells, empty file/header cases, shared chart/table read cache, atomic failures and combined byte bounds; `normalize.test.ts`: missing/extra/duplicate/mismatched/wrong-type snapshots, immutable inputs and old chart-only output |
| AC-003.5/.9 | `rich-output-view-state.test.tsx`, real Renderer/full-consumer tests and named Table browser cases: semantic desktop headers, phone labelled rows, empty/blank values, plain text and neighboring chart/file state |
| AC-003.6/.7/.9/.10 | Named StatusList browser cases and conformance: every enum, visible translated status/icon, order, long detail, empty list and no polling/callbacks |
| AC-003.8/.9/.10 | Named Callout browser cases and full-consumer tests: all kinds, localized visible label/icon, literal content, note role and no live announcement |
| AC-003.1/.3/.9 and AC-002.4/.9 | Existing historical, trusted-tool, text-result extraction and file-target consumer tests plus new table fixtures |
| AC-003.11 and AC-002.10 | Generated example validation, Go discovery/guidance budget tests, task/Office sync; isolated real-model trial recorded separately from deterministic fixtures |

## E2E tests

Create `tests/chat/rich-output-evidence.spec.ts` for Chromium and
`tests/chat/mobile-rich-output-evidence.spec.ts` for mobile-chrome; reuse
`rich-output-helpers.ts` and normal MCP delivery rather than canned transcript
insertion. Each work order adds a named Table, StatusList or Callout scenario.

- Table: CSV with eight columns, blanks and long text; compare inline/CSV
  output; delete source, reload and retain every value; header-only empty state.
  Phone asserts labelled fields, preserved order, and zero document horizontal
  overflow at the canonical viewport and just below/above 768px.
- StatusList: all five labelled statuses, wrapped explanations and empty list;
  no state mutation, callbacks or request indicating live polling.
- Callout: all four kinds and note semantics; long text contained on phone;
  combined Table/StatusList/Callout report and empty states.
- Final mixed report: existing chart filter and expanded file survive unrelated
  transcript update and desktop-phone-desktop; invalid snapshots remain local
  unavailable presentations; historical tests remain green. Covers AC-003.1
  through .10 with existing AC-001/.002 regression controls.

Guarded E2E builds the current artifacts. Run projects sequentially and use
`--no-build` only after the fresh build and without intervening code changes.
No full-suite overlap or worker override. Real-model discovery is a separate
trial, not a flaky CI assertion that every prompt selects every component.

## Work orders

- [x] [Task 01: Tables and CSV snapshots](task-01-tables.md)
- [x] [Task 02: Snapshot status lists](task-02-status-lists.md)
- [x] [Task 03: Callouts and discovery integration](task-03-callouts-and-discovery.md)

## Verification results

Implementation was authorized and completed on 2026-10-10. All three work
orders are done. Current regression passed 17 frontend files/229 tests, producer
and system-prompt packages, typecheck, full web lint, focused backend lint,
schema drift and localization gates. Desktop passed the six Table/StatusList/
historical cases plus the corrected mixed-report case; phone passed all five
addition/historical cases. Real Codex 6 Sol authored and rendered all three
components from the unchanged natural prompt. Task 03 records failed attempts,
discovery corrections and the final isolated example without claiming universal
model routing. CSV charts and version 1 replay remain covered.

Design-package checks passed on 2026-10-10:

- `python3 scripts/list-docs.py validate`: 361 decisions and 1433 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Documentation coverage preflight over the actual tracked diff and all new
  files: covered, no errors, six accepted work-order reference sets (the three
  completed replacement records and these three extension records).
- `git diff --check -- docs/specs/agents docs/plans/rich-output-evidence-components`: passed.
- `git status --short -- docs/specs/agents docs/plans/rich-output-evidence-components`:
  two modified owning specs and the new untracked plan directory; unstaged and
  uncommitted. No production or permanent test files changed in this design turn.

The replacement package's 155-test receipt remains historical evidence.
Current implementation commands and results belong to these work orders.

### Documentation coverage preflight

Run from repository root after implementation and inspect errors/reference sets:

```bash
node - <<'JS'
const fs = require('fs');
const cp = require('child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = args => cp.execFileSync('git', args).toString().split('\0').filter(Boolean);
const tracked = paths(['diff', '--name-only', '-z']);
const added = paths(['ls-files', '--others', '--exclude-standard', '-z']);
const changedFiles = [
  ...tracked.map(filename => ({ filename, status: fs.existsSync(filename) ? 'modified' : 'removed' })),
  ...added.map(filename => ({ filename, status: 'added' })),
];
const fileContents = {};
for (const p of [...tracked, ...added]) {
  if (p.startsWith('docs/') && p.endsWith('.md') && fs.existsSync(p)) {
    fileContents[p] = fs.readFileSync(p, 'utf8');
  }
}
for (const p of [
  'docs/plans/json-render-rich-output/plan.md',
  'docs/plans/rich-output-evidence-components/plan.md',
  'docs/specs/agents/requirements/agent-rich-output.md',
  'docs/specs/agents/system-design/agent-rich-output.md',
]) fileContents[p] = fs.readFileSync(p, 'utf8');
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

## Risks

- Treating CSV cells like chart numbers would destroy formatting or reject text;
  shared parsing must retain separate numeric and literal-cell policies.
- A permissive result union could bind table cells to the wrong element or
  accept incomplete mixed reports; exact expected source sets are mandatory.
- Existing UI Table has nowrap/overflow defaults; native phone composition and
  explicit desktop wrapping must be verified in the browser.
- Agent choice depends on model and task; generated descriptions/examples and
  the natural-prompt trial are evidence without a universal routing guarantee.

## Agent-usability diagnosis (2026-10-10)

The user correctly challenged the completion summary: successful rendering and
one final first-call success do not establish reliable agent authoring.
Re-read all eight isolated example histories through the ordinary API; none
was paginated or truncated. Two natural-prompt Luna sessions did not invoke
rich output. The first Sol evidence session rejected 12 calls before accepting
one composed report. The next rejected seven calls and accepted four separate
reports. The final revised-metadata Sol session accepted one composed report
on its first call. Earlier baseline reports had one successful call each.

Confirmed product defect: `sanitizedToolArgumentError` walks every nested
`oneOf`/`anyOf` failure, unions missing/unknown properties across mutually
exclusive branches, and omits the useful property-type failure. A Table whose
columns are strings is reported as having unknown `columns`, `rows`, and
`title`, while demanding unrelated Chart/Stack/FilePreview fields. The actual
inline Table error is `/props/columns/0`: expected an object containing key and
label. Table's inline/CSV source union also needs relevant-branch feedback.
A private Go overlay reproduced this against the real registered validator;
no production or permanent test files were changed during diagnosis.

The retained final MCP attachment advertises all nine variants and seven
examples (13,065 bytes compact JSON). That confirms backend advertisement;
it does not prove which schema details the Codex client surfaced or the model
used. Do not attribute all failures to provider behavior or claim a missing
schema without protocol evidence.

Open remediation: correct schema-derived union/type diagnostics, make catalog
shape/examples easy to discover without expanding a manually maintained
650-character recipe, and repeat ordinary-prompt/recovery trials across the
supported models. Preserve strict validation and the existing renderer. The
completed deterministic checks remain valid; agent usability remains an open
quality issue. Full authorized example histories are retained privately at
the privately retained usage-diagnosis receipt. The diagnostic overlay was
removed after recording its output; the user's demo remains running.

The follow-up [authoring-recovery package](../rich-output-authoring-recovery/plan.md)
owns this correction and repeated usability evidence. Its two work orders are
pending at the 2026-10-10 design checkpoint; this component package's original
implementation and verification receipts remain unchanged.


Authoring-recovery delivery (2026-10-12): the linked package is implemented
locally. Truthful union/type diagnostics and generated discovery passed their
checks; all four new natural Sol/Luna trials succeeded on the first call, and
both deliberately malformed-call trials recovered on the next call. Desktop
and phone checks passed for all four natural reports. This resolves the
recorded package usability gate, with the prior failed attempts retained as
historical evidence and no universal model-reliability claim.
