# Startup Configuration Uses One Typed Source Model

- **Status:** accepted
- **Date:** 2026-08-20
- **Area:** backend, agentctl, frontend, cli, security, operations
- **Spec:** [Startup configuration parity](../specs/platform/requirements/startup-configuration-parity.md)

## Context

Kandev reads startup configuration in several places. The common configuration
package uses typed YAML and environment bindings. The Go launcher reads a
smaller environment-only set. Backend and agentctl packages also read some
environment variables directly.

This creates different behavior for one logical setting. It also means that a
YAML value can arrive too late for the launcher. Operators cannot use one
durable file for stable startup configuration.

Kandev already uses a first-readable-file model. It checks the working
directory and then `/etc/kandev`. Kandev also has a stable home directory, but
it does not search that location for configuration.

## Decision

Kandev will maintain one typed catalog for stable, operator-facing startup
settings. Each catalog entry defines its YAML key, compatible environment
variables, default, sensitivity, and owning component.

The launcher and backend will use the same catalog and source resolution rules.
The launcher will read only the bootstrap projection that it needs. The backend
will read the complete typed configuration. Consumers will receive resolved
values through constructors or explicit startup configuration. Managed
agentctl processes will receive their resolved subset through explicit child
configuration.

Kandev will not copy YAML values into public process environment variables.
An internal launcher handoff can identify the already selected configuration
file so the child process reads the same file after a working-directory change.
That handoff is process wiring and is not a public operator setting.

The precedence for startup settings will be launch flag, explicit environment,
selected YAML, then built-in or profile default. A database override, when one
exists, follows YAML and precedes the final default.

Kandev will select one file in this order:

1. Working-directory `config.yaml`.
2. `<KANDEV_HOME_DIR>/config.yaml`, with `~/.kandev` as the default home.
3. `/etc/kandev/config.yaml`.

The first existing candidate owns startup. An unreadable or invalid candidate
causes startup to fail. Kandev will not merge files or fall through after an
error.

The explicit home flag or environment variable selects the home path before
file discovery. A selected home file cannot relocate itself through `homeDir`.
Working-directory and system files can set `homeDir` because their own path does
not depend on that value.

Stable startup secrets will support YAML. Kandev will mark secret fields in the
catalog, avoid value logging, and warn about broad Unix read permissions on a
secret-bearing file. The warning will recommend owner-only mode `0600`.

The public configuration reference will inventory every stable startup
environment variable. It will name the YAML equivalent or explain why the
variable is internal, generated, test-only, debug-only, deprecated, or managed
through another surface.

## Consequences

- Operators can keep stable startup configuration in
  `~/.kandev/config.yaml` without repeating environment setup.
- Environment variables remain compatible and keep override priority.
- Launcher and backend behavior become consistent for ports, paths, logging,
  proxy trust, and other startup settings.
- Package-level environment reads for stable operator settings must move behind
  typed configuration boundaries.
- Child process launch paths need explicit tests for agentctl configuration.
- The message queue settings API needs a `configuration` source so the UI can
  describe YAML locks accurately.
- Adding a stable startup environment variable now requires a catalog entry or
  an explicit exclusion.
- File-based secrets are possible, but operators remain responsible for file
  access controls.
- Configuration changes still require a restart.

## Alternatives considered

### Copy YAML values into environment variables

This would reuse current direct environment reads. It would hide setting
ownership, make provenance unreliable, and put file-based secrets into more
process environments. We rejected it.

### Add only `server.trustedProxies`

This would fix the immediate proxy warning but preserve the split configuration
contract. Operators would encounter the same issue for other settings. We
rejected it.

### Expose every observed environment variable as YAML

Many variables are generated handoffs, test controls, debug tools, or platform
detection inputs. Making them public would freeze internal implementation
details. We rejected it in favor of a reviewed stable catalog and explicit
exclusions.

### Add a public `--config` flag instead of home discovery

A flag is useful for one launch command but does not give manual and service
launches a shared default. The home file provides that stable location. We can
add an explicit path later if a separate use case requires it.

### Merge working-directory, home, and system files

Merging increases provenance and secret-handling complexity. It would also
change the existing first-file behavior. We rejected it.
