---
id: "01-reject-out-of-scope-repositories"
title: "Reject out-of-scope GitHub repositories"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-ERRORS-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-ERRORS-001.1
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-ERRORS-001.2
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-ERRORS-001.3
system_design:
  - ../../specs/integrations/system-design/github-repository-scope-errors.md
---

# Task 01: Reject out-of-scope GitHub repositories

## Summary

Check workspace GitHub scope for branch-list and merge-method requests before credential resolution or a provider request. Return one safe response from both endpoints. Keep the existing retry classification.

## In scope

- Give local scope rejection a typed service error that matches both scope and not-resolvable sentinels.
- Map the error to HTTP 404 with code `github_repository_out_of_scope`.
- Keep the rejected repository identity and workspace allowlist out of the response.
- Add service and endpoint regression coverage. Prove that rejected requests do not resolve credentials.
- Keep upstream not-found responses distinct.

## Acceptance

The implementation satisfies criteria 001.1 through 001.3 in the [requirements](../../specs/integrations/requirements/github-repository-scope-errors.md).

## Verification

- `go test -trimpath ./internal/github` passed.
- `python3 scripts/list-docs.py validate` passed.
- `python3 scripts/lint-spec-files.py --all` passed.
- The PR documentation evaluator returned `covered` for the changed source paths and this delivery package.
- `git diff --check` passed, and `errors.go` has no conflict markers.
