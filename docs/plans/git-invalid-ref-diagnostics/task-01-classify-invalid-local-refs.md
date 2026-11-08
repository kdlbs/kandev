---
id: "01-classify-invalid-local-refs"
title: "Classify invalid local Git refs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.14
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.10
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 01: Classify Invalid Local Git Refs

## Summary

Recognize explicit bad local reference errors in backend refresh diagnostics.
Preserve the existing launch decision and credential exclusion contract.

## In scope

- Add the diagnostic constant, fixed explanation, and fatal-line matching.
- Add focused classification and real Git fetch-path regression coverage.
- Run existing diagnostic emission and policy tests.

## Out of scope

- Live repository repair, automatic ref removal, or runtime restart.
- MCP response, user-facing copy, fallback, or raw-output changes.

## Acceptance

1. A required manager refresh with an unrelated empty ref fails with its existing
   error identity and emits `invalid_local_ref` with operation and exit code.
   A best-effort refresh retains its local fallback and emits the same category.
2. Explicit `fatal: bad object refs/...` and `fatal: ... bad ref refs/...`
   shapes classify. Incidental URL/branch text, missing blobs, arbitrary object
   IDs, and `did not send all necessary objects` alone do not classify.
3. Cancellation and timeout win. Log explanations contain no stderr substring,
   URL, secret, or broken ref name. The fixture ref remains unchanged after
   manager execution, proving the classifier performs no repair.

## Verification

Run from the repository root.
Run the new tests before changing code to establish the regression.
Use local fixture remotes and `t.TempDir`, never the user's repositories.

```bash
(cd apps/backend && go test -trimpath ./internal/worktree -run 'Test(InvalidRefFixtureIgnoresPersonalGitConfig|InvalidLocalRefDiagnosticClassification|InvalidLocalRefRefreshDiagnostic|RefreshDiagnosticClassification|BaseRefreshDiagnosticEmission|RefreshDiagnosticsPreservePolicy)$' -count=1)
bash scripts/lint-go-changed apps/backend/internal/worktree/refresh_diagnostics.go apps/backend/internal/worktree/refresh_invalid_ref_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/worktree/refresh_diagnostics.go`
- `apps/backend/internal/worktree/refresh_invalid_ref_test.go` (new)
- This work order and `plan.md` for results.

## Dependencies

None. Read scoped backend guidance and the backend testing reference before TDD.

## Risks

Avoid matching a generic suffix that also occurs for incomplete object stores.
Do not turn diagnostic categories into fallback authorization.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/workspaces/requirements/worktree-base-refresh.md), AC .14.
- [Design](../../specs/workspaces/system-design/worktree-base-refresh.md), safe refresh diagnostics and invalid-ref subsection.
- [Incident evidence](diagnostic-report.md).
- Existing patterns in `refresh_diagnostics_test.go` and `manager_git.go`.

## Results

Completed on 2026-10-22 after explicit implementation authorization.

- RED: `(cd apps/backend && go test -trimpath ./internal/worktree -run 'Test(InvalidLocalRefDiagnosticClassification|InvalidLocalRefRefreshDiagnostic)$' -count=1)` failed at diagnostic assertions: expected `invalid_local_ref`, received `unknown`. Both real Git policy cases reproduced the gap.
- GREEN: the complete targeted `go test` command in Verification passed.
- `bash scripts/lint-go-changed apps/backend/internal/worktree/refresh_diagnostics.go apps/backend/internal/worktree/refresh_invalid_ref_test.go` passed with zero issues. The installed Nix golangci-lint binary directory was added to PATH for the command.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` passed.
- The local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight reported `covered`, with no reference errors.
- `git diff --check` passed. Only backend diagnostics and their delivery records changed, so browser captures and public documentation changes were unnecessary.

The real fixture proves healthy fetch, failure with an unrelated empty ref,
unchanged required/best-effort decisions, preserved broken-ref bytes, and
restored success after fixture-only removal. No live repository repair ran.

### PR review correction

The fixture now uses a dedicated Git runner that disables global and system
configuration after removing inherited Git environment overrides. Personal
hooks cannot reject its empty setup commit. A regression supplies rejecting
hooks through both home and XDG Git configuration; both cases failed before
the correction and passed afterward. The complete six-function targeted suite
and changed-package Go lint passed. Production behavior is unchanged.
