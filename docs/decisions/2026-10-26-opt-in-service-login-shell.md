# ADR-2026-10-26-opt-in-service-login-shell: Explicit login initialization for user services

**Status:** accepted
**Date:** 2026-10-26
**Area:** backend

## Context

Issue [#4527](https://github.com/kdlbs/kandev/issues/4527) reports missing tools
under macOS launchd. The installer preserves detected Node directories, but
separate mise shims and shell exports remain absent.
The shared local-agent design deliberately avoids shell startup files.

The user selected opt-in, non-interactive login initialization on 2026-10-26.
This decision accepts that boundary. Implementation and native acceptance remain
pending in the linked delivery package.

## Decision

Shell initialization belongs to an explicit native service-install choice.
It does not belong to general PATH normalization or individual agent discovery.
Existing installations retain direct startup unless the operator opts in.
The initial delivery covers macOS user services with Bash or Zsh.
System-service and other-shell support require separate account and invocation contracts.

The native Kandev executable remains the service manager's configured entrypoint.
It replaces itself through the selected login shell before normal startup.
The selection survives reinstallation and supported Apply updates.
An explicit reset restores direct startup.

Shared PATH normalization still never runs startup files. The CLI opt-in is a
documented exception at an earlier service boundary.
Service configuration retains precedence over conflicting shell exports.

## Consequences

Operators can supply exported tool paths and environment values through login files.
Opted-in startup depends on user scripts and can fail or hang before readiness.
Interactive-only initialization, aliases, and project activation remain separate.
The implementation must preserve service identity and update serialization.
Native macOS evidence remains necessary for launchd lifetime and background attribution.

## Alternatives Considered

- Default login initialization changes existing installations and their failure modes.
- Explicit PATH configuration cannot supply the requested broader shell environment.
- Additional fixed directories only repair selected tool-manager layouts.
- A plist that directly starts Zsh changes the configured executable identity.

## Related artifacts

- [Requirements](../specs/cli/requirements/service-login-shell.md)
- [System design](../specs/cli/system-design/service-login-shell.md)
- [Fix package](../plans/service-login-shell/plan.md)
