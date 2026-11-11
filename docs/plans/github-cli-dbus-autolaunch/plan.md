---
created: 2026-11-11
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
system_design:
  - ../../specs/integrations/system-design/github-cli-session-bus.md
legacy_specs: []
---

# Implementation plan: Prevent GitHub CLI D-Bus autolaunch

## Overview

Prevent managed GitHub CLI requests from creating an unowned Linux session bus.
Preserve file-based credentials and access to an existing user keyring.
One sequential work order implements the guard, regression coverage, and service documentation.

Issue: [kdlbs/kandev#5257](https://github.com/kdlbs/kandev/issues/5257).
Investigation source commit: `bceeaf1c1d30ad05b953fd800b56ae46ebf5f2bf`.
The issue is assigned to `carlosflorencio`.

## Confirmed cause

`CredentialResolver.resolveGHCLI` calls `ResolveGHAccountToken` through `ghToken`.
`systemGHAccountRunner.Run` passes the inherited environment after token filtering.
It sets `GH_HOST` for hostname arguments but supplies no D-Bus guard.

GitHub CLI 2.103.3 asks the keyring before its configuration file.
Its godbus dependency discovers `/run/user/<uid>/bus` or a legacy address file.
Without a discovered address, it runs `dbus-launch`.
Each short-lived CLI process can therefore leave a new bus behind.
The shared `RunGH*` helpers currently apply no command environment policy.
Other managed authentication and CLI requests can take the same path.

## Reproduction evidence

The investigation used a temporary Go overlay containing
`TestReproIssue5257AccountRunnerNoAutolaunch` in the real GitHub package.
It called `ResolveGHAccountToken` three times with the issue's exact CLI version.
The subprocess ran as UID 65534, whose standard user bus was absent.
Its temporary `hosts.yml` contained only a fixture token.
A fake `dbus-launch` recorded calls and exited without creating a daemon.

| Case | Token lookups | Autolaunch attempts | Outcome |
| --- | --- | --- | --- |
| Current code, bus variable unset | 3 | 6 | Fixture token resolved; no-autolaunch assertion failed |
| Current code, bus variable empty | 3 | 6 | Same failure |
| Current code, explicit missing user socket | 3 | 0 | Fixture token resolved; assertion passed |
| Temporary shared-helper prototype, unset | 3 | 0 | Assertion passed |
| Temporary shared-helper prototype, empty | 3 | 0 | Assertion passed |
| Temporary shared-helper prototype, `autolaunch:` | 3 | 0 | Assertion passed |

A separate owned session bus proved that an explicit live address remained reachable.
The temporary prototype also passed three lookups against that bus with zero autolaunch attempts.
`strace` recorded successful socket connections in both cases.
The investigation stopped and reaped that bus.
No live Kandev service or user configuration was changed.
The overlay, downloaded CLI, fixture files, and binaries are temporary evidence.
These results prove the launch path and proposed guard, not the reported long-term memory growth.

## Scope

In scope:

- Criteria `AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.19` through `001.22` in the
  [authentication requirement](../../specs/integrations/requirements/github-authentication.md).
- All five existing shared GH execution helpers.
- Explicit, standard, and legacy user-bus selection, including a bus that appears later.
- Caller environment preservation, account selection, and command-only changes.
- A short operator note in `docs/public/run-as-a-service.md` during implementation.

Out of scope:

- Token caching, authentication precedence changes, and new credential storage.
- Service unit changes, automatic lingering, restarts, and cleanup of existing orphans.
- Agent-shell environment changes, rendered UI, and new settings or flags.

## Technical approach

Follow the [session bus design](../../specs/integrations/system-design/github-cli-session-bus.md).
Add the private guard in `internal/common/subproc/gh_environment.go`.
Wire it after command construction in `internal/common/subproc/shared.go`.
Keep the existing admission and timeout boundaries.

Use an explicit socket address even when the socket is absent.
This prevents the discovery-to-launch race and recognizes a later user login.
Preserve valid legacy `dbus-session` discovery and explicit concrete addresses.
Keep token filtering in the existing account runner.

| Runtime or credential path | Expected behavior | Evidence |
| --- | --- | --- |
| Linux service, file token, no bus | Resolve exact account without autolaunch | Account-runner regression and isolated reproduction |
| Linux user bus, implicit or explicit | Retain keyring connectivity | Selection tests and owned-bus connection trace |
| Linux legacy address file | Pin its concrete address | Legacy-file selection cases |
| Named GitHub.com or Enterprise account | Preserve host/login and token filtering | Existing account-selection tests |
| Legacy token environment or host probe | Preserve caller token/configuration values | Child-environment tests across all five helpers |
| macOS or Windows | Leave the command environment untouched | Platform policy tests or native runs |

No new public API, persistence boundary, or architectural ownership rule is required.
The local execution design and this package preserve the repair rationale.
The earlier authentication and stderr-fallback packages remain historical records.

## Regression coverage

- `gh_environment_test.go`: `TestGHEnvironmentSessionBusSelection` covers implicit
  selection, absent/empty/autolaunch input, legacy files, and a bus appearing or disappearing.
- `gh_environment_test.go`: `TestRunGHHelpersApplySessionBusPolicy` drives all five
  real execution helpers with inherited and explicit environments.
  It checks child-visible values, parent preservation, and existing cancellation behavior.
- `gh_accounts_dbus_test.go`: `TestSystemGHAccountRunnerDoesNotAutolaunchDBus`
  drives the real account resolver with an isolated executable fixture.
  The fixture attempts a marked autolaunch when the guard is missing.
  It must fail on the current code and pass after the correction.
- Existing account, Enterprise, stderr, and client authentication tests retain their current assertions.

The subprocess regression provides end-to-end evidence for this backend-only behavior.
It requires neither a browser nor installed `gh` or `dbus-launch` binaries.
The work order contains exact commands and the documentation coverage preflight.

## Work orders

- [x] [Task 01: Guard managed GH session bus discovery](task-01-guard-gh-session-bus.md)

Executed sequentially. See the work order for implementation and verification results.

## Verification results

Investigation: the original overlay assertion failed with six attempts for three lookups.
The temporary prototype passed the same assertion with zero attempts.

Implementation completed on 2026-11-11. The permanent account-runner regression
records a positive-control autolaunch and verifies three exact-account token
resolutions complete with zero further attempts. All five shared GH helpers
preserve caller environments and select an explicit Linux bus address.

The work order records the targeted race tests, changed-file lint, backend build,
public documentation checks, specification validation, coverage preflight, and
whitespace results.

## Risks

- An unconditional `disabled:` override breaks reachable keyring-only credentials. Preserve existing bus access.
- Leaving discovery enabled after a socket existence check retains the race. Always pass a concrete address.
- Reusing account-token filtering in the shared guard changes legacy credential precedence. Keep those responsibilities separate.
- Existing orphan daemons remain outside this repair's cleanup scope.
