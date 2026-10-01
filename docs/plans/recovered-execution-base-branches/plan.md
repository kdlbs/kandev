---
created: 2026-09-30
status: implemented
requirements:
  - REQ-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001
system_design:
  - ../../specs/workspaces/system-design/workspace-base-branch-propagation.md
legacy_specs: []
---

# Implementation Plan: Recovered Execution Base Branches

## Overview

Repair [issue #3806](https://github.com/kdlbs/kandev/issues/3806) by delivering
recorded bases before initial tracker scans and delaying lifecycle readiness
until configuration pushes finish. One sequential work order owns both sides of
the same recovery boundary and its Git API regression evidence.

## Confirmed root cause and evidence

`prepareExecutionCreateRequest` copies workspace metadata but never hydrates
missing base branches. `waitForAgentctlReady` calls `MarkAgentctlReady` before
the branch and comparison-target pushes. Trackers can therefore compute against
integration fallback before the configured base arrives.

A temporary probe drives `prepareExecutionCreateRequest` with a three-repository
provider returning `origin/develop` and absent metadata. It observes zero
provider calls and an empty outgoing map. The probe is removed after execution;
permanent coverage belongs to the implementation work order.

The [previous completed package](../workspace-base-branch-propagation/plan.md)
covered eventual propagation and explicitly allowed the first fallback result.
Its done status and historical test results remain accurate for that scope.
This package extends that result to initial comparisons, rather than reopening
or replacing those historical work orders.

## Scope

### In scope

- Hydrate missing creation metadata synchronously through the existing provider.
- Finish both configuration pushes before readiness, polling, or ready events.
- Prove recovery range results with real Git fixtures and API calls.

### Out of scope

- New seeded-state API gate or changed error-path fallback policy.
- Default-branch candidate changes, commit truncation UI, or unrelated Git fixes.
- Frontend markup, SDK/API fields, runtime flags, schema changes, publication.

## Technical approach

Use `getMetadataStringMap` and `MetadataKeyBaseBranches`, not a new field on
`ExecutorCreateRequest`. Put a narrow hydration helper in
`manager_base_branches.go` if needed to keep the large preparation function
within repository complexity limits. Preserve metadata/provider ownership by
copying the accepted map. Reorder the existing readiness pushes in
`manager_startup.go` while preserving client lease and cancellation semantics.

| Executor/transport | Existing map path | Evidence | Unavailable-data behavior |
| --- | --- | --- | --- |
| Standalone/worktree HTTP | Standalone create builder | Existing standalone tests plus new recovery probe | Warn; existing fallback |
| Docker/remote Docker HTTP | Shared reconnect builder | Metadata and builder tests | Same |
| SSH HTTP over tunnel | SSH create builder | Existing SSH forwarding tests | Same |
| Sprites HTTP proxy | Sprites create builder | Existing remote forwarding tests | Same |
| Kubernetes loopback forwarding | Shared reconnect builder | Existing task-pod tests | Same |
| Plugin-owned transport | Existing plugin capability | Provider-neutral metadata and readiness tests only | No SDK expansion; no new transport guarantee |

## Tests

- Creation regression in `recovered_execution_base_branches_test.go`: metadata
  reuse, missing-map provider hydration, single/multi-repo keys, JSON decoding,
  defensive copies, empty/malformed input, and nonfatal provider errors.
- Readiness regression in the same file: block each push and inspect readiness,
  event publication, and polling order; exercise provider and HTTP errors.
- Git API integration in `git_recovered_base_branches_test.go`: three real repos
  with `origin/develop` ahead of `origin/main`, initially zero task commits,
  then a task commit; mixed configured/unconfigured repositories resolve
  independently. Cover log, status, and cumulative diff on the first request.

These map to criteria .1-.3 and .5-.10. Existing fallback-observability tests
cover .4. Do not add cases to the already oversized `manager_execution_test.go`.

## E2E evidence

Use real Git repositories through agentctl HTTP endpoints, including the first
response after seeded creation. This backend boundary is sufficient for the
unchanged desktop and phone commits presentation; no Playwright file changes.
Pair it with a lifecycle `CreateInstance` capture to prove recovered metadata
actually reaches the executor boundary. Neither test alone proves both halves.

## Work orders

- [x] [Task 01: Seed recovered comparisons before readiness](task-01-seed-recovered-comparisons.md)

## Verification results

Design checkpoint, 2026-09-30:

- Temporary `go test ./internal/agent/runtime/lifecycle -run
  '^TestIssue3806TemporaryRecoveryProbe$' -count=1 -v`: PASS as a diagnostic
  probe; observed `provider calls=0; create-request base branches=map[]`.
  The temporary source was removed. This confirms the defect, not its repair.
- `python3 scripts/list-docs.py validate`: PASS, 336 decisions and 1270 specs.
- `python3 scripts/lint-spec-files.py --all`: PASS.
- `python3 scripts/lint-spec-files.test.py`: PASS, 36 tests.
- Prospective runtime-change preflight through
  `.github/scripts/pr-docs.cjs.validateCoverage`: PASS (`covered`, no errors),
  with this work order and the linked plan/requirement/design. The prospective
  runtime path was `manager_execution.go`; runtime code is unchanged.
- `git diff --check -- docs/specs docs/plans/recovered-execution-base-branches`:
  PASS. `git status --short` confirms the changed requirement plus the new
  design and plan directory are unstaged; no production or permanent test edits.

Implementation checkpoint, 2026-09-30:

- Red: creation hydration/forwarding and readiness-order regressions failed on
  missing base maps, premature readiness, and premature cached poll-mode flush.
- Green: focused lifecycle regressions passed, including provider errors,
  HTTP push errors, health cancellation, map ownership, and lease release.
- `(cd apps/backend && go test ./internal/agentctl/server/api -run
  '^TestRecoveredBaseBranches_FirstGitResponses$' -count=1)`: PASS (1.633s),
  proving initial zero-commit results, one added task commit, status/diff anchors,
  and mixed configured/fallback repository isolation with real Git fixtures.
- The work order's combined `go test -race` command passed API (33.979s),
  process (134.221s), process/probe (1.417s), and lifecycle/skill (1.207s).
  Lifecycle failed one unrelated resolver test because the inherited
  `KANDEV_BUNDLE_DIR=/home/zeval/repositories/kandev/apps/backend` contains a
  Darwin helper, while that test expects none. The same test fails with all
  three modified production files replaced by HEAD using a temporary Go overlay.
- `(cd apps/backend && env -u KANDEV_BUNDLE_DIR go test -race
  ./internal/agent/runtime/lifecycle/... -count=1 -json)`: PASS; lifecycle
  92.168s, lifecycle/skill 1.221s, 3,533 passing test events including subtests,
  no failures. JSON evidence: `/tmp/issue3806-lifecycle-race-0wp8v8eo.jsonl`.
  Only this test process's environment was changed.
- Documentation catalog/spec lint, actual changed-file documentation coverage,
  and diff checks pass. Product code and permanent tests are now included.
- Internal docs updated: requirement and design now describe initial-result
  seeding; the completed original plan retains its historical scope/results.
  No public docs change needed: `docs/public/git-operations.md` already states
  that Changes history contains task-branch commits relative to the comparison
  base, and this repair restores that behavior.

Task 01 is done. No delegation was used. Publication follows the user's
separate authorization.

Publication preflight: Go lint caught two unchecked execution-store registration
errors in the new test setup. Both now fail the test explicitly on registration
error. `(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle
-run 'TestWaitForAgentctlReady_(SeedsBeforeReady|HealthFailureDoesNotSeed)$'
-count=1)` passed (2.183s) after that test-only correction.

## Risks

- Provider failure retains best-effort fallback; mandatory gating needs a
  separate failure-policy decision.
- Persisted non-empty metadata can be stale; the current provider push refreshes
  it before lifecycle readiness, as today. This repair targets missing maps.
- Readiness includes existing push latency; preserve its bounded context and
  avoid locking across callbacks or leaking client leases.
- Explicit comparison targets must retain precedence over branch-only refs.

## Review remediation

CodeRabbit suggested reducing the readiness-ordering test size. Extracted its
blocking HTTP fixture into `blockingComparisonSeedHandler`, preserving both
configuration barriers, cached poll-mode checks, and cancellation cleanup.
The fixture extraction leaves production behavior and its contracts unchanged.

Focused verification: `(cd apps/backend && go test -race
./internal/agent/runtime/lifecycle -run
'^TestWaitForAgentctlReady_SeedsBeforeReady$' -count=1)` PASS before and after
the extraction (1.805s and 1.865s). Remote CI/review evidence is tracked in the external task plan.

Claude suggested clarifying the readiness-time provider refresh. Updated
`pushTaskBaseBranches` documentation to describe its correction of DB edits
since creation and its coverage of already-running `Manager.Start` recovery.
The two intentional provider reads and their best-effort behavior are unchanged.

Greptile identified unbounded creation-time hydration before the launch deadline.
Added a five-second child context for the DB-backed lookup, preserving shorter
caller deadlines/cancellation and best-effort request preparation. Requirement
criterion .11, design, and work-order acceptance now record this boundary.
The virtual-time regression failed before the fix: the slow lookup consumed
one minute instead of five seconds; shorter and cancelled caller cases passed.

Post-fix verification: `(cd apps/backend && go test -race
./internal/agent/runtime/lifecycle -run
'^(TestPrepareExecutionCreateRequest_(BaseBranchLookupDeadline|HydratesBaseBranches)|TestGetOrEnsureExecution_SeedsBaseBranchesAtCreation|TestWaitForAgentctlReady_(SeedsBeforeReady|SeedingFailuresNonFatal|HealthFailureDoesNotSeed)|TestPushTaskBaseBranches)$'
-count=1)` PASS (3.945s). Full changed-scope backend lint PASS with
`golangci-lint run ./... --new-from-rev=<PR-base-sha> --timeout=10m` (zero issues).
The local five-minute attempts timed out during analysis/package loading; the
completed ten-minute run used the same linters and changed-code scope.
Catalog/spec lint and diff checks PASS.
