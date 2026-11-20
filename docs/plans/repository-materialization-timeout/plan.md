---
created: 2026-11-13
status: done
requirements:
  - REQ-EXECUTORS-REPOSITORY-BRANCH-002
system_design:
  - ../../specs/executors/system-design/repository-branch-resolution.md
legacy_specs: []
---

# Repository materialization timeout

## Overview

Allow remote repository materialization to use the existing five-minute HTTP budget.
One sequential work order adds regression coverage and corrects the client selection.
Implementation completed after the explicit request on 2026-11-14.

Issue: [kdlbs/kandev#5301](https://github.com/kdlbs/kandev/issues/5301).
GitHub confirms assignment to `carlosflorencio` on 2026-11-13.

## Evidence and root cause

`Client.MaterializeRepository` calls `c.httpClient.Do(req)` in `client_workspace_sources.go`.
`newClient` gives that client a 60-second total timeout and gives `longRunningHTTPClient` five minutes.
The server permits a clone command to run for five minutes after admission.
The short HTTP deadline can therefore cancel a valid clone before the server budget expires.

`reconcileLaunchWorkspace` reaches this method for plugin primary repositories and additional repositories on clone-based executors.
The same method also serves live remote source attachment.
The issue has one comment and no image attachments.
The comment confirms failures for non-first repositories and separately requests partial clones and broader credential scope.

A temporary `TestReproIssue5301_MaterializationBudget` used the real constructor and method with a context-aware transport.
`testing/synctest` simulated a response after 61 seconds without a network connection or Git clone.
The current-client case failed at exactly 60 simulated seconds with `Client.Timeout exceeded while awaiting headers`.
The control replaced the test client's short-client reference with its existing long-running client.
That control succeeded at 61 simulated seconds. No production code changed.

Reproduction command, run before removing the temporary test:

```bash
(cd apps/backend && go test -trimpath ./internal/agent/runtime/agentctl -run '^TestReproIssue5301_MaterializationBudget$' -count=1 -v)
```

Exit status was 1: the current-client case failed and the long-client control passed.
The final package removes the temporary test. The work order specifies permanent regression coverage to author during implementation.

## Requirement and design reconciliation

The existing executor requirement owns preparation before remote agent startup.
Its previous criteria cover checkout identity but do not state the materialization request budget.
This package adds AC-EXECUTORS-REPOSITORY-BRANCH-002.6, .7, and .8 to the [owning requirement](../../specs/executors/requirements/repository-branch-resolution.md).
The [owning design](../../specs/executors/system-design/repository-branch-resolution.md#remote-materialization-request-budget) defines the shared client boundary.
No new ownership boundary, public setting, schema, or ADR is required.

Related packages `remote-clone-branch`, `remote-pr-review-checkout`, and `executor-literal-placeholder-expansion` already have completed work orders.
Their script and checkout scopes remain separate. Their recorded verification results require no change.

## Scope

In scope: client selection, the affected test fixture, and deterministic tests for success, cancellation, and finite timeout.
Partial-clone configuration, credential leases, server Git limits, retries, cleanup behavior, and script-based primary preparation are outside this repair.
No rendered UI or public API shape changes.

## Technical approach

Change only the request sender in production `apps/backend/internal/agent/runtime/agentctl/client_workspace_sources.go`.
Keep the existing request context, payload, response handling, and error wrapping.
Update `TestMaterializeRepository_ReturnsTypedRemoteError` to construct a complete client with authentication.
Its current literal initializes only `httpClient`. The corrected method otherwise encounters a nil long-running client in this test.
Add `client_workspace_sources_timeout_test.go` for the new timing cases.

| Consumer | Transport and identity | Intended coverage and limit |
| --- | --- | --- |
| Plugin remote, primary and additional repositories | Provider connection lease plus existing auth and instance identity | Shared five-minute client. Source trace and client tests establish coverage. No live provider claim. |
| Built-in clone-based executors, additional repositories | Existing agentctl connection or tunnel plus existing identity guards | Same method and budget. Primary prepare scripts retain their separate path. |
| Live remote source attachment | Existing execution client and task authorization | Same method. Existing rollback and rescan behavior remains applicable. |

Unsupported provider connections retain their existing rejection path. There is no fallback to the short client.

## Tests and end-to-end evidence

| Criterion | Planned evidence |
| --- | --- |
| AC-EXECUTORS-REPOSITORY-BRANCH-002.6 | New `TestMaterializeRepository_UsesLongRunningClient` succeeds after 61 simulated seconds and fails before the production correction. |
| AC-EXECUTORS-REPOSITORY-BRANCH-002.7 | New `TestMaterializeRepository_HonorsCallerContext` covers cancellation after request entry and an earlier deadline. |
| AC-EXECUTORS-REPOSITORY-BRANCH-002.8 | New `TestMaterializeRepository_LongRunningTimeout` observes the five-minute limit and a timeout error. |
| Existing wire and checkout contract | Existing typed-error/auth test, full client package, and focused lifecycle projection/rollback tests. |

The transport reproduction proves the timeout defect at its owning boundary.
Existing loopback HTTP tests and lifecycle batch tests provide integration evidence without external accounts.
No browser test is required for this client-only correction.
Actual large-repository startup remains an optional environment-specific check, not a claimed result.

## Work orders

- [x] [Task 01: Correct the materialization request budget](task-01-materialization-client.md)

## Verification results

Design checks completed on 2026-11-13:

- `python3 scripts/list-docs.py validate`: passed, with 450 decisions and 1999 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- The work order's exact `validateCoverage` preflight: `covered`, with one work order and no reference errors.
- `git diff --check`: passed. Both new plan files produced no whitespace diagnostics from `git diff --no-index --check` (exit 1 for added content).
- `git status --short --untracked-files=all -- docs/plans/repository-materialization-timeout`: both new package files are present and untracked.
- `git diff --quiet -- apps/backend`: passed after removal of the temporary reproduction.

The diagnostic reproduction produced the expected failure and successful control described above.
Implementation verification completed on 2026-11-14. The 61-second regression failed before the production change and passed afterward; caller cancellation/deadline and the five-minute timeout regressions also passed. The full `agentctl` race suite, focused lifecycle race suite, package build, changed-file Go lint, specification validation, documentation preflight, and public-doc validators passed. Exact commands and results are recorded in the work order.

## Documentation impact

Internal docs update the intended timeout and its regression contract.
The public executor documentation describes repository preparation without promising a one-minute materialization limit.
This package adds no operator setting or public workflow. No public-doc edit is required.

## Risks

- Five minutes limits the complete HTTP request. A clone plus checkout or admission wait can still exceed that budget.
- Earlier caller deadlines and provider connection failures can still stop materialization.
- A passing timeout test does not establish remote provider availability or arbitrary repository-size support.
