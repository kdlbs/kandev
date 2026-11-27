---
id: "02-status-lists"
title: "Snapshot status lists"
status: done
wave: 2
depends_on:
  - 01-tables
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
  - REQ-AGENTS-AGENT-RICH-OUTPUT-003
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.1
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.6
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.7
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.9
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.10
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.2
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.3
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
---
# Task 02: Snapshot status lists

## Summary

Add ordered StatusList findings to the same catalog and registry. Status
labels/icons represent the agent's recorded evidence, with no live execution
binding or task mutation.

## In scope

- Closed item/status schema and shared conformance cases, including every
  status, unknown status, Unicode/count bounds, no children, empty list and
  inclusion in the four-content limit; regenerate the schema.
- Native semantic list with icon, localized status label and optional detail.
  Long labels/details wrap in both viewports; empty copy is translated.
- Generated example and description explaining snapshot findings and when
  checks are better shown as a StatusList. Existing catalog examples stay valid.
- TDD and StatusList-named real-renderer/MCP/browser scenarios; assert snapshot
  labels do not poll or change workflow state, including running/pending.

## Out of scope

Callout, live task/execution authority, indeterminate spinners, check toggles,
completion controls, new actions, schema renumbering, dependency updates and
changes to the existing CSV or file-preview lifecycle.

## Acceptance

1. All five approved statuses render in declared order with visible localized
   labels/icons; unknown values/count bounds fail at producer and consumer.
2. StatusList composes with Table and existing content through the real Renderer,
   stays within the four-content bound, and remains static evidence with no
   callback, polling or task-state mutation.
3. Desktop/phone browser cases show UI-01/UI-02/UI-03 labels, wrapped details,
   empty state and unchanged neighboring component behavior.

## ASCII UI preview

Excerpt from [the full preview](plan.md#ascii-ui-preview). Covers the linked
AC-003 criteria for this work order. Agent content is illustrative; host labels
are localized. Preserve transcript scrolling and all fields on phone.

```text
UI-01 desktop
Verification
[check] Passed   Unit tests        38 checks
[cross] Failed   Load budget       p95 over target
[clock] Pending  Dependency audit  Not checked

UI-02 phone
Verification
[check] Passed
Unit tests
38 checks
[cross] Failed
Load budget
p95 over target
[clock] Pending
Dependency audit
Not checked

UI-03 empty
Verification -> [No checks]
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
(cd apps/web && pnpm e2e:run --project chromium -- tests/chat/rich-output-evidence.spec.ts -g StatusList)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- tests/chat/mobile-rich-output-evidence.spec.ts -g StatusList)
git diff --check
```

The first guarded browser run builds the current artifacts. Run mobile with
`--no-build` only after it finishes and no product code has changed. Retain and
join every process handle; inspect the phone rendering against UI-02. New
test files above are implementation targets created by this work order.

## Files likely touched

- `apps/backend/internal/mcp/server/rich_output.go and rich_output_composition_test.go`
- `apps/web/components/task/chat/messages/kandev/rich-output/status-list-block.tsx and status-list-block.test.tsx (new)`
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

01-tables. Shared catalog/schema/corpus files require sequential execution.

## Risks

A label such as running can be mistaken for a live session. Keep it visibly
static, use no spinner and document evidence ownership. Color alone cannot
carry the status, and localization must cover every enum.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/agent-rich-output.md), especially requirement 003.
- [Design](../../specs/agents/system-design/agent-rich-output.md), especially the structured evidence extension, CSV table snapshots and presentation/discovery sections.
- Existing catalog, renderer, CSV parser, test corpus and relevant scoped AGENTS guidance.
- [Replacement package](../json-render-rich-output/plan.md) for unchanged historical regression evidence.

## Results

Completed on 2026-10-10. Both frontend and backend shared-corpus cases failed
for the four newly valid StatusList scenarios before implementation. Generated
schema and producer validation now agree on all 72 cases, including Unicode
bounds, malformed items/children and the four-content limit. The focused
frontend regression passes 173 tests; typecheck, focused eslint (no errors),
i18n:check, i18n:ratchet, TestRichOutput and git diff --check pass.

Fresh Chromium and mobile-chrome StatusList cases pass: all five localized
labels/icons remain ordered, long details wrap, empty copy is visible and task
state/workflow step do not change. Renderer/browser outcomes cover native
markup without duplicate structural component tests.


### Badge color refinement (2026-10-12)

Applied host-owned green passed, red failed, blue running, amber pending and
neutral skipped to the existing badge/icon pairs. Labels, icons, snapshot
semantics, geometry and transcript scrolling remain unchanged. Light-mode
foregrounds use shade 700, dark-mode shade 400, with subtle tinted fills/borders.
The nearest phone exemplar remains this inline StatusList in chat.

Verification: component ESLint and Vite production build passed. Existing
catalog/generator/full-consumer tests passed (169 tests across nine files),
including restoration of the original discovery-example order after the added
FilePreview sample shifted index-based fixtures. The extra sample is appended;
all original examples retain their previous positions. No new markup-only unit
test was added.

The real persisted Luna report was checked with Playwright at 1440x1000 and
390x844 in both light and dark themes. Passed/failed/pending/skipped have distinct
computed badge colors matching their icons, tinted backgrounds, unchanged
labels, no page errors and no horizontal overflow. Screenshots were inspected;
private status-color receipts are retained with the session evidence.
The isolated lab was refreshed.
