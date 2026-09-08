---
id: "02-harden-origin-reconciliation"
title: "Harden origin reconciliation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 02: Harden Origin Reconciliation

## Acceptance

- `SetOriginURL` keeps its per-repository lock, inspects the current origin, and performs no
  configuration write when the desired canonical URL already matches.
- Changed origins still update atomically through Git and failures remain fail-closed.
- Dubious ownership returns a stable, actionable service/data-ownership diagnostic.
- Other Git failures retain bounded useful context while redacting known secrets and HTTP(S) URL
  userinfo. Successful current origin values are never logged or returned.
- Kandev does not add `safe.directory`, retry as another user, or modify filesystem ownership.

## TDD Sequence

1. Add real-Git no-op and changed-origin cases; prove the current implementation rewrites or
   invokes the write boundary in the no-op case and record RED.
2. Add pure diagnostic-classifier tests for dubious ownership, `config.lock`, bounds, URL-userinfo,
   and known-secret redaction; record RED.
3. Implement inspection/no-op and credential-safe classification inside `repoclone` while retaining
   lock coverage across read and conditional write.
4. Re-run concurrency and invalid-repository tests to prevent regression.
5. Run the commands below and record GREEN.

## Verification

```bash
cd apps/backend && go test ./internal/repoclone -run 'Test.*(SetOriginURL|RepositoryGitError|RedactGitDiagnostic)' -count=1
cd apps/backend && go test ./internal/repoclone -count=1
```

## Files Likely Touched

- `apps/backend/internal/repoclone/clone.go`
- `apps/backend/internal/repoclone/clone_test.go`
- a focused diagnostic helper/test file if needed

## Dependencies

None.

## Parallelism

Production files are disjoint from Task 01, but execution remains in the primary conversation
unless the user explicitly authorizes delegation.

## Recorded Results

- RED: the matching-origin and diagnostic tests failed before the origin inspection/no-op and
  diagnostic classifier existed; the ownership case could only report a bare exit status.
- GREEN: `cd apps/backend && go test ./internal/repoclone -run 'Test.*(SetOriginURL|RepositoryGitError|RedactGitDiagnostic|RedactCloneOutput)' -count=1` passed 7 cases; `cd apps/backend && go test ./internal/repoclone -count=1` passed 48 tests.
- `SetOriginURL` now serializes read/compare/write, skips a matching origin, classifies dubious
  ownership with `errors.Is`, and bounds/redacts Git output, including complete multi-token
  `Authorization` values.

## Output Contract

Report the RED failures, exact no-op boundary, diagnostic/redaction examples using synthetic
secrets only, final GREEN results, and remaining Git-platform differences. Update task and plan
status.
