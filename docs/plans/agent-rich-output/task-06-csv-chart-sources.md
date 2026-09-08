---
id: "06-csv-chart-sources"
title: "CSV-backed chart sources"
status: done
wave: 6
depends_on: ["05-e2e-public-docs"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-rich-output.md"
---

# Task 06: CSV-backed chart sources

## Acceptance

1. Existing inline line and bar charts remain valid.
2. A chart can name a workspace-relative CSV, x column, optional repository,
   and one to four numeric series columns without inlining row values.
3. Handler rejects unsafe, binary, oversized, malformed, empty, or invalid CSV
   input with actionable errors and no completed rich presentation.
4. Successful calls persist bounded normalized chart values in their MCP result.
   Reload renders that snapshot without reading the workspace again.
5. Tool description, schema descriptions, task/Office prompts, and public docs
   make explicit chart, graph, plot, preview, bar comparison, and time-series
   use cases discoverable while retaining Markdown-table restraint.
6. Desktop and phone E2E cover CSV-backed line and bar charts plus replay after
   source mutation or deletion.

## TDD order

1. Backend schema, resolver, snapshot, and prompt tests fail for behavior gaps.
2. Implement bounded CSV resolution in `internal/mcp/server`.
3. Frontend input/snapshot parser tests fail for replay gaps.
4. Implement strict result overlay and structured-content extraction.
5. Update real MCP E2E and public docs, then run focused and subtree checks.

## Verification

```sh
(
  set -e
  cd apps/backend
  go test ./internal/mcp/server/...
  cd ../web
  pnpm test -- components/task/chat/messages/kandev/rich-output components/task/chat/messages/kandev/parse.test.ts
  pnpm run typecheck
  pnpm run lint
  pnpm run i18n:check
  pnpm run i18n:ratchet
  pnpm e2e:run --project chromium tests/chat/rich-output.spec.ts -- --retries=0
  pnpm e2e:run --project mobile-chrome tests/chat/mobile-rich-output.spec.ts -- --retries=0
)
```

## Files likely touched

- `apps/backend/internal/mcp/server/rich_output.go`
- `apps/backend/internal/mcp/server/rich_output_test.go`
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`
- `apps/backend/config/prompts/kandev-context.md`
- `apps/backend/config/prompts/office-context.md`
- `apps/web/components/task/chat/messages/kandev/parse.ts`
- `apps/web/components/task/chat/messages/kandev/rich-output/types.ts`
- `apps/web/components/task/chat/messages/kandev/rich-output/parse.ts`
- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer.tsx`
- `apps/web/e2e/tests/chat/rich-output*.ts`
- `docs/public/automation-and-mcp.md`

## Risks

- Existing workspace reads are capped at 10 MiB, so reject response size before
  parsing and retain a tighter rich-output CSV cap of 256 KiB.
- Require exact one-to-one snapshot entries by chart block index. Reject stale,
  duplicate, missing, or inline-targeting entries at the frontend boundary.
- Return structured content and equivalent JSON text because ACP clients differ
  in how they retain MCP structured results.

## Results

- Added the mutually exclusive `csv` chart source, bounded workspace read,
  strict CSV parser, and versioned normalized result snapshot. Inline charts
  remain unchanged.
- Added fail-closed frontend snapshot overlay and ACP/standard MCP structured
  result extraction. Replay uses only persisted normalized values.
- Added explicit tool/schema examples, task and Office authoring guidance, and
  public line/time-series and bar/comparison examples.
- `go test ./internal/mcp/server/... -count=1`: passed.
- `golangci-lint run ./internal/mcp/server/... --timeout=5m`: zero issues.
- Focused rich-output Vitest: 3 files and 73 tests passed.
- `pnpm run typecheck`, full `pnpm run lint`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet`: passed. The i18n check retained only known advisory
  real-locale parity notices.
- Desktop Chromium and mobile Chrome rich-output E2E: one test each passed.
  Desktop deleted the CSV before reload; both charts remained. Mobile showed
  both charts without page overflow and retained native file-viewer behavior.
- Public docs: 61 validator tests and all 41 published pages passed.
