---
created: 2026-08-28
status: implemented
requirements: []
system_design: []
legacy_specs: []
---

# Implementation Plan: Monaco Vitest Isolation

## Overview

Frontend unit tests must not evaluate Monaco's editor contribution graph.
Vitest will alias the bare package import to a small test stub. Production and
browser tests will continue to use the real package.

The full suite on PR #3112 reproduced issue #3114. All 1,657 test files passed,
but one duplicate Monaco command registration made Vitest exit with code 1. The
same suite passed with the stub prototype.

This internal harness change has no product requirement or system design. It
follows [ADR 0037](../../decisions/0037-resource-aware-frontend-unit-tests.md).

## Scope

### In scope

- Alias only the bare `monaco-editor` specifier in Vitest.
- Supply the Monaco runtime values that current unit tests use.
- Add a focused regression test for the test-only contract.
- Keep real Monaco behavior in production builds and browser tests.

### Out of scope

- Changes to Monaco initialization in production code.
- Changes to Monaco workers or language providers.
- Disabling Vitest per-file isolation.
- Changes to PR #3112.

## Technical approach

Update `apps/web/vitest.config.ts` with an exact regular-expression alias. The
alias must not match worker paths or other Monaco subpaths.

Add `apps/web/vitest.monaco-editor.ts`. The stub will export `Uri`, editor
registration methods, language registration methods, TypeScript defaults, and
the enum values that `monaco-init.ts` uses.

Add `apps/web/vitest-monaco-editor.test.ts`. The test will check a stub marker,
URI round trips, and the TypeScript defaults. The marker makes the test fail
when the alias is absent.

## Tests

- The focused stub test must fail before the alias exists.
- The Monaco initialization, URI, plugin, and task-listing tests must pass with
  the stub.
- The full frontend unit suite must pass without an unhandled Monaco rejection.

## Work orders

- [x] [Task 01: Isolate Monaco from Vitest](task-01-isolate-monaco-from-vitest.md)

## Verification results

- The regression test failed before the alias because the stub marker was absent.
- The focused compatibility set passed 32 files and 249 tests.
- ESLint and TypeScript passed.
- The full frontend unit suite passed 1,652 files. It passed 14,182 tests,
  skipped 4 tests, and exited with code 0.
- The full suite did not report a duplicate Monaco command registration.

## Risks

- The stub can drift when production code imports another Monaco runtime value.
- An alias without an exact match can intercept worker and deep API imports.
- Unit tests cannot prove the real Monaco browser integration. Existing browser
  tests and production builds retain that responsibility.
