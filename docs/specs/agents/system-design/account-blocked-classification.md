---
status: current
system: agents
requirements:
  - REQ-AGENTS-ACCOUNT-BLOCK-001
  - REQ-AGENTS-ACCOUNT-BLOCK-002
---

# Provider Account-Block Classification System Design

## Purpose and boundaries

The Agents system classifies a restricted provider account and holds the
failed execution profile until its owner releases it. This design adds no
database table, migration, or event field. Existing settings routes keep their
contracts.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-ACCOUNT-BLOCK-001` | [Classification](#classification), [Hold](#hold) |
| `REQ-AGENTS-ACCOUNT-BLOCK-002` | [Owner release](#owner-release) |

## Components and responsibilities

- **`internal/agent/runtime/routingerr`** owns the `provider.account_blocked.v1`
  rule, the `account_blocked` code, its recovery flags, and
  `HoldsUntilRelease`.
- **`internal/agent/runtime/routingpolicy`** keeps a held code out of the
  same-candidate retry and reset-wait decisions.
- **`internal/agent/runtime/dynamic`** owns the circuit registry and the hold.
- **`internal/orchestrator`** reports the failed execution profile to the
  resolver for every dynamic-route failure.
- **`internal/agent/settings`** exposes the owner release.

## Classification

The rule is anchored to the start of a line, optionally inside a labeled JSON
message, so an agent quoting the error in prose does not match. It requires
the `403 Forbidden` gateway envelope followed by the `user_blocked` code or the
restriction message. Structured HTTP status keeps its priority, so a bare 403
stays a permission failure. The code is hard, allows fallback, and disables
automatic retry and user action.

## Hold

A hold is a circuit in the open state with no expiry whose code satisfies
`HoldsUntilRelease`. Its key uses the `account_block` scope and the concrete
execution profile ID, so it never shares a key with a credential or model
circuit. The existing circuit snapshot stores state and code, so a restart
restores the hold without a schema change. A hold applies in memory even when
its save fails; the save stays pending and is retried by the next circuit
change and by holding the same profile again.

The registry never grants a probe for a held circuit, never replaces it with a
timed suspension, and reports it as open. Candidate selection skips a held
profile before it consults the credential circuit. A legacy retry rule for a
held code moves to the next candidate. A route that was waiting to retry a
profile held in the meantime does not resume on it, by schedule or by a manual
retry; it returns to manual recovery.

Existing-route resolution also refuses a held concrete profile without
changing its retained generation. The conductor checks the hold immediately
before each downstream launch and resume, including a hold established while
building a continuation after selection. These checks prevent a retained or
already selected route from bypassing the selection and retry gates.

The orchestrator holds the failed execution's own profile when the failure is
reported, before the cancellation, attribution, and safety checks decide
whether to route, so a predecessor that still serves the session holds its
profile too. The hold keeps later selections away from the account. It does
not launch a successor or replay.

## Owner release

`POST /api/v1/agent-profiles/:id/account-block/release` runs behind the
settings configuration and interlock middleware. The controller rejects an
unknown or workspace-scoped profile as not found, then asks the routing runtime
to close that profile's hold. The registry saves the closed snapshot before it
forgets the hold; a save failure keeps the hold and returns an error. The
response is `{"released": true|false}`.
