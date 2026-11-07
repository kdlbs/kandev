# ADR-2026-11-06-notification-formatter-plugin-boundary: Keep content policy in formatter plugins

**Status:** accepted
**Date:** 2026-11-06
**Area:** backend, frontend, protocol

## Context

The Apprise template fork works but requires rebuilding the host for parser,
editor, and template-storage changes. The operator authorized a generic host
extension and independent plugin as two coordinated PRs. The inspected v0.99.0
SDK has no notification formatter hook. Bus subscriptions are not admitted
deliveries, and the Webhook payload lacks some inbox/runtime context.

The original [issue #5011](https://github.com/kdlbs/kandev/issues/5011) received
positive maintainer feedback for core templates. The
[split proposal](https://github.com/kdlbs/kandev/issues/5011#issuecomment-6682190312)
changes the execution/protocol boundary. The maintainer subsequently
[approved that split](https://github.com/kdlbs/kandev/issues/5011#issuecomment-6683963471)
and invited two coordinated contributions, with exact SDK/API review in the PRs.
The operator confirmed Q1-Q8 in grilling. The direction is accepted; public
symbol names still undergo contribution review, and no merge is implied.

## Decision

Use an opt-in provider-bound formatter contract. Core retains recipient and
subscription policy, occurrence/member claims, default copy, credentials,
transport, and sending. Core supplies authorized bounded semantic context and
accepts only a replacement title/body pair. Failure preserves the whole default
pair under the original delivery admission.

The plugin owns its renderer, variable catalog, immutable revision storage, editor,
synthetic preview, and operator-run legacy import utility in a separate
repository. Core owns generic provider binding, owner authorization, supervised
RPC dispatch, and a native UI mount/management relay. No template grammar or
Apprise-specific product state is added to upstream core.

Read the [system design](../specs/platform/system-design/notification-formatter-plugins.md)
for contract limits. Public symbols remain provisional until contribution review.
Follow the [published SDK decision](2026-10-09-published-plugin-sdk.md) for exact
versions and the SDK-wide host floor. Do not infer a future release number.

## Consequences

- Host and plugin development/review can proceed together after the contract
  discussion; public plugin release depends on host support and SDK publication.
- The unmodified v0.99.0 host cannot run this formatter. Until upstream support
  ships, any integration candidate is explicitly a patched development host.
- The management relay is an authorization boundary, not an opaque public webhook.
- Plugin-owned SQLite storage needs its own private snapshot/export in the
  backup/restore procedure. A host DB backup alone is insufficient.
- Runtime fallback retains notification availability but can deliver default
  copy; status and isolated parity checks must make that degradation observable.
- Preparation precedes first activation. Ordinary Save prepares a complete
  immutable revision before core conditionally selects it; no distributed
  transaction or cross-database atomic commit is claimed.
- Backups quiesce configuration mutation and capture a versioned host/plugin set.
  Rollback exports later edits, preserving event data unsupported by the old host.
- The operator authorized a latest-stable operational upgrade after isolated
  verification, including a minimal patched build if official stable lacks the
  extension. Public release still depends on supporting host/SDK publication.

## Alternatives considered

- **Keep templates in core:** simpler integration but couples product iteration
  and release cadence to the host; retain as an upstream discussion alternative.
- **Send from OnEvent:** duplicates recipient, enabled/subscription, claim, and
  runtime-summary policy. Not selected.
- **Webhook-to-plugin sender:** retains some admission behavior, but changes
  transport/configuration, moves sending and credentials, and lacks some semantic
  context. Not selected for behavior-preserving extraction.
- **Plugin reads host SQL/private endpoints:** bypasses public contracts and
  breaks upgrade isolation. Not selected. One-time export from an offline private
  snapshot is a migration tool, not a runtime integration.
