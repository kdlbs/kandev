---
id: "01-materialization-client"
title: "Correct the materialization request budget"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REPOSITORY-BRANCH-002
acceptance_criteria:
  - AC-EXECUTORS-REPOSITORY-BRANCH-002.6
  - AC-EXECUTORS-REPOSITORY-BRANCH-002.7
  - AC-EXECUTORS-REPOSITORY-BRANCH-002.8
system_design:
  - ../../specs/executors/system-design/repository-branch-resolution.md
---

# Task 01: Correct the materialization request budget

## Summary

Send repository materialization through the existing long-running HTTP client.
Add deterministic regression tests and preserve caller cancellation, response handling, and transport identity.

## In scope

- The `MaterializeRepository` request sender and its timing tests.
- The existing materialization test fixture that initializes only the short client.
- Results and status updates in this work order and its plan.

## Out of scope

- Clone flags, credential scope, server limits, retries, new timeout settings, and cleanup changes.
- Frontend changes, live-instance mutation, deployment, commit, push, and PR creation.

## Acceptance

1. The slow-success regression fails on current production code and passes after the request uses `longRunningHTTPClient`.
2. Caller cancellation and earlier deadlines remain observable, and the five-minute request limit remains finite.
3. The affected fixture initializes both clients, and existing payload, authentication, typed-error, projection, and rollback checks pass.

## TDD sequence

1. Mark this work order `in_progress` after an explicit implementation request.
2. Add `TestMaterializeRepository_UsesLongRunningClient` in `client_workspace_sources_timeout_test.go`.
3. Use `testing/synctest` with a context-aware in-memory `http.RoundTripper` that returns success after 61 simulated seconds.
4. Construct the client through `NewClient` or `newClient` with production timeout defaults.
5. Install the test transport on both clients before constructor transport wrappers, using a test-local `ClientOption`.
6. Run the RED command below. Expect the materialization timeout at 60 simulated seconds.
7. Change `MaterializeRepository` from `c.httpClient.Do(req)` to `c.longRunningHTTPClient.Do(req)`.
8. Update `TestMaterializeRepository_ReturnsTypedRemoteError` to use `newClient(server.URL, log, WithAuthToken("test-token"))`.
9. Preserve its payload, qualified-base identity, authentication, and typed-error assertions.
10. Add `TestMaterializeRepository_HonorsCallerContext` with earlier-deadline and cancellation-after-entry subtests.
11. Assert `errors.Is` against `context.DeadlineExceeded` and `context.Canceled` through the existing error wrapper.
12. Add `TestMaterializeRepository_LongRunningTimeout` with a response delayed beyond five simulated minutes.
13. Assert the finite timeout and its error classification. Join all test goroutines and close response bodies.
14. Run the final checks below and record their actual results.

The transport must honor `req.Context().Done()` and return a valid materialization response on success.
Use channels for request-entry synchronization and fake timers for simulated latency.
Do not wait 60 real seconds, clone a large repository, or add a production timeout option for tests.
Leave `RemoveMaterializedRepository` on its current client.

## Verification

Run each block from the repository root.
The RED command must fail for the expected timeout before the correction:

```bash
(cd apps/backend && go test -trimpath ./internal/agent/runtime/agentctl -run '^TestMaterializeRepository_UsesLongRunningClient$' -count=1 -v)
```

After the correction, run these commands sequentially:

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/agentctl -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run '^(TestRemoteWorkspaceProjectionFromLaunch_.*|TestMaterializeWorkspaceRepositories_.*|TestMaterializeRepositoriesForEnvironment_.*)$' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/agentctl/client_workspace_sources.go apps/backend/internal/agent/runtime/agentctl/client_workspace_sources_test.go apps/backend/internal/agent/runtime/agentctl/client_workspace_sources_timeout_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/repository-materialization-timeout
```

Run this documentation preflight against the planned production paths and the current uncommitted documents:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const documents = [
  'docs/plans/repository-materialization-timeout/plan.md',
  'docs/plans/repository-materialization-timeout/task-01-materialization-client.md',
  'docs/specs/executors/requirements/repository-branch-resolution.md',
  'docs/specs/executors/system-design/repository-branch-resolution.md',
];
const production = [
  'apps/backend/internal/agent/runtime/agentctl/client_workspace_sources.go',
  'apps/backend/internal/agent/runtime/agentctl/client_workspace_sources_test.go',
  'apps/backend/internal/agent/runtime/agentctl/client_workspace_sources_timeout_test.go',
];
const result = validateCoverage({
  changedFiles: [...documents, ...production].map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(documents.map(filename => [filename, fs.readFileSync(filename, 'utf8')])),
});
console.log(JSON.stringify({ status: result.status, workOrders: result.workOrders, errors: result.errors }, null, 2));
if (!result.ok || result.status !== 'covered') process.exit(1);
NODE
```

Record that result with the commands above.
Do not mark the work order `done` before all implementation checks pass.

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/client_workspace_sources.go`
- `apps/backend/internal/agent/runtime/agentctl/client_workspace_sources_test.go`
- `apps/backend/internal/agent/runtime/agentctl/client_workspace_sources_timeout_test.go` (new)
- `docs/plans/repository-materialization-timeout/plan.md`
- `docs/plans/repository-materialization-timeout/task-01-materialization-client.md`

## Dependencies

None. This is the only work order.

## Risks

The HTTP budget covers all materialization work, while server Git budgets apply per command after admission.
The test fixture must use production constructor wiring to avoid hiding a missing transport or client.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/executors/requirements/repository-branch-resolution.md), criteria 002.6 through 002.8.
- [Design](../../specs/executors/system-design/repository-branch-resolution.md#remote-materialization-request-budget).
- [Plan evidence](plan.md#evidence-and-root-cause) and the removed temporary reproduction's observed results.
- `client_preparation_budget_test.go`: existing long-operation and caller-deadline pattern.
- `client_core_test.go`: constructor defaults and transport identity checks.
- `workspace_materialization_test.go` in lifecycle: primary/additional projection, rescan, and rollback behavior.

## Results

Completed on 2026-11-14.

- RED: `go test -trimpath ./internal/agent/runtime/agentctl -run '^TestMaterializeRepository_UsesLongRunningClient$' -count=1 -v` failed as expected at the 60-second client timeout before the production change.
- GREEN: focused materialization tests passed for 61-second success, caller deadline, cancellation after request entry, five-minute timeout, and the existing typed remote error/authentication contract.
- `go test -trimpath -race ./internal/agent/runtime/agentctl -count=1`: passed.
- `go test -trimpath -race ./internal/agent/runtime/lifecycle -run '^(TestRemoteWorkspaceProjectionFromLaunch_.*|TestMaterializeWorkspaceRepositories_.*|TestMaterializeRepositoriesForEnvironment_.*)$' -count=1`: passed.
- `go build -trimpath ./internal/agent/runtime/agentctl`: passed.
- `bash scripts/lint-go-changed apps/backend/internal/agent/runtime/agentctl/client_workspace_sources.go apps/backend/internal/agent/runtime/agentctl/client_workspace_sources_test.go apps/backend/internal/agent/runtime/agentctl/client_workspace_sources_timeout_test.go`: passed with 0 issues.
- `python3 scripts/list-docs.py validate`: passed; 450 decisions and 1999 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: passed; 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Documentation coverage preflight: `covered`, one work order, no errors.
- Public docs review found no published 60-second repository-materialization promise or conflicting limit. No public-doc edit was needed. `node --test scripts/validate-public-docs.test.mjs` passed (63 tests), and `node scripts/validate-public-docs.mjs` validated 55 pages.
- `git diff --check`: passed.

`MaterializeRepository` now uses the existing five-minute client. The typed-error fixture uses the production constructor and authentication wiring. `RemoveMaterializedRepository` remains on the ordinary client.
