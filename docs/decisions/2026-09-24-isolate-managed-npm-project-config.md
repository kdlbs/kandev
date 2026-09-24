# ADR-2026-09-24-isolate-managed-npm-project-config: Isolate managed npm runtime project configuration

**Status:** accepted
**Date:** 2026-09-24
**Area:** backend, agentctl

## Context

Managed ACP processes start in a task workspace so the agent can work on the
repository. npm also treats that directory as its project configuration root.
A repository `.npmrc` with `min-release-age` can therefore reject a runtime
version that Kandev already prepared and validated on the host. Moving the
whole process out of the workspace would change agent behavior.

## Decision

Kandev uses the trusted npm argument `--prefix
~/.kandev/managed-npm-runtime` for every built-in managed npm command. npm
expands `~` on the execution host, so Kandev provisions that directory against
the child process's effective home before npm starts. It keeps the subprocess
working directory in the workspace. Cache discovery and recovery use the same
npm prefix and effective environment as the failed command. The prefix is
independent of an optional `KANDEV_HOME_DIR` override for state and logs.

The repository's project `.npmrc` does not govern Kandev's managed runtime
resolution. Explicit environment overrides and user/global npm configuration
continue to apply. npm release-date policy failures that remain after this
isolation are reported distinctly and do not trigger stale-cache repair.

## Consequences

Every managed npm execution path must provision the prefix before invoking npm.
Command validation and cache repair must use the new argument shape. The cache
key remains based on the exact package specification. Native and passthrough
commands retain their current working-directory and npm behavior.

## Alternatives Considered

- Changing the agent process working directory would break workspace-relative
  behavior and ACP consumers of `process.cwd()`.
- Setting `npm_config_min_release_age=0` would leak into agent shell commands
  and disable the repository's own supply-chain policy.
- Relying on the existing online retry would still apply npm's release-age
  filter and could delete a healthy execution tree.
- Embedding the backend's absolute Kandev-home path in command metadata would
  fail on Docker and SSH hosts with different filesystem roots.
