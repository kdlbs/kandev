---
spec: docs/specs/integrations/github-authentication.md
created: 2026-08-07
status: done
---

# Implementation Plan: GitHub CLI account stderr fallback

## Overview

Update the host `gh` account command runner so a successful command with an empty
stdout can return its stderr report to the existing legacy account parser. Keep
non-empty stdout authoritative, preserving JSON account discovery and token
resolution behavior. Add a regression test at the real subprocess boundary.

## Backend

### Successful command output selection

- `apps/backend/internal/github/gh_accounts.go`, `systemGHAccountRunner.Run`: on
  a successful `gh` command, return stdout when it contains non-whitespace; if
  stdout is empty, return stderr. Preserve existing cancellation, timeout, and
  non-zero exit handling.

## Tests

- **What:** A legacy `gh auth status` command that exits successfully and emits
  its report only on stderr is parsed into the expected `GHAccount`.
  **File:** `apps/backend/internal/github/gh_accounts_test.go`.
  **How:** Use a temporary executable named `gh` on `PATH`, make structured
  status fail with an unsupported-flag exit, emit legacy status on stderr, and
  call `listGHAccounts` with `systemGHAccountRunner`.
- **What:** Existing JSON, legacy stdout, token, and environment-sanitization
  behaviors remain green. **File:** existing tests in
  `apps/backend/internal/github/gh_accounts_test.go`. **How:** run the focused
  package test selection and the package's full test suite.

## Verification Results

- RED: `rtk go test ./internal/github -run
  TestSystemGHAccountRunnerUsesStderrWhenStdoutEmpty -count=1` failed with
  `unsupported gh auth status output` before the production change.
- GREEN: the same regression test passed after the fallback was added.
- Focused: `rtk go test ./internal/github -run
  'Test(ListGHAccounts|ResolveGHAccountToken|GitHubCLIEnvironment|SystemGHAccountRunner)' -count=1`
  passed 12 tests.
- Full package: `rtk go test ./internal/github -count=1` passed 1,092 tests.
- `rtk git diff --check` passed.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-gh-account-stderr](task-01-gh-account-stderr.md)

No parallel-safe tasks: the production runner and its regression test share one
behavioral boundary.

## Open Questions

None.
