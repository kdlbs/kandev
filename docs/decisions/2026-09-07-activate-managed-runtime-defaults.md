# ADR-2026-09-07-activate-managed-runtime-defaults: Activate Shipped Managed Runtime Defaults

**Status:** accepted
**Date:** 2026-09-07
**Area:** backend, frontend, protocol
**Amends:**
[ADR-2026-08-12-validated-managed-runtime-version-selection](2026-08-12-validated-managed-runtime-version-selection.md)

## Context

Kandev ships reviewed default versions for its managed npm agent runtimes. Operators can also select an exact stable version for each runtime.

The prior decision kept an operator selection until manual removal. Therefore, an old selection could hide a newer default after a Kandev upgrade.

The release default represents the runtime that the Kandev release reviewed. The release must use that default before an operator makes another version choice.

## Decision

Each built-in managed agent has a default generation. The generation contains the trusted package and the exact Kandev default version.

Kandev records the applied generation in its install-wide settings. Startup compares this marker with the current embedded catalog before runtime consumers start.

If the generation changed, Kandev deletes the prior operator selection. Then it records the new generation, and the shipped default becomes effective.

If the generation did not change, Kandev keeps the operator selection. An unrelated Kandev release does not reset a selected runtime.

The first release with this rule treats an unmarked legacy selection as stale. This rule makes that release activate all current reviewed defaults once.

After startup, the operator can select any validated stable version. This selection can be older or newer than the default.

Kandev deletes the selection before it writes the generation marker. If either operation fails, startup stops before readiness and retries on the next start.

The reset changes future probes and launches only. Kandev does not replace an agent process that remains active during backend recovery.

## Consequences

A Kandev release activates each changed reviewed agent default. An operator must reselect a custom version after that release starts.

Same-generation restarts preserve the new selection. A later change to the package or default starts another generation and resets the selection again.

The settings store gains one small marker per managed agent. The startup operation is local and does not query npm or start an ACP probe.

The operation is idempotent but not one database transaction. Its delete-before-save order cannot restore an older selection after an interrupted start.

## Alternatives Considered

- Preserve every operator selection across upgrades. Rejected because an older selection can hide the runtime that the new Kandev release reviewed.
- Reset selections after every Kandev version change. Rejected because an unrelated release must not erase a deliberate agent-version choice.
- Reset only when the selected version is older. Rejected because SemVer order does not prove compatibility with the new Kandev release.
- Replace active agent processes during upgrade. Rejected because runtime updates affect future launches and must not interrupt active work.
