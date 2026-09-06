---
name: tdd
description: Implement changes using Test-Driven Development (Red-Green-Refactor). Use for bug fixes, new features, or any code change that should have test coverage.
---

# TDD

## Execution Context

Use this procedure directly in the primary conversation for every code change,
regardless of size. The user may switch that conversation to the lower-cost
implementation model before beginning the Red-Green-Refactor cycle.

Implement code changes using strict Red-Green-Refactor. Iron law: **no production code without a failing test first.**

Wrote code before a test? Delete it. Start over from a failing test.

## Related skills

- **`/e2e`** — Follow this procedure when the current task needs Playwright E2E coverage.
- **`/pr-fixup`** — After the task-defined checks pass and the PR opens, use it
  only for CI or actionable reviewer findings.

## When to use

- Bug fixes — write a test that reproduces the bug before fixing
- New functions, methods, or utilities
- Refactoring existing logic that lacks tests

**Skip** for: pure UI components (we don't test React components), config files, generated code.

For UI rendering bugs, prefer extracting or using a pure helper and testing that helper. Add Playwright only when the behavior is truly visual or integration-level. Avoid adding React component tests just to assert DOM output; that does not match this project's testing convention.

## Determine test scope

- **Go unit** (`apps/backend/`): test file next to source as `*_test.go`. Run:
  ```bash
  cd apps/backend && go test -v -run TestName ./internal/path/to/package/...
  ```
- **TypeScript unit** (`apps/web/lib/`): test file next to source as `*.test.ts`. Run:
  ```bash
  cd apps && pnpm --filter @kandev/web test -- --run path/to/file.test.ts
  ```
- **Web E2E** (`apps/web/e2e/`): follow `/e2e` when the current task needs Playwright tests.

Choose the right level:
- **Unit:** pure logic or isolated service behavior.
- **Integration:** handler/service/repository boundaries, SQLite-backed flows, filesystem behavior, or process boundaries.
- **E2E:** critical user-facing browser flows; keep these focused and use `/e2e`.

Prefer state/output assertions over interaction assertions. Mock only slow, nondeterministic, or external boundaries; use real implementations or fakes when they keep the test deterministic.

When one behavior has separate wheel, keyboard, touch, or pointer handlers,
inventory and test every handler with distinct branching. For input at a hard
boundary where the underlying position cannot change, dispatch the input event
or gesture directly. Cover the eligible direction at the boundary, the wrong
direction or off-boundary no-op, one physical gesture firing at most once, and
listener cleanup when the change adds subscriptions. An E2E case for one input
modality does not cover the other handler paths.

When a work order names an `AC-*` acceptance criterion, keep the mapping visible
in the test name or a nearby `@covers AC-...` comment. Do not copy the complete
requirement into the test.

For failure-path tests, inject the error at the boundary the production code claims to handle and exercise the real downstream call chain; do not short-circuit by mocking the handler under test.
When production code has a defensive nil/fallback branch or sanitizes an internal error into a public error, inject that alternate return at the interface boundary and assert the public result plus absence of internal details or sensitive content.

For parser, canonicalization, or sanitization tests, build fixtures from the
producer's exact serialized shape. Include boundary cases such as optional
whitespace, adjacent records, terminators without a newline, and malformed or
unclosed input; assert that untrusted content is removed or fails closed while
independent following content is preserved. A test-only normalized fixture can
pass while the real wire framing still fails.

### Cross-layer contracts

When adding or renaming a field that crosses backend, WebSocket, and frontend
boundaries, use `rg` to trace every producer, DTO, store upsert or partial
merge, reconnect/readiness handler, and consumer before editing. Add focused
coverage at the affected boundaries, including refresh/reconnect and an update
that omits the field, so a partial payload cannot silently discard an existing
value.
For MCP/API tool registration or mode-gated catalogs, add positive tests for
intended modes and negative tests asserting tool absence in every excluded mode.

When content passes through multiple canonicalizers or a delayed/created-session
path, test each handoff with distinct sentinels and assert exact equality at
persistence and dispatch. Carry the exact value produced at acceptance through
later stages instead of re-deriving it from mutable source data at launch.

### Concurrent and event-driven behavior

Test ordering-sensitive behavior with channels, barriers, or controllable fakes;
do not use sleeps to create a race, except for a bounded, named delay that
models a known poll-loop schedule when synchronization would alter that
relationship. Pause at the ownership boundary, start the
competing operation, then release. Exercise the real delivery path where
practical, and prove the old interleaving fails before the fix. Assert both the
winner state and the untouched replacement state, including relevant buffers,
signals, or queue ownership. Cover stale events acting after a replacement
operation begins, stale-owner handoffs plus same-owner invalidation/no-replay,
cancellation/retry ownership, and at-most-once delivery when they apply. Run
affected Go packages with `-race`.

When a source can deliver either a complete snapshot or a partial event, define
the omission semantics and test both forms. Capture a request-start revision or
epoch before every asynchronous request, inventory all callers, and use
deferred response/event tests to prove ordering. React loading state does not
serialize same-tick callbacks; use an immediate ref or shared in-flight promise
when request identity must be single-flight.

During cancellation or recovery, do not broadly suppress stream frames. Suppress
only allowlisted cancellation acknowledgements with immutable operation identity;
same-identity message, thinking, and tool frames remain authoritative activity.
Add a negative regression for each activity class and a transport-boundary test
when ordering depends on subscriber processing.

When delayed state has a lifecycle owner, pair the boundary tests: disposal
before the threshold must cancel timers and emit nothing later; disposal or
replacement after publication must immediately clear externally observable
state; and stale callbacks must not mutate the replacement.

When setup can finish before hydration or restoration, test both readiness
states. Cover state that is ready at setup and state that becomes ready after
listener registration. Drive the readiness transition explicitly and dispose
each added subscription.

When logic rejects ambiguous candidates, filter invalid candidates before the
cardinality check. Cover zero valid candidates, one valid candidate mixed with
invalid candidates, and multiple valid candidates.

For behavior described as remaining reactive, updating, or responding after
initialization, test both the initial state and a deterministic post-initialization
transition. An initial snapshot test cannot prove that a subscription, watcher,
or reconciliation path remains active; trigger a state, event, or input change
and assert the updated outcome. If no public API exposes publication or render
counts, a controlled boundary fake may instrument those events, but retain an
observable state assertion as the proof of behavior.

## Steps

### 1. RED — Write a failing test

1. Identify the single behavior to implement or bug to reproduce
2. Write the **smallest test** that asserts the expected behavior — one assertion, clear name
3. Run the test and confirm it **fails with the expected assertion error** (not a compile/import error)
   For a brand-new Go package, create the package directory and minimal test package first, then run the focused package test so RED fails on behavior rather than package-selection or import errors.
4. If it passes immediately, the test is not testing new behavior — revise it.
   Exception: a reviewer-requested test that documents behavior already present
   on the current head is valid test-only contract coverage. Label it as such,
   make no production change, and run the focused suite.

For bug fixes, use the Prove-It Pattern: reproduce the bug with a failing test before changing production code. A fix without a regression test is not complete unless the change is explicitly untestable and you say why.

### 2. GREEN — Minimal code to pass

1. Write the **minimum production code** to make the failing test pass
2. Do not add extra logic, handle other edge cases, or refactor yet
3. Run the test again and confirm it **passes**
4. If it fails, fix the production code (not the test)

### 3. REFACTOR — Clean up

1. Improve production code: extract helpers, rename, simplify — without changing behavior
2. Improve tests: table-driven tests (Go) or `describe`/`it` blocks (TS), remove duplication
3. Run the test after each change to confirm still green

In tests, prefer DAMP over DRY: each test should read like a small specification. Shared helpers are fine when they remove noise, but not when they hide the scenario.

### 4. Repeat

Return to step 1 for the next behavior or edge case. Continue until the feature or fix is complete.

### 5. Final verification

Run the targeted tests named in the task file and report their results. Commit
and open the PR after all affected task checks pass; do not add broad local
verification by default.

After the final production-code edit, rerun every new or changed regression
test and report the exact command and result. A prior green run does not cover
a later patch.

## Testing anti-patterns

**Don't test implementation details:**
- Assert behavior, state, API response, DB row, emitted event, or UI outcome. Avoid assertions that only prove a helper was called or an internal query string happened to be built a certain way.
- Custom hooks and async controller helpers are behavior-bearing logic, not pure React markup: add focused tests for success, failure, cancellation/no-op, and busy/loading-state cleanup.
- When a change adds a bulk or action entry point, test that entry point directly,
  including its empty, partial, and success paths; coverage of a shared helper or
  neighboring single-item flow does not prove the new dispatch path works.

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
- When adding a named export to a shared module, search for full-module
  `vi.mock()` factories and add the export to each factory. Focused tests can
  pass while a full suite fails on an out-of-date module shape.

**Never swallow errors in tests:**
- `try/catch` that silently ignores failures in test helpers or setup — these hide real failures.

**Don't repeat unchanged passing commands for reassurance:**
- After a clean targeted run, re-run only after code or test inputs change. Move to the next required verification step instead.

## Red flags

- Writing production code before a failing test exists — delete and start over
- Test passes on first run — revise it, except for clearly labelled
  reviewer-requested test-only contract coverage
- Fixing a test to make it pass instead of fixing the production code
- Large jumps — multiple behaviors implemented between test runs
- Skipping the refactor step
- Mock setup longer than test logic — consider integration test
- Asserting on mock elements instead of real behavior
- "All tests pass" but no relevant test was actually run
