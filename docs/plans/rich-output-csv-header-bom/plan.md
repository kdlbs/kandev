---
created: 2026-10-13
status: implemented
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
  - REQ-AGENTS-AGENT-RICH-OUTPUT-003
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
legacy_specs: []
---

# Implementation Plan: Rich-output CSV header BOM repair

## Overview

Accept a valid workspace CSV whose first header is quoted and preceded by a
file-leading UTF-8 BOM, for both Chart and Table reports. One sequential work
order adds real MCP-dispatch regressions, corrects shared reader admission, and
verifies exact snapshots and retained rejection behavior.

Agents owns this repair because it owns the agent-facing report acceptance and
snapshot contract shared by task and Office transcripts. Reuse active
AC-AGENTS-AGENT-RICH-OUTPUT-001.4/.16 and 003.2/.4; their intended CSV behavior
and the design's explicit normalized-header/BOM support already cover this
regression. No new requirement, acceptance ID, schema or ADR is needed.

## Evidence and assumptions

Confirmed scope: remove exactly one BOM at byte zero before CSV tokenization.
Preserve original source admission, all existing header normalization and
strict quoting, cell semantics, numeric limits, caches and snapshots.
There are no unresolved material choices.

ROOT's qualified real-tool evidence is retained at
`/tmp/kandev-root-rich-csv-bom-discovery-20261013-c_tb6pnq/`.
`proof.log` shows two causal failures in `TestRootDiscoveryQuotedBOMCSV`:
Chart and Table reject `quoted_BOM` with a bare quote in a non-quoted field at
line 1, column 4. All four `quoted_no_BOM`/`unquoted_BOM` controls pass.
ROOT reports exact chart value `420`, literal table value `00420`, and one read
per call. `qualified-proof.json` records exit 1 after actual process join;
the original compile timeout (exit 124) is explicitly not a test verdict.
The owning package completed in 0.381s. Qualification ran at
`19c9bec19dabdb1195274fab5a6b44d6bcb7f3d8`; ROOT verified all four implicated
source/spec blobs identical at the design base
`630ac804ce28d3fe8833cbe2a29d92c490aeda1c`.

The ROOT-owned private `0400` `candidate_test.go` has SHA-256
`2fe13ffb02ddf5add02e4c63c4c8edffaece3b0be6006e3e4c9c9a869f174e8b`.
Do not change, delete or replay it or the evidence files. Permanent regressions
are authored separately after the design checkpoint.

## Scope

### In scope

- Shared CSV reader admission for `parseRichOutputCSV` and
  `parseRichOutputTableCSV`.
- Focused Chart/Table MCP report regressions in one new Go test file.
- Existing agent-rich-output requirement/design pair and this one plan/order.

### Out of scope

- `LazyQuotes`, global U+FEFF removal, whitespace/dialect changes, malformed
  quote repair, cell conversion or broader CSV import support.
- Schema/catalog/renderers, caches, replay/preservation/race redesign,
  authorization, workspace lifecycle, UI layout and new public documentation.
- New dependencies or lockfile changes, runtime rebuilds, browser suites, full backend lint,
  foreign worktrees/refs/processes/caches, and ROOT evidence cleanup.

After the later implementation/heavy grant, a routine frozen install from
`apps/` is allowed only if this managed worktree lacks dependencies needed by
normal hooks. Do not install anything during design or change dependency versions.

## Technical approach

Both parsers currently call `csv.NewReader(strings.NewReader(content))` before
`readRichOutputCSVHeader` can reach `indexRichOutputCSVHeaders`. With a BOM
outside the opening quote, the strict CSV reader fails first. Later header
normalization cannot repair this tokenization error.

Add `newRichOutputCSVReader(content string) *csv.Reader` in
`apps/backend/internal/mcp/server/rich_output_csv.go`: remove only one leading
`\ufeff` using `strings.TrimPrefix`, then create the default strict reader.
Use it in both parsers. Keep `readRichOutputCSV` unchanged so original reported
size, original content bytes, binary status and UTF-8 validity are checked before
normalization. Keep `indexRichOutputCSVHeaders` and its existing header
normalization unchanged. Do not mutate cached `FileContentResponse.Content`.

| Source shape | Chart | Table | Evidence |
| --- | --- | --- | --- |
| File-leading BOM then quoted first header | Accept finite values/gaps | Accept literal selected cells | New real-tool RED/GREEN |
| Quoted header without BOM; unquoted BOM header | Existing support | Existing support | Paired controls |
| Same `(repo, path)` in Chart and Table | Numeric snapshot | Text snapshot | One authorized read, exact element IDs and both result forms |
| Header-only BOM/quoted source | Reject zero chart rows | Empty rows array | Real-tool control |
| Malformed quotes, invalid text/numbers or exceeded bounds | Fail tool | Fail where applicable | Strict controls; no partial structured result |

## Tests

New file: `apps/backend/internal/mcp/server/rich_output_csv_bom_test.go`.
Use existing `callTool`, `testBackend`, `tableReadBackend`, `tableArgs`,
`compositionArgs` and `assertJSONEquivalent` patterns. Mock only the authorized
workspace transport; invoke `show_rich_output_kandev` through real registered MCP
dispatch and schema/semantic validation, rather than calling parsers directly.

- `TestRichOutputCSVHeaderBOMReports`: independent Chart and Table cases in
  task and Office modes. BOM plus quoted first header must fail RED with the
  known header parse error, then pass GREEN. Quoted/no-BOM and unquoted/BOM
  controls pass in both phases. Verify complete expected snapshots, not merely
  success. Covers AC-001.4/.16 and AC-003.2/.4.
- `TestRichOutputCSVHeaderBOMSharedSourceSnapshot`: compose Chart and Table
  over one BOM/quoted file, for both task and Office. Check session/path/repo
  request, one backend read, input versions, exact element IDs and selected
  column order; chart `420` and null gap versus table `00420` and empty text.
  Preserve raw selected whitespace, escaped quotes, markup/formula text and
  embedded/data-cell U+FEFF; exclude unselected sentinel cells from snapshots.
  Assert both structured content and parsed JSON text fallback equal the exact
  complete result. Covers AC-001.4/.16, AC-002.5/.6 and AC-003.2/.3/.10.
- `TestRichOutputCSVHeaderBOMStrictControls`: header-only source is a successful
  empty Table but a rejected Chart; malformed first-header and row quotes fail
  for both, without partial snapshots. A BOM after whitespace or two BOMs before
  an opening quote must not be silently repaired. Duplicate/blank headers and
  width errors still fail. With a valid BOM/quoted header, NaN, positive/negative
  infinity, overflow and a BOM inside a numeric cell still fail chart conversion;
  empty numeric cells remain null. Original binary/invalid-UTF-8 and declared or
  actual byte limits are retained, including a file whose original length is
  above 256 KiB but would fit after removing three BOM bytes. Existing row,
  label, cell and snapshot bounds run with the focused rich-output suite.
  Covers AC-001.16 and AC-003.4/.10.

Retain existing `TestRichOutputToolRejectsInvalidCSVData`,
`TestRichOutputTableCSVBounds`, `TestRichOutputTableReadAndSnapshotLimits`,
`TestRichOutputTableSharesChartSourceRead`, and shared-composition tests. Add
only missing focused controls; do not rewrite unrelated tests or append to a
large test file.

## E2E evidence and report outcomes

Real MCP dispatch, authorized source-read requests and complete result emission
are the end-to-end boundary for this parsing repair. No frontend code changes
or new browser scenario is needed. Existing desktop and phone flows in
`apps/web/e2e/tests/chat/rich-output-evidence.spec.ts` and
`mobile-rich-output-evidence.spec.ts` remain prior rendering/replay evidence,
not new runs for this package.

A previously failed valid CSV report will complete through the existing native
report renderer. Desktop Tables retain semantic columns/cells; phones retain
ordered labelled row fields. Charts remain inline and bounded in both views;
the transcript remains the vertical scroll owner. Header-only Tables retain
the localized empty state. Malformed CSV retains the ordinary tool error.
There is no new layout or control, so no UI preview is required.

## Work orders

- [x] [Task 01: Admit file-leading BOM before strict CSV parsing](task-01-csv-reader-admission.md)

## Verification results

Design checks at `630ac804ce28d3fe8833cbe2a29d92c490aeda1c`:

- `python3 scripts/list-docs.py validate`: PASS (381 decisions, 1589 specifications).
- `python3 scripts/lint-spec-files.py --all`: PASS.
- `git diff --check` and plan-path status: PASS; one untracked plan directory
  contains the manifest and exactly one pending order, both unstaged.
- Task 01's planned-production-scope `validateCoverage` preflight: PASS,
  `requiresCoverage: true`, `status: covered`, one work order, all three owning
  requirements referenced, `errors: []`. Used the existing Node 24.18.0 binary
  at `/home/jcfs/.nvm/versions/node/v24.18.0/bin/node`; the initial bare `node`
  attempt could not start because PATH omitted it. No install was performed.
- Public-doc impact assessed with `/docs-maintainer`: existing native-rich-
  output reference already describes normalized headers, literal Table cells,
  bounds and snapshots. This design restores that contract with no new public
  API, syntax, copy or layout; no public-doc edit is needed.

The design checkpoint completed before ROOT's later separate implementation
interrupt and sole heavy-lease grant. The following checks then ran serially
with the existing Go 1.26.0 toolchain, `GOTOOLCHAIN=local`, warmed
race/trimpath/fts5 cache, one build worker, two runtime processors, 512 MiB memory
target and GNU 620-second/10-second kill bounds:

- Real MCP RED (`^TestRichOutputCSVHeaderBOM`): expected exit 1 after actual join.
  All four independent task/Office Chart/Table BOM/quoted cases failed at the
  qualified line-1/column-4 quote boundary; all eight paired controls passed.
  Other BOM-dependent snapshot/header/row/number checks reached the same header
  error. Package assertion time was 1.824s. The generic JSON fixture replaced
  invalid UTF-8, so that control's transport now injects raw content into the
  real file-response seam rather than accepting JSON's replacement character.
- Focused GREEN (`^TestRichOutput`): exit 0, 25 top-level tests and all 47 new
  BOM subcases passed (12 independent report cases, 2 mixed snapshots, 33 strict
  controls), 8.068s. Both result forms match exact accepted data.
- Complete `./internal/mcp/server` package with race detector: exit 0, 32.304s.
- Canonical `bash scripts/lint-go-changed` over the three owned Go paths:
  exit 0, zero issues; complete affected package, no full backend scan.
- Actual changed-file `validateCoverage` preflight: exit 0, covered, one order,
  three owning requirements, `errors: []`.
- Catalog validation and full specification lint: exit 0.

Task 01 records exact commands, joins and local evidence paths. Normal hooks,
publication and exact-head CI/review/merge evidence are tracked externally;
local implementation completion does not claim hosted validation or merge.

## Risks and execution boundary

- Removing BOMs globally, after tokenization, or from cached bytes would violate
  the bounded repair. Use only the shared constructor and exact snapshots.
- A permissive reader could mask malformed quoting; retain default strict CSV
  options and pair positive cases with rejection controls.
- Shared code may move on main. Reconcile relevant files and preserve others'
  edits; do not rebase for drift alone.
- Design stopped before production/tests/heavy work. ROOT later explicitly
  granted same-primary implementation and the heavy lease. Normal delivery
  follows completed local checks; a separate ROOT serial grant remains required
  for a normal merge of the exact ready head.
