---
id: "01-csv-reader-admission"
title: "Admit file-leading BOM before strict CSV parsing"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
  - REQ-AGENTS-AGENT-RICH-OUTPUT-003
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.4
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.16
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.5
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.6
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.2
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.3
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.4
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.10
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
---

# Task 01: Admit file-leading BOM before strict CSV parsing

## Summary

Remove exactly one file-leading UTF-8 BOM before strict CSV tokenization for
Chart and Table. Prove the correction through real registered MCP dispatch,
including exact combined snapshots, shared-source reads and rejection controls.

## In scope

- Add the three `TestRichOutputCSVHeaderBOM*` tests defined in the plan in
  `apps/backend/internal/mcp/server/rich_output_csv_bom_test.go`.
- Add `newRichOutputCSVReader` in `rich_output_csv.go` and use it in
  `parseRichOutputCSV` and `parseRichOutputTableCSV`.
- Record actual RED/GREEN, package/race, affected-package lint and document
  results in this work order and the sibling plan.

## Out of scope

Keep `readRichOutputCSV`, header indexing, cell/numeric conversion, row limits,
source cache and snapshot/result plumbing unchanged. No `LazyQuotes`, global
U+FEFF stripping, whitespace/dialect changes, malformed quote repair, schemas,
frontend/renderers, new dependencies/lockfile changes, preservation/race work or
ROOT evidence cleanup.
Do not touch other agents' work, the protected paused task/worktrees/volume,
foreign refs/caches/processes, or managed dependencies.

## Acceptance

1. Real MCP Chart and Table calls with file-leading BOM/quoted first headers
   succeed in task and Office modes with exact expected data. Each fails RED
   with the qualified header error before the production correction; paired
   no-BOM quoted and unquoted BOM controls pass.
2. A combined report resolves its shared `(repo, path)` with one authorized
   read. Both result representations carry the exact version/element/column
   mapping, chart numbers/nulls and raw selected Table cells, excluding unused
   columns and preserving embedded U+FEFF.
3. Header-only Table/Chart behavior and malformed quoting, source admission,
   finite numbers and all existing bounds stay strict. Rejections produce no
   partially resolved structured report.

## Implementation sequence

Execute only after ROOT reviews this design and sends a later separate
same-primary implementation interrupt with an explicit heavy-lease grant.
Keep the selected implementation profile; no delegates, new sessions, tasks or
model switch. You are not alone in the shared codebase; accommodate others'
changes and never revert them.

1. Re-read current owned source and artifacts; mark this order `in_progress`.
   Activate the existing Go 1.26.0, Node 24.18.0 and pnpm 9.15.9 binaries below.
   If `apps/` lacks normal-hook dependencies, one routine
   `(cd apps && pnpm install --frozen-lockfile)` is allowed after the later
   implementation/heavy grant. Keep the managed worktree/dependencies and do
   not add dependencies, alter lockfiles or bypass hooks.
2. Write the new permanent report tests. Use the existing real `callTool` seam,
   transport fixture and snapshot assertions. Run the RED command and retain
   actual output/handle/exit. Both independent quoted/BOM cases must fail for
   the expected tokenization cause, while meaningful controls pass; combined
   positive cases may also fail for that same cause. An incomplete compile or
   timeout gives no assertion verdict.
3. Add the shared constructor using `strings.TrimPrefix(content, "\ufeff")`
   before `csv.NewReader`. Route both parsers through it. Preserve original
   content size/UTF-8/binary admission and cached responses unchanged.
4. Run focused GREEN, complete owning-package race tests, affected complete-
   package Go lint and document gates below. Join every command handle and
   record actual results. Do not repeat passing checks without a changed or
   unresolved concern. CI owns full backend lint and broad suites.
5. Update Results and the plan, then follow the already-authorized normal
   delivery flow with active hooks. Heavy lease remains governed by ROOT's
   explicit grant/return; no heavy commands at the design checkpoint.

## Test details

Follow the sibling plan's exact three test names and fixtures. For the mixed
snapshot test use selected `service`, `cost`, and text-evidence fields plus an
unselected sentinel field. Include `00420`, an empty cost, raw whitespace,
escaped quotes, formula/markup text and a data-cell U+FEFF. Assert the actual
structured result and decoded `mcp.TextContent` JSON fallback against the same
complete expected snapshot. Check read count and session/path/repo payload.

For strict controls use BOM/quoted valid headers so NaN/Inf/overflow rejection
demonstrates numeric validation after successful header admission. Pair
header-only Table success (`rows: []`) with Chart's minimum-row error. Exercise
malformed header and data quotes, duplicate/blank header, row width, non-leading
or repeated BOM before a quote, binary status, invalid UTF-8 with `is_binary`
false, declared/actual file size, and the original-byte-size boundary. Check
appropriate error text and nil structured content. Existing focused suites
provide row, label, selected-cell and combined snapshot bound evidence.

## Verification

Run from repository root with the existing toolchain on PATH:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.nvm/versions/node/v24.18.0/bin:/home/jcfs/.local/share/mise/installs/pnpm/9.15.9/bin:$PATH"
export GOTOOLCHAIN=local
```

These implementation commands are planned, not run
at design time. ROOT's one global heavy lease must cover tests, lint and active
commit hooks; retain every native/session handle and join it before proceeding.
Use the warmed race/trimpath/fts5 cache and the same bounded compile/runtime
settings for each Go check. GNU `timeout` allows 620 seconds then a 10-second
kill grace; a timeout or incomplete compilation is not a test verdict.

RED before production edits:

```bash
(cd apps/backend && timeout --kill-after=10s 620s env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -race -tags=fts5 -p=1 ./internal/mcp/server -run '^TestRichOutputCSVHeaderBOM' -count=1 -v)
```

GREEN and owning-package regression checks after the correction:

```bash
(cd apps/backend && timeout --kill-after=10s 620s env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -race -tags=fts5 -p=1 ./internal/mcp/server -run '^TestRichOutput' -count=1 -v)
(cd apps/backend && timeout --kill-after=10s 620s env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -race -tags=fts5 -p=1 ./internal/mcp/server -count=1)
GOMAXPROCS=2 GOMEMLIMIT=512MiB bash scripts/lint-go-changed apps/backend/internal/mcp/server/rich_output_csv.go apps/backend/internal/mcp/server/rich_output_table_csv.go apps/backend/internal/mcp/server/rich_output_csv_bom_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/rich-output-csv-header-bom
```

The lint helper deduplicates these files into the complete
`./internal/mcp/server` package. Do not run full backend lint locally.

Read-only/light documentation coverage preflight from repository root, usable
at design time and after implementation. The future production paths below
exercise the real coverage requirement before a docs-only diff would be exempt;
this is a planned-scope preflight, not evidence of production changes.

```bash
node - <<'JS'
const fs = require('fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/plans/rich-output-csv-header-bom/plan.md',
  'docs/plans/rich-output-csv-header-bom/task-01-csv-reader-admission.md',
  'docs/specs/agents/requirements/agent-rich-output.md',
  'docs/specs/agents/system-design/agent-rich-output.md',
];
const source = [
  'apps/backend/internal/mcp/server/rich_output_csv.go',
  'apps/backend/internal/mcp/server/rich_output_table_csv.go',
];
const changedFiles = [...docs, ...source].map(filename => ({ filename, status: 'modified' }));
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.errors.length || result.workOrders.length !== 1) process.exitCode = 1;
JS
```

Before publishing, also evaluate actual tracked/untracked changed paths through
this validator as in the completed structured-evidence plan's preflight; include
all referenced owning documents. Do not substitute validator unit suites for
checking the actual package.

## Files likely touched

- `apps/backend/internal/mcp/server/rich_output_csv.go`
- `apps/backend/internal/mcp/server/rich_output_table_csv.go`
- `apps/backend/internal/mcp/server/rich_output_csv_bom_test.go` (new)
- `docs/specs/agents/requirements/agent-rich-output.md` (plan link only)
- `docs/specs/agents/system-design/agent-rich-output.md` (shared admission and link)
- `docs/plans/rich-output-csv-header-bom/plan.md`
- `docs/plans/rich-output-csv-header-bom/task-01-csv-reader-admission.md`

## Dependencies

No earlier work order. Requires the later explicit implementation interrupt and
ROOT heavy-lease grant. Existing completed Chart/Table/catalog packages are
baseline context, not reopened work.

## Risks

Stripping at the wrong boundary could change source admission or literal cells;
reusing chart conversion for Table would lose `00420` or blanks. Exact result
checks and strict malformed/byte/number controls constrain both risks.
No frontend change or UI preview is required; desktop semantic Tables, phone
labelled row fields, inline Charts and localized empty/error outcomes remain as
described in the plan.

## Parallelism

`sequential`

## Inputs

- [Owning requirements](../../specs/agents/requirements/agent-rich-output.md).
- [Owning design](../../specs/agents/system-design/agent-rich-output.md), especially CSV table snapshots and persistence.
- [Sibling plan](plan.md), including qualified ROOT evidence and test matrix.
- `apps/backend/AGENTS.md` and `.agents/skills/tdd/references/backend-tests.md`.
- Existing `rich_output_test.go`, `rich_output_table_csv_test.go`, and
  `rich_output_composition_test.go` tool-dispatch and snapshot/cache controls.

## Results

Implemented after ROOT's later explicit release. The production change adds
one strict reader constructor and routes both parsers through it. Existing
source/header/cell/number/cache/snapshot/schema behavior remains in place.

All Go commands used the reviewed GNU 620s timeout/10s kill grace, race detector,
trimpath, fts5 tag, `-p=1`, `GOMAXPROCS=2`, `GOMEMLIMIT=512MiB` and
`GOTOOLCHAIN=local`:

- RED command above: native 59384 actually joined exit 1 (chunk 82b4a3).
  Four independent task/Office Chart/Table quoted-BOM cases failed with the
  known header error; eight paired controls passed. Assertion time 1.824s.
  Additional BOM-dependent expectations failed at the same header boundary.
- Focused GREEN command above: native 6718 actually joined exit 0 (77d4f5),
  25 top-level tests, all 47 new BOM subcases passed, 8.068s.
- Complete owning-package command above: native 90163 actually joined exit 0
  (aa9be6), 32.304s.
- Affected-package lint command above: native 42367 actually joined exit 0
  (bcb6b7), zero issues.
- Catalog and full specification lint commands: exit 0.
- Planned-production and actual changed-file coverage evaluations: exit 0,
  covered, exactly one order, all three owning requirements, zero errors.

The invalid-UTF-8 control uses a raw file-response transport fixture because
the existing generic JSON fixture replaces invalid bytes. No parser or handler
is mocked. Exact selected cells, header-only behavior, both output forms and
strict quote/byte/number controls pass through registered MCP dispatch.
No other production export or test-only hook was added.

Logs and coverage JSON are retained in the task-owned
`/tmp/kandev-csv-bom-implementation-20261013-chZjVp/` directory. ROOT's private
qualification evidence was neither changed nor replayed. Publication, normal
hook receipts and exact-head hosted checks remain external delivery evidence;
there is no merge claim here.
