---
id: "03-type-linear-slice"
title: "Type the Linear issue-watch slice"
status: done
wave: 3
depends_on:
  - 02-type-jira-slice
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

# Task 03: Type the Linear issue-watch slice

## Summary

After Jira completes in this workspace, type Linear's issue-watch slice for its
recipe-only Immer setter and compose it directly into the root store. Preserve
its six actions, defaults, hydration, and unrelated root-state references.

## In scope

- Narrow `createLinearSlice` to the setter capability its current recipes use.
- Pass root `set` directly at the Linear creator call in `store.ts`.
- Add regression coverage for isolated defaults, actual root composition,
  the existing initial-state merge and `hydrateState` path, mutation isolation
  between independent stores, and unchanged unrelated root-state references.
- Remove only the three Linear root-state-cast baseline entries.
- Re-run the combined Jira and Linear regression suites and final required
  checks from the plan.

## Out of scope

- Provider APIs, credentials, issue-watch behavior, cache ownership, or UI.
- Changes to Linear action signatures/order, loading/reset semantics, or
  hydration shape; new Linear-specific boot or hydration routes.
- Generic root-state types, replacement-state overloads, a shared slice helper,
  test-only cast cleanup, or any non-Linear baseline entry.

## Acceptance

- `AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.1`: TypeScript accepts
  root composition with the Linear recipe setter and no `as any`/`as unknown
  as` escape at the Linear creator call.
- `AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.2`: Existing defaults and
  all six action semantics remain covered; root-store assertions exercise the
  existing initial-state merge, prove separate stores remain isolated after a
  Linear mutation, and preserve unrelated root references. Hydration coverage
  uses the existing `hydrateState` path.
- `AC-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001.3`: Exactly the three
  Linear marker identities are removed; no other root-state-cast baseline
  identity changes. From the measured design baseline, the total becomes 39.

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/state/slices/linear/linear-slice.test.ts lib/state/store.test.ts lib/state/hydration/hydrator.test.ts)
(cd apps/web && pnpm exec vitest run lib/state/slices/jira/jira-slice.test.ts lib/state/slices/linear/linear-slice.test.ts lib/state/store.test.ts lib/state/hydration/hydrator.test.ts)
(cd apps/web && pnpm run typecheck)
python3 -m unittest discover -s scripts/architecture_lint_tests -p 'test_frontend_root_state_cast.py'
python3 scripts/lint-architecture.py --all --baseline-base-ref origin/main
```

Re-measure the root-state-cast baseline at entry. Remove only three current
Linear identities. The expected total is 39 from the design snapshot; report
main drift rather than deleting unrelated entries or forcing that count.

## Files likely touched

- `apps/web/lib/state/slices/linear/linear-slice.ts`
- `apps/web/lib/state/slices/linear/types.ts` only if truly required
- `apps/web/lib/state/slices/linear/linear-slice.test.ts`
- The Linear creator call only in `apps/web/lib/state/store.ts`
- The exact three Linear entries in
  `config/architecture-lint/frontend_root_state_cast.json`
- `apps/web/lib/state/store.test.ts` and
  `apps/web/lib/state/hydration/hydrator.test.ts` only for narrow composition,
  hydration, or root-reference assertions

## Dependencies

Task 02 must be complete in this workspace before Linear begins.

## Risks

- If the recipe-only setter needs broader root types or behavior changes to
  compile, stop and escalate before expanding scope.
- Separate-store isolation must be tested through current store construction;
  do not replace the default object with a factory as part of this refactor.
- Preserve Task 02's changes and any unrelated workspace edits.

## Parallelism

`sequential`

## Inputs

- The new root-state slice typing requirement and design.
- The current Linear slice and its six existing action tests.
- Jira's completed implementation in this workspace for the adjacent pattern.

## Results

Typed the Linear issue-watch slice with its Immer recipe setter and passed root
`set` directly to `createLinearSlice`. Existing action signatures and
behavior are unchanged. Added matching coverage for isolated store mutation,
actual `createAppStore` composition and initial-state merging, unrelated root
references, and preservation through the existing `hydrateState` path.

The TDD compiler red was `cd apps/web && pnpm run typecheck`, which reported
`TS2554: Expected 3 arguments, but got 1` at the direct creator call. After the
minimal setter-type change, the same typecheck passed.

Final combined verification passed:

- `cd apps/web && pnpm exec vitest run lib/state/slices/jira/jira-slice.test.ts lib/state/slices/linear/linear-slice.test.ts lib/state/store.test.ts lib/state/hydration/hydrator.test.ts` — 66 tests passed across four files.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm run lint` — passed with `--max-warnings 0`.
- `python3 -m unittest discover -s scripts/architecture_lint_tests -p 'test_frontend_root_state_cast.py'` — 5 tests passed.
- `python3 scripts/lint-architecture.py --all --baseline-base-ref origin/main` — passed.
- Pinned JSON identity comparison against main `64f8830d0d7b1ea98da96d3373d68bc0386cf49e` — 45 to 39; exactly three Jira and three Linear identities removed, no additions or other removals.
- After fast-forwarding the unpublished branch to current main `3fed5570cec533f468c25ed03c84967bdc972588`, the same pinned identity comparison remained 45 to 39; no source or baseline paths changed in the intervening main delta.
- `python3 scripts/list-docs.py validate` — 368 decisions and 1,498 specifications validated on the current-main branch.
- `python3 scripts/lint-spec-files.py --all` — all specification files passed.
- `git diff --check` — passed.
- Trusted-main pure PR-docs `validateCoverage` using `.github/scripts/pr-docs.cjs` from `3fed5570cec533f468c25ed03c84967bdc972588` and all 19 actual changed paths/content — `ok: true`, status `covered`, no errors; all three changed work orders were accepted.
