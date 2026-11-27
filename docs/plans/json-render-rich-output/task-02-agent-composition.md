---
id: "02-agent-composition"
title: "Deliver version 2 agent composition end to end"
status: done
wave: 2
depends_on:
  - "01-library-renderer"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.1
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.2
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.3
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.4
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.5
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.6
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.7
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.8
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.13
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.14
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.15
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.16
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.17
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.18
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.19
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.20
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.1
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.2
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.3
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.4
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.5
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.6
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.7
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.8
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.9
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.10
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.11
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
---
# Task 02: Agent composition and CSV contract

## Summary

Enable agents to send version 2 catalog compositions through the existing native
MCP tool. Deliver validated element-addressed CSV snapshots, refreshed discovery,
and real desktop/mobile transcript flows using the renderer from task 01.

## In scope

- Embed the generated catalog schema and extend the existing portable version 2 object envelope. Reject cross-version fields and malformed graphs
  before source reads. Keep current read-only/profile/authorization behavior.
- Add version 2 CSV resolution keyed by element ID using one version-neutral data resolver and per-call source caching. Preserve finite numeric,
  row/byte limits, null gaps, result size, and JSON text/structured parity.
- Consume the same Go/TS conformance corpus and schema examples; include invalid
  graph/source combinations proving no workspace read occurs before rejection.
- Update Task `richOutputSection`, Office context, discovery examples, and
  instruction tests within 650/750-rune budgets. Prefer tool-call composition,
  not raw JSONL patch output.
- Extend real result-extraction/trusted-identity fixtures for both versions.
  Preserve foreign/incomplete fallback and standalone activity selection.
- Extend existing browser fixtures/specs for v2 nested/horizontal composition,
  retained v1 replay, CSV deletion/reload, filtering/motion, file opening,
  responsive state retention, and phone/coarse-pointer geometry.

## Out of scope

New ACP provider normalization, a new agent transport, persistent Kandev task
creation as a work mechanism, live Office/provider evaluation claims from mocks,
new catalog actions, feature flags, and public documentation publication.

## Acceptance

1. Real MCP calls accept valid v2 compositions and reject invalid shapes before
   reads; Go and frontend agree on the corpus and snapshots, while v1 history remains readable and stale v1 calls fail before reads and advertised profiles remain unchanged.
2. Inline/CSV/metric examples use the new contract, fit instruction budgets,
   and arrive through existing normalized persistence and trusted dispatch.
3. Chromium and mobile-chrome demonstrate UI-01/UI-02, legacy replay, chart/file
   interactions, retained local state, and unchanged accepted CSV data after
   source deletion/reload. Invalid new calls render normal tool errors.

## ASCII UI preview

Excerpt from [the full preview](plan.md#ascii-ui-preview):

```text
UI-01 desktop
[Build health]
[Checks | Interpretation]
[Chart: axes + summary + legend]
[Evidence file: Preview / Open]

UI-02 phone
[Build health]
[Checks]
[Interpretation]
[Contained chart + wrapping legend]
[Evidence file]
[Preview] [Open]  (44px touch targets)

UI-03 pending/error
[compact tool activity] -> [valid report OR normal tool error]
```

Primary scroll remains the transcript; Open uses the existing viewer. Preserve
state when the viewport changes. Covers AC-002.1/.6/.7/.8/.9/.11 and
AC-001.5/.6/.13/.14/.17/.18.

## Verification

Run from the repository root, sequentially:

```bash
(cd apps/backend && go test -trimpath ./internal/mcp/server/... ./internal/sysprompt/... -count=1)
(cd apps/web && pnpm exec tsx scripts/generate-rich-output-catalog.ts --check)
(cd apps/web && pnpm exec vitest run lib/rich-output components/task/chat/messages/kandev/rich-output components/task/chat/messages/kandev-tool-message components/task/chat/messages/kandev/types.test.ts hooks/use-processed-messages-rich-output.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium -- tests/chat/rich-output.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- tests/chat/mobile-rich-output.spec.ts)
(cd apps/web && pnpm run lint:e2e-sleeps -- e2e/tests/chat/rich-output.spec.ts e2e/tests/chat/mobile-rich-output.spec.ts e2e/tests/chat/rich-output-helpers.ts)
git diff --check
```

The first guarded E2E command builds the current artifacts. Run the second only
after the first completes and no product code changes. Retain and join any
returned process handle. Use `/e2e` for fixtures, causal waits, and failure
investigation. Do not overlap full suites or add worker overrides.

## Files likely touched

- `apps/backend/internal/mcp/server/rich_output.go`, `rich_output_csv.go`,
  `rich_output_test.go`, and proposed `rich_output_composition_test.go`
- Generated catalog schema and shared server `testdata` corpus from task 01
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`,
  `tool_metadata_test.go`, and server profile/argument-validation controls
- `apps/backend/internal/sysprompt/sysprompt.go` and tests
- `apps/backend/config/prompts/office-context.md`
- Frontend normalization tests, native full-consumer renderer tests,
  `components/task/chat/messages/kandev-tool-message`, chat predicate,
  and rich-output processing tests
- `apps/web/e2e/tests/chat/rich-output-helpers.ts`, `rich-output.spec.ts`,
  `mobile-rich-output.spec.ts`

## Dependencies

Task 01. Its generated schema and real renderer must pass before advertising
version 2. No parallel authoring/renderer branch is authorized.

## Risks

Root schema combinators break provider portability. Position-based CSV
resolution can attach values to the wrong node. Stale version 1 callers need an actionable discovery-refresh error. Browser tests cannot imply live provider smoke coverage.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/agent-rich-output.md)
- [Design](../../specs/agents/system-design/agent-rich-output.md), especially
  Version 2 contract, Validation, Persistence, Provider identity, and Responsive
  composition
- Backend/frontend scoped AGENTS guidance, `/tdd`, `/e2e`, `/mobile-parity`

## Results

Completed 2026-10-08. The live MCP tool accepts only version 2/catalog version 1,
using the generated catalog schema and semantic graph/path/data guards. CSV
results bind exact element IDs; a shared source is read once per call in Task
and Office. Historical v1 decoding remains frontend-only; stale authoring fails
before file access with discovery-refresh guidance.

- RED: old backend rejected v2 authoring; a valid 64-node plain-text report was
  overcounted by optional HTML escaping; a missing inherited child crashed the
  frontend; measured pressed phone legend controls shrank below 44px.
- GREEN: server and sysprompt tests pass via the command above, including the
  shared 39-case corpus, generated examples, profile/authorization guards, CSV
  parsing failures, no-read negatives, and element-addressed snapshots.
- Targeted Vitest: 14 files / 155 tests passed. The message-test prefix includes
  rich-output/provider-envelope and CSV result-selection suites. No library
  Renderer or registry is mocked in the composition consumer tests.
- `pnpm run typecheck`, full `pnpm run lint`, final changed-file ESLint,
  schema drift check, `pnpm run i18n:check`, `pnpm run i18n:ratchet`, E2E sleep
  lint, and `git diff --check`: passed. Final core/react leaf styling changes
  passed changed-file lint after the full-tree lint receipt.
- `/home/zeval/go/bin/golangci-lint run ./internal/mcp/server/...
  ./internal/sysprompt/... --new-from-rev=HEAD --timeout=10m`: passed, zero issues.
- Guarded production-build Chromium run: four passed, including CSV source
  deletion/reload, native file opening, static chart geometry, historical v1
  replay, and narrow fine-pointer layout/control geometry.
- Immediately following mobile-chrome run with `--no-build`: two passed,
  including measured 44px minimum targets after pressed scaling, chart filters,
  vertical grouping, containment, native viewer, and historical CSV replay.
- Desktop and phone viewport captures were inspected against UI-01/UI-02.
  Desktop and phone captures are privately retained verification evidence.

Native controls use 48px nominal phone/coarse-pointer height to preserve a
44px minimum when pressed. The input byte limit avoids optional HTML escaping,
so literal markup characters remain plain data within the agreed bound.
Browser evidence covers Task delivery; Office/profile and provider envelopes
are covered by integration/full-consumer fixtures, not live provider smoke.
