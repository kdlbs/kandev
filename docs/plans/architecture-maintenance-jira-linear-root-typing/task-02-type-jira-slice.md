---
id: "02-type-jira-slice"
title: "Type the Jira issue-watch slice"
status: done
wave: 2
depends_on:
  - 01-refresh-maintenance-roadmap
plan: "plan.md"
requirements:
  - REQ-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001
acceptance_criteria:
  - AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.1
  - AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.2
  - AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.3
system_design:
  - ../../specs/architecture-lint/system-design/root-state-slice-typing.md
---

# Task 02: Type the Jira issue-watch slice

## Summary

Type Jira's issue-watch slice for the recipe-only Immer setter it uses, then
compose it directly into the root store. Preserve the existing six actions,
defaults, hydration, and unrelated root-state references.

## In scope

- Narrow `createJiraSlice` to the setter capability its current recipes use.
- Pass root `set` directly at the Jira creator call in `store.ts`.
- Add regression coverage for isolated defaults, actual root composition,
  the existing initial-state merge and `hydrateState` path, mutation isolation
  between independent stores, and unchanged unrelated root-state references.
- Remove only the three Jira root-state-cast baseline entries.

## Out of scope

- Provider APIs, credentials, issue-watch behavior, cache ownership, or UI.
- Changes to Jira action signatures/order, loading/reset semantics, or
  hydration shape; new Jira-specific boot or hydration routes.
- Generic root-state types, replacement-state overloads, a shared slice helper,
  test-only cast cleanup, or any non-Jira baseline entry.

## Acceptance

- `AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.1`: TypeScript accepts
  root composition with the Jira recipe setter and no `as any`/`as unknown as`
  escape at the Jira creator call.
- `AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.2`: Existing defaults and
  all six action semantics remain covered; root-store assertions exercise the
  existing initial-state merge, prove separate stores remain isolated after a
  Jira mutation, and preserve unrelated root references. Hydration coverage
  uses the existing `hydrateState` path.
- `AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.3`: Exactly the three
  Jira marker identities are removed; no other root-state-cast baseline
  identity changes. From the measured design baseline, the total becomes 42.

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/state/slices/jira/jira-slice.test.ts lib/state/store.test.ts lib/state/hydration/hydrator.test.ts)
(cd apps/web && pnpm run typecheck)
python3 -m unittest discover -s scripts/architecture_lint_tests -p 'test_frontend_root_state_cast.py'
python3 scripts/lint-architecture.py --all --baseline-base-ref origin/main
```

Before editing, fetch and record current `origin/main`, compare it with the task
base, and re-measure `config/architecture-lint/frontend_root_state_cast.json`.
Report any drift. The Jira slice must remove only three current Jira identities;
do not force the historical 45/42 totals if the measured base changed.

## Files likely touched

- `apps/web/lib/state/slices/jira/jira-slice.ts`
- `apps/web/lib/state/slices/jira/types.ts` only if truly required
- `apps/web/lib/state/slices/jira/jira-slice.test.ts`
- The Jira creator call only in `apps/web/lib/state/store.ts`
- The exact three Jira entries in
  `config/architecture-lint/frontend_root_state_cast.json`
- `apps/web/lib/state/store.test.ts` and
  `apps/web/lib/state/hydration/hydrator.test.ts` only for narrow composition,
  hydration, or root-reference assertions

## Dependencies

Task 01 completes the roadmap reconciliation and checkpoint documentation.

## Risks

- If the slice's recipe-only setter cannot type-check without broader root
  types or behavior changes, stop and escalate before expanding scope.
- Separate-store isolation must be tested through current store construction;
  do not replace the default object with a factory as part of this refactor.
- The existing slice test harness uses a test-local assertion. Do not turn this
  task into unrelated test-harness cleanup.

## Parallelism

`sequential`

## Inputs

- The new root-state slice typing requirement and design.
- The current Jira slice and its six existing action tests.
- The Features and Azure DevOps plans as pattern evidence only; their ACs do
  not define Jira behavior.

## Results

Typed the Jira issue-watch slice with its Immer recipe setter and passed root
`set` directly to `createJiraSlice`. Existing action signatures and behavior
are unchanged. Added coverage for isolated store mutation, actual
`createAppStore` composition and initial-state merging, unrelated root
references, and preservation through the existing `hydrateState` path.

The TDD compiler red was `cd apps/web && pnpm run typecheck`, which reported
`TS2554: Expected 3 arguments, but got 1` at the direct creator call. After the
minimal setter-type change, the same typecheck passed.

Passed:

- `cd apps/web && pnpm exec vitest run lib/state/slices/jira/jira-slice.test.ts lib/state/store.test.ts lib/state/hydration/hydrator.test.ts` — 55 tests passed across three files.
- `cd apps/web && pnpm run typecheck` — passed after the Jira typing change.
- `python3 -m unittest discover -s scripts/architecture_lint_tests -p 'test_frontend_root_state_cast.py'` — 5 tests passed.
- `python3 scripts/lint-architecture.py --all --baseline-base-ref origin/main` — passed.
- Pinned JSON identity comparison against main `64f8830d0d7b1ea98da96d3373d68bc0386cf49e` — 45 to 42; exactly the three Jira identities removed, no additions or other removals.
- `git diff --check` — passed.

The final combined regression run also covers the Jira loading-action toggle
and reset-preserves-loading assertions added during Task 03: 66 tests passed
across the Jira, Linear, root-store, and hydration suites.
