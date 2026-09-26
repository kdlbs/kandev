---
id: "01-isolate-monaco-from-vitest"
title: "Isolate Monaco from Vitest"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 01: Isolate Monaco from Vitest

## Summary

Replace the real Monaco package with a narrow stub during unit tests. Keep the
real package unchanged for production builds and browser tests.

## In scope

- Add the exact bare-package alias to `vitest.config.ts`.
- Add the minimal Monaco unit-test stub.
- Add a regression test for the stub contract.
- Record the targeted and full-suite results.

## Out of scope

- Production Monaco initialization changes.
- Monaco dependency upgrades.
- Vitest pool, worker, or isolation changes.
- New browser tests.

## Acceptance

- The regression test fails when Vitest resolves the real package.
- The stub supplies URI behavior and the language defaults that current tests use.
- Deep Monaco paths remain available to Vite and production code.
- The full PR #3112 frontend suite exits with code 0 and has no Monaco rejection.

## Verification

```bash
pnpm exec vitest run vitest-monaco-editor.test.ts
pnpm exec vitest run vitest-monaco-editor.test.ts components/editors/monaco/monaco-init.test.ts lib/lsp/file-uri.test.ts lib/plugins lib/task-listing
pnpm exec eslint --max-warnings 0 vitest.config.ts vitest.monaco-editor.ts vitest-monaco-editor.test.ts
pnpm run typecheck
pnpm test
```

Run these commands from `apps/web`. In a fresh worktree, first run
`pnpm install --frozen-lockfile` from `apps`.

## Files likely touched

- `apps/web/vitest.config.ts`
- `apps/web/vitest.monaco-editor.ts`
- `apps/web/vitest-monaco-editor.test.ts`

## Dependencies

None.

## Risks

- A missing stub export can hide a new unit-test dependency until the focused
  contract test changes.
- A broad alias can replace worker modules and break production-like tests.

## Parallelism

`sequential`

## Inputs

- [ADR 0037](../../decisions/0037-resource-aware-frontend-unit-tests.md)
- GitHub issue #3114.
- The full-suite failure on PR #3112.
- `apps/web/components/editors/monaco/monaco-loader.ts`.
- `apps/web/components/editors/monaco/monaco-init.ts`.
- `apps/web/lib/lsp/file-uri.test.ts`.

## Results

- RED: `pnpm exec vitest run vitest-monaco-editor.test.ts` failed because
  `__KANDEV_VITEST_STUB__` was absent from the real package.
- GREEN: The same command passed one test after the exact alias and stub were added.
- The focused compatibility command passed 32 files and 249 tests.
- The focused ESLint command exited with code 0.
- `pnpm run typecheck` exited with code 0.
- `pnpm test` passed 1,652 files. It passed 14,182 tests, skipped 4 tests,
  and exited with code 0. It did not report a duplicate Monaco command registration.
- The investigation prototype also passed the PR #3112 suite. That run passed
  1,657 files and 14,258 tests with the same stub design.
