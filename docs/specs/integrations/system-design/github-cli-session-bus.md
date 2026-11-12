---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
---

# GitHub CLI session bus system design

## Purpose and boundaries

Integrations owns this contract because it owns GitHub credential resolution.
The shared subprocess package applies the policy at the existing execution boundary.
The design covers managed `gh` requests from the backend and agentctl.
It does not configure D-Bus for agent shells or arbitrary user commands.

The [authentication requirements](../requirements/github-authentication.md)
remain the behavioral source.
The [authentication design](github-authentication-02.md) owns credential routing.

## Requirement mapping

| Criteria under REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001 | Design section |
| --- | --- |
| AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.19 | Execution boundary; Bus address selection |
| AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.20 | Credential compatibility |
| AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.21 | Bus address selection; Failure and recovery |
| AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.22 | Execution boundary; Credential compatibility |

## Execution boundary

Add a private command preparation helper in
`apps/backend/internal/common/subproc/gh_environment.go`.
Call it immediately before execution in all five helpers in `shared.go`:

- `RunGH`
- `RunGHOutput`
- `RunGHCombinedOutput`
- `RunGHAfterAcquire`
- `RunGHCombinedAfterAcquire`

For the last two helpers, preparation follows the builder callback.
Keep admission, execution deadlines, output capture, and error propagation unchanged.
The existing GitHub account runner, client, and diagnostics then receive the policy.
The host credential probe and clone-protocol reader receive it through the same boundary.

On Linux, derive the effective environment from `cmd.Environ()`.
This preserves command-specific configuration and normal inheritance when `cmd.Env` is nil.
Use the existing environment lookup and replacement helpers.
Replace only the bus assignment when necessary.
Do not change `os.Environ()` or the environment supplied to unrelated children.
On other operating systems, leave the command untouched.

## Bus address selection

Preserve a nonempty explicit address unless its value is exactly `autolaunch:`.
For an absent, empty, or autolaunch value, choose an explicit address for this command:

1. Derive `/run/user/<uid>` from the process user ID, matching Linux godbus discovery.
2. If its `bus` path exists, select `unix:path=/run/user/<uid>/bus`.
3. Otherwise, read its legacy `dbus-session` file using the godbus assignment format.
   Accept a nonempty concrete address after the `DBUS_SESSION_BUS_ADDRESS=` prefix.
   Remove trailing line endings. Do not evaluate shell syntax or accept `autolaunch:`.
4. Without a usable legacy address, select the standard socket address even when it is absent.

Always pass the selected address explicitly.
A missing or disappearing socket then causes a connection error instead of another discovery attempt.
Do not leave the variable unset after an existence check.
Re-evaluate implicit discovery for each command. Do not cache a missing bus.

## Credential compatibility

The shared helper does not filter tokens or choose accounts.
`githubCLIEnvironment` retains its account-discovery filtering rules.
`systemGHAccountRunner.Run` retains its explicit `GH_HOST` and host/login arguments.
Legacy callers retain their ambient token precedence.
Preserve `HOME`, `GH_CONFIG_DIR`, `PATH`, and other caller-provided values.

GitHub CLI owns the keyring-to-configuration fallback.
Do not read, copy, persist, or log tokens in the bus helper.
Do not add a token cache, call `gh auth switch`, or substitute credentials.

## Failure and recovery

A bus lookup or connection error remains a keyring failure inside `gh`.
A file-stored credential can still resolve through its normal fallback.
A keyring-only account still needs access to its keyring.
Preserve the existing sanitized account errors when no credential can resolve.

If a login creates the user bus later, the next command can use it.
If a discovered socket disappears before execution, the explicit address prevents autolaunch.
The change does not terminate existing orphan daemons or restart a service.

## Persistence and observability

No migration, setting, runtime flag, or new metric is required.
Existing subprocess admission and error reporting remain authoritative.
Regression tests observe the child environment and autolaunch marker counts.
They must not print credential values or depend on a developer's user bus.

## Related decisions and sources

- [GitHub identity ownership](../../../decisions/0047-github-authentication-ownership.md).
- [GitHub CLI 2.103.3 credential fallback](https://github.com/cli/cli/blob/v2.103.3/internal/config/config.go).
- [godbus 5.2.2 address resolution](https://github.com/godbus/dbus/blob/v5.2.2/conn.go).
- [godbus 5.2.2 Linux discovery and launcher](https://github.com/godbus/dbus/blob/v5.2.2/conn_other.go).
- [Implementation package](../../../plans/github-cli-dbus-autolaunch/plan.md).
