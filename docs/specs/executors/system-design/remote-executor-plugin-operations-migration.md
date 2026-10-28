---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-PLUGIN-OPS-001
  - REQ-EXECUTORS-PLUGIN-LAUNCHPOLICY-001
  - REQ-EXECUTORS-PLUGIN-PREPARATION-001
  - REQ-EXECUTORS-PLUGIN-MIGRATION-001
---

# Remote executor plugins: operations, launch policy, preparation, and migration system design

## Purpose and boundaries

Four operator-facing mechanisms gated by [capabilities](remote-executor-plugin-capability-versioning.md). Names marked
**new** do not describe existing APIs. Core owns the admin routes, redaction, audit, rate limits, the launch-policy
enforcement point in agentctl, intent verification, and the authority field. The provider owns node state, maintenance
effects, wrappers, and preparation strategies.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-EXECUTORS-PLUGIN-OPS-001 | Provider operations; Maintenance |
| REQ-EXECUTORS-PLUGIN-LAUNCHPOLICY-001 | Launch policy |
| REQ-EXECUTORS-PLUGIN-PREPARATION-001 | Preparation hints |
| REQ-EXECUTORS-PLUGIN-MIGRATION-001 | Migration and coexistence |

## Provider operations

**New** additive RPCs gated by `provider_operations`: `GetExecutorProviderStatus`, `ListExecutorInventory` (cursor, page at most
100), and `ReadExecutorProviderLog` (tail lines at most 500, 64 KiB). The host enforces the size caps itself after receiving the
reply: it rejects an oversize status (`response_too_large`), never truncates silently. The admin route
(`GET /api/v1/plugins/executor-providers/:installation/status`, `/inventory`, `/log`) is administrator-only and reuses the
existing plugin admin authorization. Log lines go through the host's secret redaction before they are returned. Inventory
reconciliation is read-only: entries are matched to host records by the opaque provider ID and shown as `unrecognized` when
absent; no host record changes (AC 001.2).

**Events.** A host callback `PublishProviderEvent` (alongside the existing executor callbacks, same dispatch-generation and
installation binding) validates the closed kind, severity, subject, message code, and parameters, applies a token bucket of
30 per minute per installation and a five-minute dedupe on the dedupe key, and calls the existing notification service for the
workspace administrators. It stores nothing new. Free text is rejected by schema, so an event cannot carry a log line or secret
(AC 001.3, 001.4).

## Maintenance

**New** RPC `ApplyProviderMaintenance` gated by `provider_maintenance` (which requires `provider_operations`). The admin route
`POST /api/v1/plugins/executor-providers/:installation/maintenance` takes `{action, subject_id, reason_code, idempotency_key}`
with the closed action set. The host writes the audit record before and after the call (actor, installation, action, subject,
outcome) and keeps a short-lived idempotency map keyed by installation and key whose entry holds the request digest, so a replay
with a different body is rejected (AC 001.7). The audit sink is the existing structured audit log line; if the owner wants a
persisted audit trail, that is a separate change (open question). This absorbs the former deferred provider-maintenance item.
An unavailable or version-mismatched provider returns `unavailable`; launches are unaffected (AC 001.8).

## Launch policy

Profile field `launch_policy` (`none`|`preferred`|`required`) and `policy_id` are validated by the profile editor path and again in
`validateLaunch` before allocation (AC 001.1, 001.2). The provider reports supported policy IDs in profile validation. The
workspace descriptor gains `applied_launch_policy`; the host rejects one weaker than the profile requires (AC 001.3). The host
passes `launch_policy` and `policy_id` in the agentctl bootstrap. Enforcement is in agentctl: for `required`, the process
manager refuses to start an agent process without the wrapper and reports `launch_policy_unenforced`; there is no unwrapped
fallback (AC 001.4). The existing `common/launchwrapper` package on main provides the spec and executable handling; the
node-policy files exist only on the native reference branch and are ported by the work order, test-first, from that
read-only reference. Wrapper discovery stays in the provider (AC 001.5).

## Preparation hints

The repository intent entry (native design) gains `checkout {kind: branch|ref|pull_request, value}`, `sparse_paths` (at most 64),
and `hints` (16 entries, key and value rules in AC 001.2). All of it is in the operation input digest, so changed intent with
the same operation ID is `conflict`. The host never interprets a hint key or a strategy label. Verification is generic and
unchanged by strategy: the returned set equals the intent set, each path is contained in the workspace path, the head commit is
40 or 64 lowercase hex characters, and the applied checkout kind matches the request (AC 001.3, 001.4). Adding a mirror, sparse
checkout, or pull-request strategy therefore needs provider work only. The fixture proves this with two strategies the host has
never seen, selected only through hints.

## Migration and coexistence

**Authority.** The envelope for plugin rows already names the installation; built-in rows get an explicit `authority: builtin`
metadata key on write, and rows without the key read as `builtin`. A record has exactly one authority and only
`transitionAuthority`, called by the migration operation under the cleanup claim, can change it; there is no code path that renames
an executor type (AC 001.2).

**Enumeration.** `GET /api/v1/executors/:id/migration/environments` reads built-in rows by executor type, classifying `active`
(live session), `drainable` (no live session, resources present), and `orphaned` (no owner). It is read-only (AC 001.1).

**Operation.** A **new** state machine stored on the executor profile metadata: `freezing`, `draining`, `switching`, `done`,
`rolled_back`. `freezing` sets `migrating`, which the allocation gate treats like an admission denial (`executor_migrating`),
committed with the profile revision so a launch admitted earlier finishes and is drained and a later one is refused (AC 001.3).
`draining` waits for sessions to stop and runs the built-in cleanup (with the drain receipt where the built-in supports one).
`switching` flips the profile to the plugin in one write. Each step records itself before acting, so a restart resumes from the
recorded step (AC 001.4).

**Rollback.** `rolled_back` switches new launches to the built-in profile. Plugin environments remain under the plugin as
retained records, are never adopted, and are read by the status and list paths; disabling or removing the plugin never deletes
them (existing retention rule). A later drain destroys them (AC 001.5, 001.6). D3 stands: the built-in code stays until the plugin
passes its live scenarios (AC 001.7).

## Data model

No new table and no DDL. Profile metadata holds the migration step; the envelope gains `authority` for built-in rows (tolerant
decode). Idempotency entries are in memory with a short TTL; losing them on restart only re-allows a replay, which the provider
treats idempotently by key. SQLite and PostgreSQL: the profile revision CAS covers the freeze race; PostgreSQL takes
`pg_advisory_xact_lock(hashtextextended('executor_migration:'||profile_id, 0))` for step transitions.

## Interleavings and failing-first tests

| Interleaving | Required outcome | Tests (level) |
| --- | --- | --- |
| Migration freeze races a new launch (MIGRATION 001.3) | earlier launch finishes and drains; later gets `executor_migrating` | sqlite and PostgreSQL race; lifecycle unit |
| Backend restart in each migration step (001.4) | resumes from the recorded step; no double freeze | barrier e2e; unit per step |
| Rollback with retained plugin environments (001.5, 001.6) | not adopted; still listed; survive plugin disable | store and lifecycle unit |
| Two operators send one maintenance key with different bodies (OPS 001.7) | second rejected | route unit |
| Event flood from a provider (OPS 001.4) | rate-limited and deduped; others delivered | callback unit with fake clock |
| Oversize status or log response (OPS 001.1, 001.5) | rejected, not truncated | route and lifecycle unit |
| Provider restarts mid-read (OPS 001.8) | `unavailable`; launches unaffected | fixture over the process boundary |
| `required` policy with no wrapper present (LAUNCHPOLICY 001.4) | agentctl refuses; no unwrapped start | agentctl process test |
| Provider reports weaker applied policy (001.3) | rejected before ready; cleanup | lifecycle unit |
| New strategy label and hint key (PREPARATION 001.4) | recorded, no host change | fixture over the process boundary |
| Returned repository set differs from the intent (001.3) | `repository_inventory_mismatch` cleanup | lifecycle unit |

Each test is written first and fails on main. Remaining ACs are table tests (hint rules, caps, action set).

## Observability

`executor_plugin_provider_event_total` (`kind`, `outcome`: delivered, rate_limited, deduped), `executor_plugin_maintenance_total`
(`action`, `outcome`), `executor_plugin_launch_policy_total` (`policy`, `outcome`), and `executor_migration_step_total`
(`step`). No installation, node, or environment label.

## Security

Admin routes require the existing administrator authorization and are audited. Node and environment IDs from the provider
are opaque and are never used to build host paths or commands. Logs are redacted by the host, not trusted to the provider.
`required` launch policy fails closed. Preparation hints and intent are non-secret by validation; credentials stay executor-owned.

## Related decisions

- [Remote executor plugin boundary](../../../decisions/2026-09-26-remote-executor-plugin-boundary.md)
- [Cleanup inventory](../../../decisions/0025-runtime-cleanup-uses-executors-running.md)
