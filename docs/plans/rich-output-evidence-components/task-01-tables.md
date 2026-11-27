---
id: "01-tables"
title: "Tables and CSV snapshots"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
  - REQ-AGENTS-AGENT-RICH-OUTPUT-003
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.1
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.2
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.3
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.4
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.5
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.9
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.10
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.2
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.3
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.4
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.5
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.6
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.8
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
---
# Task 01: Tables and CSV snapshots

## Summary

Add inline and CSV Tables end to end in the existing catalog, MCP resolver,
normalizer and native registry. New reports render through the real json-render
Renderer and persist selected cells without depending on the source at replay.

## In scope

- Closed Table schemas, unique columns, literal cell limits, row widths and
  Table content-count parity in generated discovery and Go/TypeScript guards.
- Reuse authorized bounded CSV reads/header indexing/cache; add a table parser
  and element-addressed resolved_tables snapshots. Chart-only output stays
  compatible, mixed chart/table sources share reads, and failures are atomic.
- Exact snapshot matching and immutable normalization for structured and JSON
  text results, including missing/extra/wrong-type/column mismatch negatives.
- Desktop semantic table and phone labelled rows, blank/empty states, localized
  host copy, wrapping and no phone horizontal scroll. Add inline/CSV examples.
- TDD and Table-named cases in the new desktop/mobile suites; source deletion
  and replay, mixed sources, long fields, empty table and breakpoint checks.

## Out of scope

StatusList, Callout, stock catalogs, actions, filters/sort/export, dependency
upgrades, new transport/storage, historical decoder changes and public-doc
publication before the final integration work order.

## Acceptance

1. Real MCP calls accept bounded inline/CSV Tables, fail invalid input before
   reads, preserve chart-only results and resolve mixed sources atomically
   through current session authorization and a per-call shared file cache.
2. The real Renderer displays exact accepted cells on desktop and phone from
   either result representation; deletion/reload never reads a CSV and invalid
   snapshot bindings fail closed while legacy history retains its meaning.
3. Table-named browser cases demonstrate UI-01/UI-02/UI-03, all fields and empty
   values, no phone document overflow, and unchanged adjacent chart/file state.

## ASCII UI preview

Excerpt from [the full preview](plan.md#ascii-ui-preview). Covers the linked
AC-003 criteria for this work order. Agent content is illustrative; host labels
are localized. Preserve transcript scrolling and all fields on phone.

```text
UI-01 desktop
Services
Service      Monthly cost (USD)    p95 (ms)
API          420                   165
Payments     680                   240

UI-02 phone
[Service: API]
[Monthly cost (USD): 420]
[p95 (ms): 165]
[Service: Payments]
[Monthly cost (USD): 680]
[p95 (ms): 240]

UI-03 empty / blank
Services -> [No rows]
Optional note -> [Empty]
```

## Verification

Run from repository root, sequentially, using `/tdd`, `/e2e` and `/mobile-parity`.
If this is a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`
once. After schema edits, run `(cd apps/web && pnpm run rich-output:generate)`
before the non-writing drift check. Do not substitute stale production assets.

```bash
(cd apps/web && pnpm run rich-output:check)
(cd apps/backend && go test -trimpath ./internal/mcp/server/... -run TestRichOutput -count=1)
(cd apps/web && pnpm exec vitest run lib/rich-output components/task/chat/messages/kandev/rich-output scripts/generate-rich-output-catalog.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/rich-output components/task/chat/messages/kandev/rich-output)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium -- tests/chat/rich-output-evidence.spec.ts -g Table)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- tests/chat/mobile-rich-output-evidence.spec.ts -g Table)
git diff --check
```

The first guarded browser run builds the current artifacts. Run mobile with
`--no-build` only after it finishes and no product code has changed. Retain and
join every process handle; inspect the phone rendering against UI-02. New
test files above are implementation targets created by this work order.

## Files likely touched

- `apps/backend/internal/mcp/server/rich_output.go, rich_output_csv.go`
- `apps/backend/internal/mcp/server/rich_output_table_csv.go and rich_output_table_csv_test.go (new)`
- `apps/web/lib/rich-output/normalize.ts and normalize.test.ts`
- `apps/web/components/task/chat/messages/kandev/rich-output/table-block.tsx and table-block.test.tsx (new)`
- `apps/web/lib/rich-output/catalog.ts, validate.ts, examples.ts and their tests`
- `apps/backend/internal/mcp/server/rich_output_catalog_v1.schema.json (generated)`
- `apps/backend/internal/mcp/server/testdata/rich_output_composition_cases.json`
- `apps/web/components/task/chat/messages/kandev/rich-output/registry.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer.test.tsx`
- `apps/web/e2e/tests/chat/rich-output-helpers.ts`
- `apps/web/e2e/tests/chat/rich-output-evidence.spec.ts and mobile-rich-output-evidence.spec.ts (new)`
- `apps/web/src/locales/<locale>/task.json and generated Traditional Chinese catalogs`
- `Owning requirements/design and this plan/work-order status/results`

## Dependencies

None. The prior replacement is implemented locally.

## Risks

CSV table parsing must retain text and blanks rather than reuse chart numeric
coercion. Mixed snapshots must not attach data by traversal position. Existing
UI Table nowrap/overflow defaults need explicit responsive adaptation.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/agent-rich-output.md), especially requirement 003.
- [Design](../../specs/agents/system-design/agent-rich-output.md), especially the structured evidence extension, CSV table snapshots and presentation/discovery sections.
- Existing catalog, renderer, CSV parser, test corpus and relevant scoped AGENTS guidance.
- [Replacement package](../json-render-rich-output/plan.md) for unchanged historical regression evidence.

## Results

Completed on 2026-10-10. Red producer/consumer cases were observed before implementation.
The first desktop browser case passed. The phone case exposed chat remounting
at the tablet boundary, which reset the neighboring file disclosure. Added
bounded, host-only json-render view stores to retain preferences without file
bytes; unit regression passes (162 tests). Current-build Chromium and mobile-chrome
Table cases both pass, including source deletion and 767/768/390px layout
switches. Backend TestRichOutput, generated drift check, typecheck, focused
eslint, i18n:check, i18n:ratchet and git diff --check pass. Phone screenshot
matches the planned labelled-field hierarchy, with all eight fields wrapping. Component presentation is
covered through the real Renderer and browser outcomes rather than tests that
mirror pure markup.
