---
id: "01-guard-gh-session-bus"
title: "Guard managed GH session bus discovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.19
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.20
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.21
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.22
system_design:
  - ../../specs/integrations/system-design/github-cli-session-bus.md
---

# Task 01: Guard managed GH session bus discovery

## Summary

Apply the Linux session bus policy at every shared GH execution boundary.
Prove the correction through child processes and document service credential behavior.

## In scope

- Add the private command guard and implicit address selector described in the design.
- Wire all five `RunGH*` helpers after their callers finish command construction.
- Use TDD for the account-runner regression and shared-helper environment cases.
- Add a short GitHub CLI credential note to the service guide during implementation.
  Its primary content type remains a how-to guide.
- Record exact results here and synchronize the plan after implementation.

## Out of scope

Token caches, new settings, service installation changes, orphan cleanup, and agent-shell environment changes.

## Acceptance

1. The real account-runner regression fails before the guard because its fixture observes autolaunch.
   After the guard, repeated resolution returns the selected fixture token with zero launch attempts.
2. All five shared execution helpers enforce criteria 001.19 through 001.22.
   Existing token filtering, host/login selection, output handling, admission, and deadlines remain intact.
3. The service guide explains file-token fallback and the need for a reachable keyring for keyring-only accounts.
   It does not prescribe lingering as a requirement for file-stored CLI credentials.

## Test cases

In `gh_environment_test.go`, add `TestGHEnvironmentSessionBusSelection`:

- Missing, empty, and exactly `autolaunch:` bus assignments use an explicit address.
- An explicit concrete address, including an unreachable one, stays authoritative.
- The standard user bus wins over a simultaneously present legacy address file.
- A valid legacy assignment resolves when the standard socket is absent.
- Missing, unreadable, malformed, empty, and autolaunch legacy values fall back to the explicit standard socket address.
- A later bus appearance is recognized. A selected address stays explicit if its socket disappears.
- Non-Linux preparation leaves the command unchanged.

Add `TestRunGHHelpersApplySessionBusPolicy` in the same file:

- Execute a fixture through `RunGH`, `RunGHOutput`, `RunGHCombinedOutput`,
  `RunGHAfterAcquire`, and `RunGHCombinedAfterAcquire`.
- Cover nil `cmd.Env` and an explicit environment with distinct credential/configuration sentinels.
- Assert the final child environment, including `HOME`, `PATH`, `GH_CONFIG_DIR`,
  `GH_HOST`, and token-variable preservation. Never print token values.
- Assert that the parent environment stays unchanged.
- Use the existing timeout/cancellation regressions to preserve the admission boundary.

In `gh_accounts_dbus_test.go`, add `TestSystemGHAccountRunnerDoesNotAutolaunchDBus`:

- Use an isolated executable fixture on `PATH` and call `ResolveGHAccountToken` repeatedly.
- Make the fixture recognize account-token help and exact host/login arguments.
- Without a usable bus assignment, make it invoke a marker helper that immediately exits.
- Require a file-token fixture result and zero marker invocations.
- Keep the fixture independent of the host user bus and installed CLI tools.
- Skip the Linux-specific autolaunch assertion on other operating systems.
  Keep native command-environment preservation coverage available there.
- Use private helper dispatch with both an environment marker and exact argument validation.
  Register resource cleanup before assertions. Do not create orphan daemons.

Reference the four AC IDs from their regression tests with `@covers` comments.
Production comments state the invariant without AC IDs.

## Files likely touched

- `apps/backend/internal/common/subproc/shared.go`
- `apps/backend/internal/common/subproc/gh_environment.go` (new)
- `apps/backend/internal/common/subproc/gh_environment_test.go` (new)
- `apps/backend/internal/github/gh_accounts_dbus_test.go` (new)
- `docs/public/run-as-a-service.md`
- The paired requirement, design, plan, and this work order for delivery status and results.

## Verification

Run from the repository root. Run the new account regression first for RED, then apply the correction.

```bash
(cd apps/backend && go test -trimpath ./internal/github -run '^TestSystemGHAccountRunnerDoesNotAutolaunchDBus$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/common/subproc -run 'Test(GHEnvironment|RunGH)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/github -run 'Test(ListGHAccounts|ResolveGHAccountToken|GitHubCLIEnvironment|SystemGHAccountRunner|GHClient_IsAuthenticated|GHClient_RunAuthDiagnostics)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/common/subproc/shared.go apps/backend/internal/common/subproc/gh_environment.go apps/backend/internal/common/subproc/gh_environment_test.go apps/backend/internal/github/gh_accounts_dbus_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run this documentation coverage preflight from the repository root:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const workOrder = 'docs/plans/github-cli-dbus-autolaunch/task-01-guard-gh-session-bus.md';
const paths = [
  workOrder,
  'docs/plans/github-cli-dbus-autolaunch/plan.md',
  'docs/specs/integrations/requirements/github-authentication.md',
  'docs/specs/integrations/system-design/github-cli-session-bus.md',
];
const result = validateCoverage({
  changedFiles: [
    { filename: 'apps/backend/internal/common/subproc/shared.go', status: 'modified' },
    { filename: workOrder, status: 'added' },
  ],
  fileContents: Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exit(1);
NODE
```

This preflight checks the requirement-to-work-order link for the production and documentation changes.
It passed after implementation. The system design is `current`, the plan is
`implemented`, and the requirement remains `active`.

## Dependencies

None. All changes belong to one sequential work order.

## Risks

Preserve explicit and legacy user-bus access.
Do not reuse the named-account token filter for legacy callers.
Permission-denied cases need a fixture that actually enforces permissions on root-like test hosts.

## Parallelism

`sequential`

## Inputs

- [Authentication requirement](../../specs/integrations/requirements/github-authentication.md), criteria 001.19 through 001.22.
- [Session bus design](../../specs/integrations/system-design/github-cli-session-bus.md).
- [Confirmed cause and reproduction](plan.md#confirmed-cause).
- Existing command-environment utilities in `git_environment.go` and subprocess tests in `shared_test.go`.
- Account fixtures in `gh_accounts_test.go`, runtime client tests in `gh_client_runtime_test.go`, and backend test guidance.

## Results

Implementation completed on 2026-11-11. Temporary investigation evidence is recorded in the plan.

Design-package checks on 2026-11-11:

- `python3 scripts/list-docs.py validate`: passed, including the new design and existing requirement.
- `python3 scripts/lint-spec-files.test.py`: passed all 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- The documentation coverage preflight above: passed with `status: covered` and no errors.
- `git diff --check`: passed. New package files also passed whitespace checks.

Implementation verification on 2026-11-11:

- RED was confirmed before the guard: the account fixture recorded four D-Bus autolaunch attempts across three token resolutions.
- `go test -trimpath -race ./internal/common/subproc -run 'Test(GHEnvironment|RunGH)' -count=1`: passed.
- `go test -trimpath -race ./internal/github -run 'Test(ListGHAccounts|ResolveGHAccountToken|GitHubCLIEnvironment|SystemGHAccountRunner|GHClient_IsAuthenticated|GHClient_RunAuthDiagnostics)' -count=1`: passed.
- `bash scripts/lint-go-changed apps/backend/internal/common/subproc/shared.go apps/backend/internal/common/subproc/gh_environment.go apps/backend/internal/common/subproc/gh_environment_test.go apps/backend/internal/github/gh_accounts_dbus_test.go`: passed with zero issues.
- `make -C apps/backend build`: passed. The cross-build reported that `codesign` and `rcodesign` were unavailable for Darwin artifacts.
- `node --test scripts/validate-public-docs.test.mjs`: passed all 63 tests; `node scripts/validate-public-docs.mjs`: validated 55 published pages.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed all 36 tests; `python3 scripts/lint-spec-files.py --all`: passed.
- Documentation coverage preflight: passed with `status: covered` and no errors.
- `git diff --check` and the changed-file `gofmt -l` check: passed.
