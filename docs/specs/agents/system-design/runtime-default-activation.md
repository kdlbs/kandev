---
status: current
system: agents
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-002
created: 2026-09-07
owners:
  - Kandev
---

# Managed Runtime Default Activation System Design

## Purpose and boundaries

The agent system owns the effective version of each managed npm runtime. This design defines how a shipped default replaces an older operator selection.

The design covers built-in managed ACP runtimes only. Native agents, passthrough commands, authentication helpers, and active agent processes remain outside this boundary.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-RUNTIME-UPDATES-002` | [Startup reconciliation](#startup-reconciliation), [Persistence](#persistence), [Failure behavior](#failure-behavior) |

## Default generation

A default generation contains these trusted values for one built-in agent:

- The agent ID.
- The managed npm package.
- The exact Kandev default version.

The package and version come from the embedded managed-runtime catalog. A changed package or version starts a new generation for that agent.

An unrelated Kandev release does not start a new generation. Thus, the release preserves a user selection when its reviewed agent default remains unchanged.

## Components and responsibilities

- `agents.ManagedNPMRuntimeAgent` supplies each trusted package and default version.
- `managedruntime.Store` owns active selections and applied default-generation markers.
- `system/settings.Store` persists both record types in the install-wide `settings` table.
- `backendapp.provideServices` completes reconciliation before discovery, probes, command construction, or readiness.
- Existing lifecycle, registry, host-utility, and Settings consumers continue to read the effective selection through `managedruntime.SelectionReader`.

## Persistence

The existing active-selection key remains unchanged:

```text
managed_runtime.active.<agent-id>
```

Each managed agent also has an applied-generation key:

```text
managed_runtime.default.<agent-id>
```

The marker value contains the trusted package and exact default version. The marker does not contain an operator-selected version.

No database schema change is required. Both SQLite and PostgreSQL use the existing install-wide `settings` table.

## Startup reconciliation

`managedruntime.Store.ReconcileDefaults` receives the current built-in default generations. It processes agents in a stable order before runtime consumers start.

For each agent, reconciliation uses this flow:

1. Read the applied-generation marker.
2. Preserve the active selection when the marker matches the current package and default.
3. Delete the active selection when the marker is absent or differs from the current generation.
4. Save the current generation marker after the selection deletion succeeds.

The delete-before-save order is idempotent. If the process stops between these writes, the next startup repeats the safe reset.

A fresh installation writes generation markers and deletes no selections. The first release with this behavior treats each legacy selection without a marker as stale.

## Effective-version behavior

After reconciliation, existing resolution rules remain unchanged. A current-generation operator selection wins over the default. Otherwise, the shipped Kandev default is effective.

Settings can activate any validated stable version after startup. This action creates a new operator selection under the current generation, including a downgrade.

A later default-generation change removes that selection during startup. An unchanged generation preserves the selection across backend and browser restarts.

The reset applies to future probes and commands. Kandev does not replace an agent process that remains active during backend recovery.

## Failure behavior

An unreadable marker, failed selection deletion, or failed marker write stops service initialization. Kandev does not become ready with an unresolved older selection.

The marker advances only after selection deletion succeeds. Therefore, the next startup can retry every incomplete reconciliation.

Reconciliation does not query npm or probe a candidate. The reviewed default already passed repository validation before the Kandev release included it.

## Security

Only built-in agent metadata can create a default generation. Requests, profiles, and stored selection values cannot supply the package or default version.

Reconciliation removes settings keys only for known built-in managed agents. It does not scan or remove unrelated install-wide settings.

## Observability

Kandev logs one structured information event for each changed generation. The event includes the agent ID and the old and new trusted generation values.

Kandev logs the reconciliation error before startup stops. No new metric is required because the operation occurs once before readiness.

## Test strategy

Store tests cover fresh markers, matching generations, changed packages, changed defaults, legacy rows, replay, and each write error.

Backend composition tests prove that reconciliation finishes before runtime selection consumers start. Lifecycle tests prove that later resume commands use the new default.

The existing Settings end-to-end test proves that the operator can select an older validated version after the default becomes effective.

## Related decisions

- [Activate shipped managed-runtime defaults after an upgrade](../../../decisions/2026-09-07-activate-managed-runtime-defaults.md)
- [Validate and persist managed runtime version selection](../../../decisions/2026-08-12-validated-managed-runtime-version-selection.md)
