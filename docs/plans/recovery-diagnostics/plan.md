---
created: 2026-09-22
status: complete
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-001
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
  - ../../specs/platform/system-design/diagnostic-logging-01.md
legacy_specs: []
---

# Implementation plan: Recovery diagnostic fixes

## Overview

Improve evidence for failed Git refresh and PR discovery. Remove warnings for
intentional setup-script omissions. Execute three work orders sequentially.
Each work order has an independent result. The order prioritizes lost failure
evidence before warning cleanup.

## Evidence and requirement conformance

The supplied September 22 logs show two failed recovery attempts at 16:12 and
16:15 Lisbon time. Both stop at required base refresh with `exit status 128`.
Terminal requests report a missing workspace path. Another task later continues
with an `origin/master` fallback. The original task reaches setup resolution at
16:25, then reports no PR after push at 16:26.

These logs do not establish the Git cause, installed version, local-ref state,
or whether a PR existed. The plan repairs source-confirmed diagnostic defects.
It does not claim to repair the user's environment or change refresh policy.

- `handleBaseFetchFailure` uses output for coarse classification, then discards
  it. `pullCurrentBranchOrFallback` passes the process error without useful
  command context. The original logs can match pull failures too.
- `detectPushAndAssociatePRWithIdentity` merges errors and empty search results.
  `searchPRForExistingWatch` also discards provider errors. These are concrete
  gaps in discovery AC .3, extended with diagnostic AC .5.
- `resolvePreparerSetupScript` intentionally skips comment-only scripts but
  logs WARN. Platform AC .12 makes the intended severity explicit.

Smallest reproductions use fake Git command output, a scripted PR service,
and the existing setup resolver with an observed logger. No live environment
or GitHub credentials are needed. Permanent regression tests belong to the
implementation turn and must fail before the correction.

## Scope

### In scope

- Credential-safe Git operation, checkout identity, and diagnostic categories.
- Distinct provider-error, empty, found, and cancellation outcomes for post-push lookup.
- Debug severity for comment-only setup scripts.
- Tests for diagnostics and unchanged execution decisions.

### Out of scope

- Terminal readiness, frontend reconnect timing, 503 severity, or rate limiting.
- Git fallback policy, automatic credentials repair, branch changes, or lock deletion.
- Provider retry budgets, discovery health state, schema, APIs, UI, or translations.
- Deployment, live-instance changes, persistent subtasks, or delegated implementation.

## Ownership and technical approach

Workspaces owns base refresh and its Git diagnostics. Integrations owns PR
query outcomes. Platform owns diagnostic severity. Extend those existing
requirement/design pairs rather than creating a separate repair specification.

Task 01 adds diagnostic-only classification to fetch and pull failure paths in
`internal/worktree`. Fixed explanations preserve secret exclusion without a new
raw-output redaction boundary. Existing reasons retain their policy meaning.
Task 02 separates PR lookup results in both orchestrator entry paths and reuses
the GitHub category classifier. Task 03 changes one setup omission log level.
The paired designs define fields, cancellation, and final-outcome semantics.

No new ADR is needed. Existing credential exclusion and refresh decisions remain
authoritative, including the local-first and required-refresh ADRs linked by the
workspace design. No public documentation changes are needed for this change.

## Tests

| Acceptance criteria | Regression evidence |
| --- | --- |
| Workspace .14; preserve .3, .6, .10, .13 | New `internal/worktree/refresh_diagnostics_test.go`: `TestRefreshDiagnosticClassification`, `TestBaseRefreshDiagnosticEmission`, `TestRefreshDiagnosticsPreservePolicy`. Cover fetch/pull, fallback failure, unknown output, secrets, context, and unchanged decisions. |
| Discovery .3, .5; preserve .1, .2, .4 | New `internal/orchestrator/event_handlers_github_push_diagnostics_test.go`: `TestPushDiscoveryDiagnostics`, `TestExistingWatchDiscoveryDiagnostics`. Include mixed outcomes, cancellation, found PR, and exact repository identity. |
| Platform .12 | Extend `internal/agent/runtime/lifecycle/preparer_script_test.go`: `TestResolvePreparerSetupScriptDiagnosticSeverity`. Assert debug omission and unchanged resolution. Retain actual failure coverage in `env_preparer_setup_script_test.go`. |

These are backend log contracts. Observer tests through the real service paths
provide end-to-end diagnostic evidence. No browser layout or flow changes, so
no Playwright work order is necessary. Unknown environmental failures remain
unknown even after richer diagnostics.

## Companion package inventory

The completed `github-pr-discovery-health` and `github-pr-watch-reconciliation`
packages own health UI/admission and watch target stability. Their delivered
scope, browser matrices, and historical results stay unchanged. This package
adds only post-push log correctness. Existing Git fallback tests remain policy
regressions; this package does not reopen the refresh decisions.

## Work orders

- [x] [Task 01: Explain Git refresh failures](task-01-git-diagnostics.md)
- [x] [Task 02: Distinguish PR lookup outcomes](task-02-pr-diagnostics.md)
- [x] [Task 03: Quiet empty setup scripts](task-03-setup-log-severity.md)

## Verification results

Implementation complete on September 22, 2026. Design validation passed before
implementation:

- Catalog validation: 299 decisions and 1,108 specifications.
- Specification linter tests: 36 passed.
- Full specification lint: passed.
- Scoped diff whitespace check: passed.
- Scoped status inspection: six specification edits and four new plan files.

Implementation verification passed for every work order, including targeted
race checks, and `go build ./...` passed. Targeted `golangci-lint` for all four
affected backend packages passed with zero issues. Specification catalog
validation and full specification lint also passed. A repository-wide
`go test ./... -count=1` run reached the changed packages successfully but exited non-zero on unrelated
environment-sensitive failures in process probing, home-config discovery,
websocket subscription counting, GitHub fixture setup, launcher restart tests,
and executor timing.

The review remediation then corrected provider deadline handling in PR
discovery and the pull fast-forward diagnostic classifier, and added focused
regression fixtures. Builds and tests were not rerun for that remediation per
the review instruction. The verification results above therefore apply to the
pre-remediation implementation state at that time.

PR fixup then split the PR discovery retry handler to satisfy the backend
function-size limit, added the review-requested assertions and comments, and
guarded optional secret checks. The focused orchestrator, worktree, and GitHub
tests passed, and base-relative `golangci-lint` completed with zero issues. The
full backend build and test suites remain unrun locally.

Each work order supplies exact Go checks, rooted at the repository root.
Final package checks:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/recovery-diagnostics
git status --short -- docs/specs docs/plans/recovery-diagnostics
```

## Risks

- Git text varies by version and locale. Unrecognized output must remain unknown.
- Diagnostic codes must not become fallback policy inputs.
- Raw Git and provider output can contain credentials. Only fixed summaries leave classifiers.
- PR retries can have mixed results. Tests must prove the final outcome reflects the latest attempt.
- Package-level logger replacement in tests needs cleanup and serial execution.
- The original Git root cause requires the affected version and additional safe evidence.
