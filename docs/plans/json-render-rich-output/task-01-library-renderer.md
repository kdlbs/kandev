---
id: "01-library-renderer"
title: "Replace composition with the library renderer"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.5
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.6
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.7
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.8
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.9
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.10
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.11
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.12
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.13
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.14
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.19
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.1
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.2
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.3
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.4
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.5
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.7
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.8
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.11
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
---
# Task 01: Library renderer and legacy replay

## Summary

Establish the pure catalog and real json-render composition path, converting
existing version 1 presentations to it. The current live tool remains usable
while the frontend can render validated version 2 fixture compositions and
snapshots for the next work order.

## In scope

- Add pinned core/react 0.21.0 and compatible Zod 4 dependencies and lockfile
  changes. Install workspace dependencies before the first package command in
  a fresh worktree.
- Implement the six-component closed catalog and deterministic schema generator
  with non-writing check mode. Wire drift checking into the relevant build/CI
  scripts. Add shared valid/invalid conformance cases and generated examples.
- Separate legacy decoding from v2 catalog/graph validation; normalize both
  versions to immutable resolved specs and exact CSV snapshot sets.
- Replace the fixed JSX dispatcher with actual json-render Renderer and a stable
  native registry. Keep the host shell, trusted context, localized failures,
  native chart/file implementations, and independent file identity.
- Add TDD coverage using the actual library and registry. Extend full-consumer
  state tests for both versions, grouping changes, and viewport changes.

## Out of scope

New backend tool acceptance, prompt changes, forms/actions, streaming patches,
MCP Apps, stock component catalogs, and changes to file or chart APIs.

## Acceptance

1. Valid v1 and v2 fixture inputs use the real library renderer; arbitrary trees,
   props, versions, and mismatched snapshots fail closed without side effects.
2. Legacy content/ordering, chart motion/filtering, lazy file previews,
   target-replacement races, independent cards, and same-target state survive
   through the full consumer with only external file transport mocked.
3. Catalog/schema export is deterministic, drift checking rejects stale output,
   and UI-01/UI-02 responsive composition preserves mounted local state.

## ASCII UI preview

Excerpt from [the full preview](plan.md#ascii-ui-preview), UI-01 and UI-02:

```text
UI-01 desktop: [Checks | Interpretation] -> [Chart] -> [File: Preview / Open]
UI-02 phone:   [Checks] -> [Interpretation] -> [Chart] -> [File: Preview / Open]
UI-03 invalid: [localized presentation unavailable; no reads or actions]
```

These are actual ordered groups, not alternate viewport trees. Titles are
illustrative data. Preserve the full preview's scroll, focus, and control rules.
Covers AC-002.1/.7/.8 and AC-001.13/.14.

## Verification

Run these blocks from the repository root after dependency/lockfile edits:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec tsx scripts/generate-rich-output-catalog.ts --check)
(cd apps/web && pnpm exec vitest run lib/rich-output components/task/chat/messages/kandev/rich-output scripts/generate-rich-output-catalog.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/rich-output components/task/chat/messages/kandev/rich-output scripts/generate-rich-output-catalog.ts scripts/generate-rich-output-catalog.test.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

The first install bootstraps a new worktree; do it once before package tooling,
then update dependencies through pnpm and validate the final frozen lockfile.
The proposed test paths are created by this work order. Product checks remain
pending in this design turn.

## Files likely touched

- `apps/web/package.json`, `apps/pnpm-lock.yaml`
- Proposed `apps/web/lib/rich-output/catalog.ts`, `types.ts`, `normalize.ts`,
  `legacy.ts`, `validate.ts`, and their targeted tests
- Proposed `apps/web/scripts/generate-rich-output-catalog.ts` and test
- Proposed backend `rich_output_catalog_v1.schema.json` and
  `testdata/rich_output_composition_cases.json`
- Existing `rich-output/rich-output-renderer.tsx`, `parse.ts`, `types.ts`, renderer
  and file-target tests; proposed `registry.tsx` and registry test
- Existing `chart-block.tsx`, `metrics-block.tsx`, `file-preview-block.tsx` only
  for catalog-derived props and state/geometry corrections required by the replacement
- Locale task catalogs only if new host copy is required

Paths abbreviated above are below
`apps/web/components/task/chat/messages/kandev/` unless otherwise stated.

## Dependencies

None. Read the owning requirement/design, dependency evidence, native-output
ADR, and shipped chart-performance/file-lifetime records before coding.

## Risks

Library schema validation can accept features outside the exposed catalog.
Changed registry or adapter identity can remount expensive or stateful leaves.
Avoid automatic schema repair and a parallel legacy rendering path.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/agent-rich-output.md)
- [Design](../../specs/agents/system-design/agent-rich-output.md), especially
  Catalog, Validation, Single rendering path, and Native components
- `apps/web/AGENTS.md`, chat scoped guidance, `/tdd`, and `/mobile-parity`

## Results

Completed 2026-10-08. Installed pinned core/react 0.21.0 and Zod 4.3.6; replaced
composition with the actual library registry/Renderer. Native content takes
catalog props; old parser/types are confined to historical decoding. Removed
legacy dispatch and special styling, and omitted the unused MIME hint.

- RED: valid v2 composition was rejected by the old parser; real-renderer v2
  scenarios failed before wiring; component failure previously disappeared;
  generic library schema export lacked CSV property contracts.
- GREEN: targeted Vitest command passed 10 files / 111 tests including shared
  conformance, historical replay, exact CSV snapshot identity, real-renderer
  preview races, chart scheduling, generator drift, and localized failures.
- `pnpm run typecheck`, targeted ESLint, `pnpm run i18n:check`,
  `pnpm run i18n:ratchet`, generator check, and `git diff --check`: passed.
- Frozen-lockfile bootstrap passed; dependencies then updated through pnpm.
  Final delivery rechecks the updated frozen lockfile.

Native phone hierarchy is implemented through the canonical breakpoint in one
mounted tree. Browser geometry verification belongs to task 02.
