---
id: "01-preserve-live-commits"
title: "Preserve repository-qualified live commits"
status: done
plan: plan.md
wave: 1
depends_on: []
requirements:
  - REQ-TASKS-MULTI-BRANCH-001
acceptance_criteria:
  - AC-TASKS-MULTI-BRANCH-001.2
system_design:
  - ../../specs/tasks/system-design/multi-branch-live-commits.md
---

# Task 01: Preserve repository-qualified live commits

## Outcome and scope

Implement the [plan](plan.md) through repository-scoped amend selection in
`addSessionCommit`. Retain both repository rows with shared parents and keep
same-repository amend replacement and duplicate notifications correct.

Owned files:

- `apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts`
- `apps/web/lib/state/slices/session-runtime/session-commits-refetch.test.ts`
- `apps/web/e2e/tests/git/multi-repo-live-commits.spec.ts`
- The linked requirement, design, and this delivery package.

## Acceptance

1. Different repository scopes cannot replace each other's commits,
   even with matching parent or commit SHAs.
2. Same-scope amendments and missing/empty root names retain existing behavior.
3. Open desktop and phone Changes views display both real live commits without
   reload or PR association.

## Verification

Run the new store regression and browser scenario before the production change.
Both must fail because a repository commit disappears. After the correction:

```bash
cd apps/web
pnpm exec vitest run lib/state/slices/session-runtime/session-commits-refetch.test.ts lib/ws/handlers/git-status.test.ts
pnpm exec eslint lib/state/slices/session-runtime/session-runtime-slice.ts lib/state/slices/session-runtime/session-commits-refetch.test.ts
NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck
cd ../..
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host e2e/tests/git/multi-repo-live-commits.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

For this host, remove inherited private startup-config variables before E2E and
supply existing Nix browser libraries through `LD_LIBRARY_PATH`.
Set `CAPTURE_PR_ASSETS=true` for the final browser run.
Inspect, compress, and publish the fresh desktop and phone captures with the PR.

## Dependencies, risks, and parallelism

Dependencies: none. Execution is sequential. The user authorized implementation
after the diagnostic plan. No subagents or persistent platform tasks are used.
The compatibility risk and excluded behavior are recorded in the plan.

## Results

- RED: five repository-isolation cases failed, with ten controls passing.
  The real browser test failed because one desktop commit row was missing
  after both `commit_created` events arrived.
- GREEN: all 38 store and handler tests passed. ESLint passed for the source,
  unit tests, and E2E test. Prettier and typecheck passed.
- The default 4 GiB Node heap was insufficient for typecheck. The documented
  8 GiB rerun passed without source changes.
- The rebuilt browser test passed. The final screenshot run reused that build
  with `--no-build` and passed in 9.7 s. Desktop and Pixel 5 each displayed both
  live commits without reload or PR association.
- Fresh desktop and phone screenshots were inspected and compressed. Capture
  closes the desktop terminal to exclude the host username.
- Catalog validation and specification lint passed. The specification linter's
  36 tests and `git diff --check` passed.
- PR review added an interleaved amendment regression: one failure before
  selecting the repository's own leading row, then all 38 focused tests passed.
  Lint, browser verification, and typecheck passed on the revised source.
- The PR documentation evaluator rejected missing `depends_on`, `wave`, and
  `plan` metadata. Adding those fields produced `status: covered` with no errors
  from the same local evaluator.
- The final E2E observer uses `watchWs` to ignore unrelated payloads and arm
  both repository waits before creating commits. ESLint and the browser test
  passed after that test-only change. The criterion describes repository
  isolation without claiming new branch mapping coverage.
