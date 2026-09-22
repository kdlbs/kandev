---
id: "01-git-diagnostics"
title: "Explain Git refresh failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.10
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.13
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.14
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 01: Explain Git refresh failures

## Summary

Add diagnostic evidence without changing refresh admission or fallback. Use
fixed categories and explanations instead of raw Git output.

## In scope

- Add `refresh_diagnostics.go` with a diagnostic-only classifier and fixed summaries.
- Integrate both fetch and pull failure paths, including configured fallback failures.
- Record operation, local checkout path, branch, existing reason, diagnostic code,
  fixed detail, and optional exit code. Do not read or emit remote URLs.
- Preserve error identities, progress payloads, return values, and ref selection.
- Correct the misleading raw-output comment on `syncFailureCause`.

## Out of scope

Other work orders, terminal behavior, UI, schema, retry policy, and live-instance changes.

## Acceptance

1. Known failures have useful fixed explanations. Unknown output remains unknown.
2. Logs, errors, and progress contain no injected secret or raw command output.
3. Required refresh, local fallback, missing-ref fallback, and cancellation retain their decisions.

## Regression evidence

In new `refresh_diagnostics_test.go`, add `TestRefreshDiagnosticClassification`,
`TestBaseRefreshDiagnosticEmission`, and `TestRefreshDiagnosticsPreservePolicy`.
The emission test must fail first because current logs lack the new fields.
Use the fake command pattern from `manager_test.go` and the observer pattern
from `manager_state_test.go`. Exercise fetch and pull through the manager,
not only the classifier. Include SSH public-key and host-key errors, DNS,
connection/TLS errors, missing refs, non-fast-forward errors, inaccessible
repositories, locks, unknown text, timeout, and cancellation.

Inject tokens, credential URLs, headers, control characters, and long output.
Assert their absence from all emitted fields, returned errors, and progress.
Use actual local refs to cover required failure and successful local fallback.
Assert no ref mutation or extra Git invocation. Keep the existing missing-ref
policy tests authoritative for eligibility.

## Verification

Run the regression first and record its expected failure. After implementation,
run these commands from the repository root:

```bash
(cd apps/backend && go test ./internal/worktree -count=1)
(cd apps/backend && go test -race ./internal/worktree -run 'Test(RefreshDiagnostic|BaseRefreshDiagnostic|RefreshDiagnosticsPreservePolicy)' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/worktree/manager_git.go`
- `apps/backend/internal/worktree/refresh_diagnostics.go (new)`
- `apps/backend/internal/worktree/refresh_diagnostics_test.go (new)`

## Dependencies

None. Execute in the plan order by default.

## Risks

Git text is not a stable API. Keep diagnostic classification separate from policy.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/workspaces/requirements/worktree-base-refresh.md)
- [System design](../../specs/workspaces/system-design/worktree-base-refresh.md)
- [Plan evidence and exclusions](plan.md)
- Backend AGENTS.md and TDD backend test guidance.

## Results

Implemented credential-safe diagnostic classification and emission for base
branch fetch and pull failures. The diagnostic fields preserve the existing
refresh reason and policy decisions, while fixed explanations and optional
exit codes keep raw Git output out of logs, returned errors, and progress.

The new regression suite covers authentication, SSH key and host failures,
DNS, connection, TLS, missing refs, non-fast-forward updates, repository
access, lock contention, unknown output, timeout, cancellation, fetch and
pull emission, and required-refresh policy preservation. The first emission
test failed before the production change because no diagnostic log existed.

Verification passed:

```text
go test ./internal/worktree -count=1
go test -race ./internal/worktree -run 'Test(RefreshDiagnostic|BaseRefreshDiagnostic|RefreshDiagnosticsPreservePolicy)' -count=1
git diff --check
```

## Review remediation

The classifier now recognizes Git's pull fast-forward refusal, uses local
branch wording for that outcome, and leaves tag-clobber output unknown. Fetch
and pull fixtures cover the real failure forms. Builds and tests were not
rerun for this review remediation, so the verification above describes the
pre-remediation implementation state at that time.

## PR fixup verification

The focused worktree regression passed after the review fixes:

```text
go test ./internal/worktree -run 'Test(RefreshDiagnostic|BaseRefreshDiagnostic|RefreshDiagnosticsPreservePolicy)' -count=1
golangci-lint run ./... --new-from-rev='195530e5766abf7f44f08e2d5ad436b22c28d748' --timeout=5m
```

The full backend build and test suites remain unrun locally.
