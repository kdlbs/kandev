# ADR-2026-09-25-session-mode-configuration-boundary: Keep start-mode overlays separate from configuration transfer

**Status:** accepted
**Date:** 2026-09-25
**Area:** backend

## Context

An agent can read a start mode from its settings file. Kandev can create a
session-owned settings file before launch, but an isolated executor must receive
that file at the path the agent reads. The executor profile has a separate,
explicit choice to copy host agent configuration. The first implementation of
start-mode delivery read host settings even when that choice was off. It also
wrote the mode before a later bundle transfer could replace the same file.
Changing the configuration-directory environment variable can affect agent
authentication, especially when a credential store binds access to a path.

## Decision

Treat the start mode as a Kandev-owned, per-session overlay. Build it from an
empty settings object or from a configuration bundle the executor profile
explicitly selected. Apply that overlay after selected bundle preparation and
before agent startup. Do not read, link, or transfer unselected host settings
or credentials to construct the overlay.

The lifecycle manager owns the resolved overlay and its delivery outcome.
Each executor adapter owns installation at the actual agent-visible path.
Only a verified installed path and preserved authentication may be reported as
start-mode delivery. If the agent's only start-mode channel requires a
configuration-directory change that breaks its authentication or an explicit
directory selection, report the start mode as unavailable for that launch.
Keep the authenticated session and do not claim that a later mode switch
provided start-mode enforcement.

## Consequences

- The portable-configuration opt-in remains authoritative for host file transfer.
- Start-mode delivery has executor-specific installation and failure results.
- Some host authentication methods may lack a safe start-mode channel until the
  agent integration offers another one. The session exposes that limitation.
- The overlay can use selected settings, but it cannot overwrite source files
  or remove other selected settings.

## Alternatives Considered

- Clone or link the entire host agent directory for each session. This bypasses
  the explicit transfer choice and can change authentication behavior.
- Write the mode into the user's shared settings. This lets one session change
  another session or the user's own agent.
- Write the mode first and let executor provisioning continue. A later selected
  bundle can replace the mode without Kandev noticing.
- Report delivery when the file exists only on the backend host. SSH and
  Kubernetes agents can read a different filesystem and path.
