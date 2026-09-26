---
id: "01-gh-account-stderr"
title: "Handle gh account status on stderr"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 01: Handle `gh` account status on stderr

## Acceptance

- A successful `systemGHAccountRunner.Run` returns non-empty stdout unchanged,
  and returns stderr when stdout is empty.
- `listGHAccounts` discovers a legacy account when the real `gh` command emits
  its successful status report only on stderr.
- Existing timeout, cancellation, non-zero exit, JSON, legacy stdout, and token
  behaviors remain unchanged.

## Verification

- `cd apps/backend && go test ./internal/github -run 'Test(ListGHAccounts|ResolveGHAccountToken|GitHubCLIEnvironment|SystemGHAccountRunner)' -count=1`
- `cd apps/backend && go test ./internal/github -count=1`

## Files likely touched

- `apps/backend/internal/github/gh_accounts.go`
- `apps/backend/internal/github/gh_accounts_test.go`
- `docs/specs/integrations/requirements/github-authentication.md`
- `docs/plans/github-cli-account-stderr/plan.md`
- `docs/plans/github-cli-account-stderr/task-01-gh-account-stderr.md`

## Dependencies

None.

## Parallelism

Sequential. The production runner and regression test share the same command
output contract.

## Inputs

- The amended named-CLI behavior and scenario in
  `docs/specs/integrations/requirements/github-authentication.md`.
- The confirmed root cause in `systemGHAccountRunner.Run` and the existing
  stderr-first pattern in `GHClient.RunAuthDiagnostics`.
- Issue `kdlbs/kandev#2348`.

## Output contract

Report the files changed, exact test commands and outcomes, any remaining risk,
and synchronized task/plan statuses in the primary session.

## Results

- RED: `rtk go test ./internal/github -run
  TestSystemGHAccountRunnerUsesStderrWhenStdoutEmpty -count=1` failed as
  expected with `unsupported gh auth status output` before the production
  change.
- GREEN: the same regression test passed after the fallback was added.
- Focused task check: `rtk go test ./internal/github -run
  'Test(ListGHAccounts|ResolveGHAccountToken|GitHubCLIEnvironment|SystemGHAccountRunner)' -count=1`
  passed 12 tests.
- Full task check: `rtk go test ./internal/github -count=1` passed 1,092 tests.
- `rtk git diff --check` passed.
- The helper executable is copied into `t.TempDir()` and removed by Go test
  cleanup; no temporary files or processes remain.
- Security/trust boundary: the runner still strips ambient `GH_TOKEN` and
  `GITHUB_TOKEN`; the fallback only selects command output after a zero exit.
