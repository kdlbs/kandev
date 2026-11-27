---
status: draft
system: platform
created: 2026-10-07
requirements:
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-002
---

# Profile enabled omission design supplement

## Ownership and scope

This supplements [agent settings parity](agent-settings-parity.md), the owner of
the existing cross-interface omission contract. Agents retains profile data and
[selection semantics](../../agents/requirements/profile-disable.md). The parent
design was 31,414 bytes before this repair, leaving 1,354 bytes under its 32 KiB
limit; the cohesive profile-write contract is documented here without duplicating
requirements or migrating the legacy selection specification.

AC-PLATFORM-AGENT-SETTINGS-PARITY-002.2 maps to enabled intent propagation;
002.4 to failure and dynamic rollback; 002.5 and 002.6 to existing publication
and saved-value projection; 002.10 to concurrent preservation and statement
results. This repair does not implement the entire settings-parity program.

No schema, wire field, global revision, admission policy, runtime flag, Office
writer migration, picker, UI layout, copy, navigation, or session behavior changes
are included. Other omitted fields retain current semantics. Mobile parity uses
the backend producer/data-only exception: the same existing desktop and phone
consumers receive corrected data; no browser or new mobile E2E work is needed.

## Current paths and failure

Paths below are relative to `apps/backend/`. At source base
`e4f11385ec772d421ab55b90e76c750a92233c02`:

| Entry or consumer | Existing contract and evidence boundary |
| --- | --- |
| REST `PATCH /api/v1/agent-profiles/:id` | `internal/agent/settings/handlers/profile_handlers.go:httpUpdateProfile` decodes `dto.ProfileUpdateRequest`, calls `UpdateProfileRequestFromDTO` and `Controller.UpdateProfile`, broadcasts `agent.profile.updated`, then returns the same DTO. Organization configuration scope and browser interlock remain in `handlers.go`. |
| Compatibility MCP update | `internal/mcp/server/config_handlers.go` forwards to `ws.ActionMCPUpdateAgentProfile`; `internal/mcp/handlers/config_agent_handlers.go:handleUpdateAgentProfile` uses the shared DTO/conversion/controller, publishes `events.AgentProfileUpdated`, and returns the DTO. Retain actual registered tool/dispatcher authorization. |
| Compact MCP update | `internal/mcp/handlers/settings_handlers.go:handleUpdateSettings` validates its registered patch, then `internal/backendapp/settings_operations.go:UpdateSettings` authorizes and invokes the same controller for `agent_profile`. It returns a sanitized saved-profile result. This source path has no profile publication call; this repair preserves that path and makes no claim of existing compact notification coverage. |
| Ordinary metadata update | `controller/profile_crud.go:UpdateProfile` reads a snapshot, merges supplied fields, then calls `Repository.UpdateAgentProfile`. `req.Enabled == nil` is documented as unchanged, but the full-row SQL writes the snapshot flag. |
| Enabled-only update | `enabledOnlyUpdate` chooses `UpdateAgentProfileEnabled`, which writes only enabled, user-modified state, and timestamp. Dependency/force checks run before this shortcut. |
| Dynamic metadata without route patch | The general base-row save has the same omission problem. Existing dynamic availability checks still apply. |
| Dynamic metadata plus routes | `updateDynamicProfileAtomically` uses `AtomicDynamicProfileRepository.UpdateAgentProfileWithDynamic`, which calls the same full-row helper inside the route transaction. A stale version rolls back both writes. Non-atomic lightweight adapters have the existing fallback path. |
| Legacy full-row consumers | Reconciler, discovery fallback, Office agents service and test support call `UpdateAgentProfile` with explicit full snapshots. They retain replacement semantics. `UpdateAgentProfileModelIfEmpty` and the SQL working-owner guards remain unchanged. |

The failure occurs when a second controller commits an explicit toggle after the
first controller's `GetAgentProfile` snapshot but before its rename/model write.
The general writer persists and returns the old flag. Existing notification
publishers faithfully forward that incorrect DTO, affecting availability for new
work while existing sessions keep their launch state.

## Enabled intent at the repository boundary

Add a required internal `Repository.UpdateAgentProfileWithEnabledIntent(ctx,
profile, enabled *bool) error`. Nil preserves the column at execution; a pointer
sets its explicit value, including false. It retains all other current full-row
fields and validation. This is an internal method, not a new public API or a
general patch abstraction. The controller forwards `req.Enabled` after its
current validation and dependency checks. Enabled-only requests keep the narrow
method unchanged.

Keep `UpdateAgentProfile` and `UpdateAgentProfileWithDynamic` with their existing
signatures and full enabled semantics. Both use the common writer with explicit
enabled intent. Extend the existing optional `AtomicDynamicProfileRepository`
with `UpdateAgentProfileWithDynamicEnabledIntent(ctx, profile, dynamic,
expectedVersion, routes, enabled *bool) error`; the controller's atomic path
forwards the same request intent. The production SQLite/PostgreSQL repository
implements it. Update bounded fakes and embedded wrappers so they exercise the
new caller path. No omission-aware production call may silently fall back to
the old full-row method.

For existing adapters without the optional atomic dynamic extension, keep the
existing `DynamicProfileRepository` fallback ordering and version behavior, but
use the required intent method for its base save. This adds no new atomicity
promise for such adapters and does not weaken the production atomic path.

## SQL and result projection

The common full-row writer changes only enabled assignment and how it obtains
the statement result. Use a rebound, parameterized assignment of the form
`enabled = CASE WHEN ? = 1 THEN ? ELSE enabled END`, binding presence and an integer
boolean value with the existing dialect helpers. Bind a non-null dummy value
when absent; never infer absence from false or from `profile.Enabled`.

Use `UPDATE ... WHERE id = ? AND deleted_at IS NULL RETURNING enabled` and scan
the statement result on the same writer/transaction. A query-capable update seam
is separate from the insert-only `profileExecer`; `*sqlx.DB` and `*sqlx.Tx` satisfy
it. All other SET clauses, enrichment, exact-model policy, provider
normalization, MCP selection, and working-owner CASE expressions remain intact.
No-row results retain the current not-found behavior. Statement, scan, context,
and commit errors must propagate without a success response or notification.

For an ordinary save, fully consume the returned row before applying enabled
and the saved timestamp to the caller profile. For a dynamic save, retain the
captured enabled result locally, update/version the routes in that transaction,
and expose enabled only after `Commit` succeeds. Route conflicts, missing
parents, insert failures, and commit failures cannot produce a successful DTO
or leave a committed base-row change. Preserve the current dynamic version and
rollback rules; do not add asynchronous success or a retry.

The statement naturally serializes against a concurrent row writer. Nil uses
the enabled value of the row actually updated. No pre-write reread, post-commit
reread, process mutex, advisory framework, profile revision, or schema change is
needed. A subsequent toggle may commit before delivery; the DTO and existing
event still describe this operation's committed flag, with no new global event
ordering promise. Explicit mixed saves continue to set their requested enabled
value under the current dependency/force rules. Legacy full saves continue to
set the value carried by their snapshot.

## Verification boundaries

Permanent regressions exercise two production controllers over the real SQLite
repository, with a wrapper only around `GetAgentProfile` to commit the second
production toggle after the first actual snapshot. Cover both directions and
rename/model payloads, persisted metadata, returned enabled state, and
uncontested/explicit controls. A second wrapper barrier after a real intent
write permits a later toggle before DTO delivery to prove statement-result
projection rather than a post-commit read.

REST and registered guarded MCP tests reuse this real controller/store fixture.
Mock only transport/broadcast/provider boundaries and assert response enabled
and exactly one existing event on success, none on rejection. Compact results
are tested through actual `settingsOperations`; no nonexistent event is claimed.
Do not replace caller tests with predicates over request pointers.

Environment-gated PostgreSQL store behavior tests use
`KANDEV_TEST_POSTGRES_DSN` and an isolated schema. `testutil.OpenIsolatedPostgres`
is single-connection and cannot alone prove waiting: open separately identified
physical connections to that same test-owned schema, set search path on each,
and assert distinct backend PIDs. Hold an uncommitted enabled row update, start
the intent write on another connection, observe the actual lock wait with a
bounded monitor query, then commit or roll back the held toggle and assert the
returned/stored result. Repeat through the dynamic transaction and force a route
version conflict to prove base rollback. Use channels/observable DB barriers,
bounded contexts, and joined goroutines, never a sleep as proof.

The sequential [work order](../../../plans/profile-enabled-omission/task-01-preserve-enabled-intent.md)
owns the full matrix, physical PostgreSQL resource receipt, scoped local gates,
and explicit native Windows execution wiring. Schema replay and Windows
cross-compilation do not substitute for actual behavior-test passes.

## Compatibility and delivery

Existing REST null decodes to nil, so it preserves enabled exactly as omission.
MCP's published boolean schema continues to reject null where currently rejected;
the guarded compatibility adapter's existing behavior is retained. Do not widen
schema nullability. The compatibility update tool advertises only `profile_id`,
`name`, `model` and `auto_approve`; its regression proves supported metadata
omission, while explicit enabled controls run at REST, compact settings and the
controller. Unknown/null controls follow the existing guard, without adding an
enabled argument or inventing stricter rejection. Creation defaults,
unchanged/no-op saves and user-modified
timestamps keep current behavior. Missing/deleted rows and failed statements
keep current error envelopes.

The existing domain-owned catalog and utility dependency safety decisions remain
authoritative. This is a bounded repair of an existing omission promise, so no
new ADR, persistence owner, or settings-catalog migration is needed. Public
`agents-and-profiles.md`, root README and screenshot guidance remain accurate:
no user operation, field, label, or recovery step changes.
