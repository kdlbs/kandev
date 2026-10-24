---
id: "03-callouts-and-discovery"
title: "Callouts and discovery integration"
status: done
wave: 3
depends_on:
  - 02-status-lists
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
  - REQ-AGENTS-AGENT-RICH-OUTPUT-003
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.1
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.8
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.9
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.10
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.11
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.13
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.14
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.17
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.18
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.4
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.6
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.7
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.8
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.9
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.10
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.11
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
---
# Task 03: Callouts and discovery integration

## Summary

Add static Callout notes, then reconcile discovery, public guidance and
integration evidence for all three catalog additions. Confirm natural-prompt
usage in the isolated real-Codex instance without scripting the user recipe.

## In scope

- Closed Callout props and corpus with all kinds, title/text bounds and unknown
  properties. Render kind icon/localized label and plain agent text with a note
  role; Callout remains text-like under graph/byte bounds, not a data-content node.
- Callout-named desktop/mobile cases, combined Table/StatusList/Callout report,
  long/literal text and note-role semantics without live announcements.
- Descriptions/examples and task/Office routing guidance for all three leaves,
  preserving instruction budgets, task/Office profile visibility and Markdown
  for small textual comparisons. No user component recipes are required.
- Full-consumer structured/JSON-text result and identity negatives; mixed report
  state retention through transcript updates and desktop-phone-desktop, plus
  current v1/v2 replay/file/chart tests and all new browser cases.
- Apply docs-maintainer to public MCP reference and scoped web engineering
  guide. Reconcile owning specs and plan lifecycle only after checks pass.
- Record one isolated real-Codex run and browser evidence using the natural
  prompt below; do not claim fixture results are live provider results.

## Out of scope

Progress, arbitrary styling/HTML, action buttons, callback/event bridges,
new providers, altered identity matching, database migration, broad review/QA,
commit/push/PR and all main-instance mutations.

## Acceptance

1. All four Callout kinds render literal content with localized icon/label and
   static note semantics through the existing registry on desktop and phone;
   malformed values fail closed without resetting other components.
2. Generated discovery, task/Office guidance and public docs describe all three
   additions within existing budgets; examples pass actual producer/consumer
   validation, old fixtures retain meaning and all package browser tests pass.
3. The isolated natural-prompt trial records the real model, selected components,
   successful/failed tool outcome and rendered report. Guidance issues or missing
   evidence remain explicit; plan/spec promotion waits for required outcomes.

## ASCII UI preview

Excerpt from [the full preview](plan.md#ascii-ui-preview). Covers the linked
AC-003 criteria for this work order. Agent content is illustrative; host labels
are localized. Preserve transcript scrolling and all fields on phone.

```text
UI-01 desktop
[Warning: Investigate Payments]
[Highest cost and slowest service in this sample]

UI-02 phone
Warning: Investigate Payments
Highest cost and slowest service
in this sample

UI-03 literal / invalid
<script>... -> plain visible text
Bad kind -> normal tool error / unavailable historical presentation
```

## Verification

Run from repository root, sequentially, using `/tdd`, `/e2e` and `/mobile-parity`.
If this is a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`
once. After schema edits, run `(cd apps/web && pnpm run rich-output:generate)`
before the non-writing drift check. Do not substitute stale production assets.

```bash
(cd apps/web && pnpm run rich-output:check)
(cd apps/backend && go test -trimpath ./internal/mcp/server/... ./internal/sysprompt/... -count=1)
(cd apps/web && pnpm exec vitest run lib/rich-output components/task/chat/messages/kandev/rich-output components/task/chat/messages/kandev-tool-message components/task/chat/messages/kandev/types.test.ts hooks/use-processed-messages-rich-output.test.ts scripts/generate-rich-output-catalog.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium -- tests/chat/rich-output-evidence.spec.ts tests/chat/rich-output.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- tests/chat/mobile-rich-output-evidence.spec.ts tests/chat/mobile-rich-output.spec.ts)
(cd apps/web && pnpm run lint:e2e-sleeps -- e2e/tests/chat/rich-output-evidence.spec.ts e2e/tests/chat/mobile-rich-output-evidence.spec.ts e2e/tests/chat/rich-output-helpers.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The first guarded browser run builds the current artifacts. Run mobile with
`--no-build` only after it finishes and no product code has changed. Retain and
join every process handle; inspect the phone rendering against UI-02. New
test files above are implementation targets created by this work order.

## Natural-prompt trial

Use an isolated current-build instance with its own HOME, database, ports and
fictional repository; preserve the main listener on :9998. The user already
authorized real Codex (6 Luna) for test examples. Reuse that authorization and
the existing isolation patterns, not the old copied binary/assets. Do not
create workers using current-task Kandev MCP APIs. Use the disposable instance's
ordinary API to start a fixture session and stop only owned processes.

Provide fictional service CSV and verification results, then send exactly:

> Review the service costs and verification results in this repository. Compare
> the services, explain which checks passed or failed, and highlight the main
> issue worth investigating.

Do not append tool/component names, output format, schema, or hidden user
recipes. Normal Kandev context and discovered tools remain available. Record
actual model metadata, tool choice, accepted input/result, browser rendering,
URL and shutdown command. If no rich output or an unexpected subset is chosen,
record it as a discoverability limitation and improve advertised descriptions
within the approved scope. Deterministic tests prove every component; the real
trial proves the observed choice only and must not become a flaky CI gate.

Also run the documentation coverage preflight in the plan's Verification results
procedure over the final diff, inspect errors and accepted references, and
record the result. It complements spec lint and does not publish or commit.

## Files likely touched

- `apps/backend/internal/mcp/server/rich_output.go, sysprompt_sync_test.go, tool_metadata_test.go`
- `apps/backend/internal/sysprompt/sysprompt.go and related tests`
- `apps/backend/config/prompts/office-context.md`
- `apps/web/components/task/chat/messages/kandev/rich-output/callout-block.tsx and callout-block.test.tsx (new)`
- `apps/web/components/task/chat/messages/kandev-tool-message-rich-output.test.tsx`
- `docs/public/automation-and-mcp.md and apps/web/AGENTS.md`
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

02-status-lists. Shared catalog/schema/corpus files require sequential execution.

## Risks

Native Alert defaults to role=alert; override it so replay is not a new urgent
announcement. Rich-output choice is model-dependent: record real smoke results
honestly, and keep deterministic component coverage as the repeatable contract.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/agent-rich-output.md), especially requirement 003.
- [Design](../../specs/agents/system-design/agent-rich-output.md), especially the structured evidence extension, CSV table snapshots and presentation/discovery sections.
- Existing catalog, renderer, CSV parser, test corpus and relevant scoped AGENTS guidance.
- [Replacement package](../json-render-rich-output/plan.md) for unchanged historical regression evidence.

## Results

Completed on 2026-10-10.

- Observed six newly valid Callout corpus cases fail before adding the closed
  catalog schema. All 86 shared conformance cases now pass through Go and
  TypeScript, including four data-leaf bounds and literal explanatory Callouts.
- Current frontend regression: 17 files, 229 tests passed with the actual
  catalog, normalizer, registry and Renderer. Full-consumer coverage includes
  11 structured/text result and trusted-identity cases. Typecheck, full web lint,
  i18n checks/ratchet, schema drift and public JSON example validation passed.
- Producer and system-prompt tests passed; focused backend lint reported zero
  issues. All discovery corrections below passed the same producer/prompt checks and zero-issue backend lint.
- Shared Go cache artifacts disappeared during the initial guarded build.
  Rebuilt every required native helper, fresh E2E web assets and plugin fixture
  with a task-owned Go cache, then used the guarded runner with `--no-build`.
  The first final desktop run passed Table, StatusList and all four historical
  regression cases. The mixed-report case revealed a test scrolling assumption:
  lazy charts must be brought into view after responsive chat scroll restoration.
  The corrected mixed-report case passed (18.5s run). Final phone project:
  five tests passed (56.2s), covering all additions and historical CSV replay.
  Inspected the native Table, StatusList and Callout phone screenshots; fields
  and long text wrap within the transcript without document overflow.
- Real Codex ACP reported `gpt-6-luna` and completed the exact natural prompt in
  session `e205c27c-062f-4c2a-ba70-5459b2f1a5be`, task
  `abe60830-9470-4558-b3d2-7fc8a910ba14`. It chose Markdown and did not invoke
  rich output. This exposed chart-first routing in the advertised instructions.
  Added a red regression requiring structured comparisons and recorded checks
  as direct triggers, then corrected task/Office guidance and tool metadata
  within their unchanged size budgets. The unchanged repeat with Luna also
  selected Markdown (task `3368c39d-18c0-4b4f-ad08-9dfd92e5e6bd`). Both outcomes
  remain explicit discovery limitations; no user/component recipe was added.
- Real Codex ACP reported `gpt-6-sol` in session
  `f5ad42cd-59b0-4b53-ba07-d74226cdce19` and task
  `56918f81-e2e2-4e1c-ab2a-4523e05f420a`. The same natural prompt produced a
  successful version 2/catalog 1 Stack with CSV Table, StatusList and warning
  Callout. The accepted result retains four services/four selected columns in
  `resolved_tables`; the real browser verified desktop semantic headers and
  all 16 labelled phone fields, one static note and zero page errors/overflow.
  The model first made 12 rejected schema guesses. Advertised element/leaf
  shape guidance reduced ambiguity; a subsequent trial produced separate
  accepted Callout, StatusList, inline Table and CSV Table reports but still
  guessed columns. Added a red discovery assertion, then clarified exact
  `{key,label}` columns and string-array rows within the original 650-rune
  tool-description limit. All backend tests, size budgets and lint passed.
  The final unchanged-prompt trial, task
  `f3eecd44-0da0-4830-8a28-4b40c6ea0c52`, session
  `745be799-c407-423a-8ec5-0cc5d75882bb`, reported `gpt-6-sol` and made one
  successful rich-output call with no rejected attempts: Stack, inline Table
  (four services plus a total row, five columns), StatusList and warning Callout. It settled to
  WAITING_FOR_INPUT. The browser verified desktop/phone native rendering and
  all 25 phone label/value fields without page errors or horizontal overflow.
  This demonstrates observed composition, not universal model compliance.
  The supporting trial and browser evidence is privately retained.
- Public documentation validation passed (47 pages), owning spec/catalog lint
  passed (361 decisions, 1433 specifications), and final diff documentation
  coverage is covered with no errors and six accepted work-order references.
  `git diff --check` passed. Owning requirements/design are active/current;
  this package is implemented. Repository changes remain uncommitted.

Trial inputs/results and browser receipts are privately retained; native phone
screenshots were inspected. The disposable environment used direct Tailscale
access and has since been shut down at the user's request. Main :9998 remained
unchanged.
