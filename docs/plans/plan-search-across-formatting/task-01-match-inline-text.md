---
id: "01-match-inline-text"
title: "Match Plan text across inline formatting"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PLAN-TEXT-SEARCH-001
acceptance_criteria:
  - AC-UI-PLAN-TEXT-SEARCH-001.1
  - AC-UI-PLAN-TEXT-SEARCH-001.2
  - AC-UI-PLAN-TEXT-SEARCH-001.3
  - AC-UI-PLAN-TEXT-SEARCH-001.4
  - AC-UI-PLAN-TEXT-SEARCH-001.5
  - AC-UI-PLAN-TEXT-SEARCH-001.6
system_design:
  - ../../specs/ui/system-design/plan-text-search.md
---

# Task 01: Match Plan Text Across Inline Formatting

## Summary

Derive Plan search ranges across consecutive text nodes in each textblock while
retaining the existing commands and state behavior. Prove the production plugin
with real document transactions and the shipped Plan panel with a focused
formatted-phrase browser regression.

## In scope

- Matcher correction following the owning design's run/boundary and position
  contracts.
- Independently authored permanent plugin tests and one addition to the
  existing Plan-search browser spec; all test groups in the plan are required.
- Exact checks and accurate requirement/design/plan/work-order lifecycle.

## Out of scope

All exclusions in [plan.md](plan.md#scope), including persistence, settings or
accepted-state races, task switching, companion searches, controls, copy,
layout, dependencies, and broad editor changes. Protected private discovery
source and foreign/shared resources are not implementation inputs.

## Acceptance

1. The real plugin passes all named test groups in
   [Tests](plan.md#tests), with exact document ranges, boundaries, logical
   decoration coverage, navigation, UTF-16 positions, and document edits.
2. The focused browser regression proves actual Markdown formatting, logical
   count, highlighted phrase, and wrapped current-hit navigation; existing
   basic highlighting and navigation scenarios still pass. The shared editor
   continues to satisfy the phone data-only parity assessment.
3. No integration contract or out-of-scope UI/persistence behavior changes;
   listed checks pass and statuses/results reflect actual implementation.

## ASCII UI preview

**UI-01: Existing Plan find region**, shared desktop/phone excerpt from the
[full preview](plan.md#ascii-ui-preview), criteria `.1`, `.3`, `.6`:

```text
Before: [deploy service] [0 / 0] [Prev] [Next] [Close]
        deploy B(service)
After:  [deploy service] [1 / 1] [Prev] [Next] [Close]
        [deploy B(service)] <- one current logical match
```

Brackets represent highlight, and `B(...)` existing bold formatting. Preserve
the top-right bar, editor scroll owner, and phone Plan destination above the
existing bottom navigation. This data preview adds no copy or geometry.

## Execution gate and TDD

ROOT reviewed the actual four artifacts and later explicitly released
same-primary implementation and the global local-heavy lease. Implementation
and the listed local checks completed under that release. Delivery continues
in the same primary without delegates, recursive tasks, model/session changes,
or touching paused taska6032d95. ROOT retains serial merge authority.

After release, re-read current source and package, reconcile any base movement,
and mark this work order `in_progress`. First independently write and run
`finds literal phrases across marks` against the production extension, proving
that formatted fixtures fail for missing matches and plain/block/absent controls
pass. Then implement the smallest matcher correction and run the full targeted
matrix. Do not inspect, replay, copy, modify, or clean up ROOT's protected private
source. Use freshly authored `release build` fixtures and real schema/state.

## Verification

Run these from the repository root after execution release. Commands root their
working directories independently. If this managed worktree has no dependencies,
report that to ROOT and follow its dependency lease; a newly created worktree
normally needs `(cd apps && pnpm install --frozen-lockfile)` once, but this work
order does not authorize an install or cache replacement by itself.

```bash
(cd apps/web && pnpm exec vitest run components/editors/tiptap/search-highlight-extension.test.ts components/editors/tiptap/tiptap-plan-editor.test.tsx components/task/use-plan-find-shortcut.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/editors/tiptap/search-highlight-extension.ts components/editors/tiptap/search-highlight-extension.test.ts e2e/tests/search/plan-search.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/search/plan-search.spec.ts -- --grep 'finds phrases across formatting and wraps navigation|highlights all matches|Enter advances' --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- apps/web/components/editors/tiptap/search-highlight-extension.ts apps/web/components/editors/tiptap/search-highlight-extension.test.ts apps/web/e2e/tests/search/plan-search.spec.ts docs/specs/ui/requirements/plan-text-search.md docs/specs/ui/system-design/plan-text-search.md docs/plans/plan-search-across-formatting
git status --short -- docs/specs/ui/requirements/plan-text-search.md docs/specs/ui/system-design/plan-text-search.md docs/plans/plan-search-across-formatting
```

For RED, run the first command limited to the new plugin file before applying
the matcher correction. Record expected causal failures and passing controls.
For browser evidence, confirm discovery of exactly the three named scenarios;
do not treat zero discovered tests as a pass. Managed E2E rebuilds assets,
enforces resource bounds, and must run headless under ROOT's lease. No full
suite, new mobile Playwright spec, or all-worker override is required.

Run the exported `.github/scripts/pr-docs.cjs::validateCoverage` local preflight
with the changed work order, its linked plan, owning requirement/design content,
and intended production matcher path. Require `ok: true`, with all six criteria
and the owning requirement/design validated. Use local files only; this is not
an instruction to query or open a PR. Actual PR CI and reviews remain delivery
gates when ROOT later authorizes publication. Do not rerun full bot review at
each SHA solely because the head changed.

## Files likely touched

- `apps/web/components/editors/tiptap/search-highlight-extension.ts`
- `apps/web/components/editors/tiptap/search-highlight-extension.test.ts` (new)
- `apps/web/e2e/tests/search/plan-search.spec.ts`
- These four design artifacts, for status/results reconciliation only.

Read-only integration inputs: `tiptap-plan-editor.tsx`, `use-plan-find-shortcut.ts`,
`task-plan-panel.tsx`, `components/search/panel-search-bar.tsx`, and
`components/task/mobile/session-mobile-layout.tsx` under `apps/web`.

## Dependencies

None. Existing Tiptap/ProseMirror, test tooling, and managed E2E fixtures.

## Risks

- Combining across structural nodes or double-scanning nested text creates
  false matches. Assert positive hits on both sides as well as negative joins.
- UTF-16 offsets and split decorations must survive repeated hits and edits;
  exact range/state evidence is required, not only aggregate counts.
- An ordinal selection clamp preserves current compatibility after deletions;
  do not invent occurrence identity or edit/save state coordination.

## Parallelism

`sequential`. No subagent/delegation authorization.

## Inputs

- [Owning requirement](../../specs/ui/requirements/plan-text-search.md).
- [Owning design](../../specs/ui/system-design/plan-text-search.md).
- [Root-cause evidence and limits](plan.md#evidence-and-root-cause).
- Root/scoped AGENTS guidance and `fix`, `planner-orchestration`, `tdd`,
  `mobile-parity`, `e2e`, and `docs-maintainer` skills in the primary session.
- Existing `tiptap-plan-editor.test.tsx`, `use-plan-find-shortcut.test.tsx`,
  `e2e/tests/search/plan-search.spec.ts`, and `e2e/tests/search/shared.ts` patterns.

## Results

Completed on 2026-10-13 after the later explicit release:

| Required check | Result |
| --- | --- |
| Frozen workspace install | Passed once; pnpm 9.15.9, all 935 packages reused |
| New plugin regression RED | 18 expected assertion failures, 12 passing controls; actual production extension and real ProseMirror schema/state |
| Listed Vitest command | 40 passed, 3 files, including all 30 plugin cases |
| Listed scoped ESLint | Passed with zero warnings after splitting test groups and centralizing the phrase fixture |
| `pnpm run typecheck` | Passed |
| `pnpm run i18n:ratchet` | Passed; all owned new code/tests staged, zero added copy, one modified source file clean |
| Listed managed E2E command, adding `--host` and `CAPTURE_PR_ASSETS=1` | 3 passed in 27.1s, Chromium, one worker/shard, fresh builds, zero retries |
| `python3 scripts/list-docs.py validate` | Passed; 379 decisions, 1580 specifications |
| `python3 scripts/lint-spec-files.test.py` | 36 passed |
| `python3 scripts/lint-spec-files.py --all` | Passed |
| Local PR documentation coverage preflight | Passed; owning requirement/design and all six acceptance references valid |
| Diff/status checks | Passed; exactly seven owned files, no foreign changes |

All native local handles were polled to actual join. The E2E log is
`/tmp/child117-84f121d2-delivery/kandev-run.e2e.8tGhUwZz.log` (session 70588,
exit 0). The fresh one-entry screenshot manifest identifies
`plan-search--plan-formatted-phrase.png`, inspected with synthetic data.
The shared plugin/editor/hook checks satisfy the documented phone data-only
exception; no rendered phone check is claimed. ROOT's private source was never
read, copied, replayed, edited, or cleaned up.

The final matcher conforms to the owning requirement/design, whose statuses
are active/current; plan is implemented and this work order done. Normal-hook
publication and hosted CI/review receipts belong in the external task plan.
