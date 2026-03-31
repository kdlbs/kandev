---
name: tdd
description: Implement changes using Test-Driven Development (Red-Green-Refactor). Use for bug fixes, new features, or any code change that should have test coverage.
---

# TDD

Implement code changes using strict Red-Green-Refactor. Iron law: **no production code without a failing test first.**

Wrote code before a test? Delete it. Start over from a failing test.

## Available skills and subagents

- **`/e2e`** — Use when the change needs a Playwright E2E test instead of a unit test.
- **`/verify`** — Run after completing all TDD cycles to ensure everything passes across the monorepo.

## When to use

- Bug fixes — write a test that reproduces the bug before fixing
- New functions, methods, or utilities
- Refactoring existing logic that lacks tests

**Skip** for: pure UI components (we don't test React components), config files, generated code.

## Determine test scope

- **Go unit** (`apps/backend/`): test file next to source as `*_test.go`. Run:
  ```bash
  cd apps/backend && go test -v -run TestName ./internal/path/to/package/...
  ```
- **TypeScript unit** (`apps/web/lib/`): test file next to source as `*.test.ts`. Run:
  ```bash
  cd apps && pnpm --filter @kandev/web test -- --run path/to/file.test.ts
  ```
- **Web E2E** (`apps/web/e2e/`): delegate to `/e2e` for Playwright tests.

Choose the right level: unit tests for isolated logic, E2E for user-facing flows.

## Steps

### 1. RED — Write a failing test

1. Identify the single behavior to implement or bug to reproduce
2. Write the **smallest test** that asserts the expected behavior — one assertion, clear name
3. Run the test and confirm it **fails with the expected assertion error** (not a compile/import error)
4. If it passes immediately, the test is not testing new behavior — revise it

### 2. GREEN — Minimal code to pass

1. Write the **minimum production code** to make the failing test pass
2. Do not add extra logic, handle other edge cases, or refactor yet
3. Run the test again and confirm it **passes**
4. If it fails, fix the production code (not the test)

### 3. REFACTOR — Clean up

1. Improve production code: extract helpers, rename, simplify — without changing behavior
2. Improve tests: table-driven tests (Go) or `describe`/`it` blocks (TS), remove duplication
3. Run the test after each change to confirm still green

### 4. Repeat

Return to step 1 for the next behavior or edge case. Continue until the feature or fix is complete.

### 5. Final verification

Run `/verify` to ensure all formatters, linters, typechecks, and tests pass across the monorepo.

## Testing anti-patterns

**Don't test mock behavior:**
- If your assertion checks a mock element (`*-mock` test ID, mock return value), you're testing the mock, not the code. Test real behavior or don't mock it.

**Don't add test-only methods to production code:**
- `destroy()`, `reset()`, `_testHelper()` that only tests call — put these in test utilities, not production classes.

**Mock minimally and understand dependencies:**
- Before mocking, ask: what side effects does the real method have? Does the test depend on any of them?
- Mock the slow/external part (network, disk), not the method the test depends on.
- If mock setup is longer than test logic, consider an integration test instead.

**Don't use incomplete mocks:**
- Mock the complete data structure as it exists in reality, not just fields your test uses. Partial mocks hide bugs when downstream code accesses omitted fields.

**Never swallow errors in tests:**
- `try/catch` that silently ignores failures in test helpers or setup — these hide real failures.

## Red flags

- Writing production code before a failing test exists — delete and start over
- Test passes on first run — it tests nothing new, revise the test
- Fixing a test to make it pass instead of fixing the production code
- Large jumps — multiple behaviors implemented between test runs
- Skipping the refactor step
- Mock setup longer than test logic — consider integration test
- Asserting on mock elements instead of real behavior
