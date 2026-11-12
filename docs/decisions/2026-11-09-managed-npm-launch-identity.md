# ADR-2026-11-09-managed-npm-launch-identity: Separate installation identity from launch arguments

**Status:** accepted
**Date:** 2026-11-09
**Area:** backend, agentctl, protocol

## Context

PR #5208 preserves repaired npm installations across compatible launches.
Full-argv equality currently rejects a trusted package when a profile appends valid agent arguments.
The Claude ACP incident used `@agentclientprotocol/claude-agent-acp@0.100.0` with `--dangerously-skip-permissions`.
Both missing-manifest evidence and repair selection reject that command.

Installation ownership also differs between interactive and background callers.
Disabled authentication supplies a synthetic default user to HTTP requests.
Background callers can lack that identity and an explicit workspace owner.
These differences fragment repair identity without changing the installation.

## Decision

Recognize the registered npm launcher and its exact package separately from agent arguments.
Validate the canonical npm option region and required protocol arguments against trusted metadata.
Treat the remaining authorized argv as agent arguments.
Preserve those values and their order through retry and owned-cache rewriting.
Never search an arbitrary command prefix or diagnostic text for installation authority.
Wrappers, operator-defined commands, native binaries, and unsupported npm structures retain their existing exclusions.

Use one repair-owner resolution policy across task and discovery entry points.
A real authenticated actor takes precedence over an explicit workspace owner.
Without a real actor, retain an explicit workspace owner.
Only explicitly disabled authentication permits fallback to the established default single-user identity.
Unknown authentication state and unresolved ownership never authorize that fallback.
Profile discovery retains its authorized profile scope.
Authenticated system discovery retains a separate system scope when no single-user fallback applies.

Installation identity retains package, protocol, owner, platform, toolchain, npm configuration, and installer environment boundaries.
Profile CLI arguments do not independently partition identical installation files.
Attempt evidence still binds the full launch arguments and configuration revision.
Changing a profile cannot reuse stale failure or publication authority.

This extends the [reusable repair decision](2026-11-08-reusable-managed-npm-repairs.md).
It does not change authorization, permission policy, retry counts, normal npm preferences, or shared-cache ownership.

## Consequences

Valid profile arguments no longer disable recovery or compatible repair reuse.
Single-user background launches can reuse installations prepared through an interactive launch.
Authenticated users and profile result owners remain separate.

Callers share command recognition and owner resolution instead of maintaining independent predicates.
Parser tests must distinguish npm options, required protocol arguments, and opaque agent values.
Existing signed store records remain valid for their original keys.
A corrected owner can produce a normal miss without deleting older generations.

## Alternatives considered

- Full-argv equality rejects legitimate profile arguments.
- Searching for a package anywhere in argv accepts unsupported launchers and ambiguous npm structures.
- Removing CLI flags changes permissions and the selected launch behavior.
- Adding every profile flag to installation identity repeats identical installations.
- Mapping every missing actor to the default user weakens isolation when authentication is enabled.
- Keeping a separate single-user background owner repeats recovery for otherwise compatible launches.

## References

- [Incomplete-cache requirements](../specs/agents/requirements/managed-npm-cache-recovery.md)
- [Reusable repair requirements](../specs/agents/requirements/managed-npm-repair-reuse.md)
- [Recovery design](../specs/agents/system-design/managed-npm-cache-recovery.md)
- [Reuse design](../specs/agents/system-design/managed-npm-repair-reuse.md)
- [Fix package](../plans/managed-npm-review-fixes/plan.md)
