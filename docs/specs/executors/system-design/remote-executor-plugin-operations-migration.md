---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-PLUGIN-OPS-001
  - REQ-EXECUTORS-PLUGIN-LAUNCHPOLICY-001
  - REQ-EXECUTORS-PLUGIN-PREPARATION-001
---

# Remote executor plugins: operations, launch policy, and preparation system design

## Purpose and boundaries

Three operator-facing mechanisms gated by [capabilities](remote-executor-plugin-capability-versioning.md). Names marked
**new** do not describe existing APIs. Core owns the admin routes, redaction, audit, rate limits, the launch-policy
enforcement point in agentctl, and intent verification. The provider owns node state, maintenance
effects, wrappers, and preparation strategies.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-EXECUTORS-PLUGIN-OPS-001 | Provider operations; Maintenance |
| REQ-EXECUTORS-PLUGIN-LAUNCHPOLICY-001 | Launch policy |
| REQ-EXECUTORS-PLUGIN-PREPARATION-001 | Preparation hints |

## Provider operations

**New** additive RPCs gated by `provider_operations`: `GetExecutorProviderStatus`, `ListExecutorInventory` (cursor, page at most
100), and `ReadExecutorProviderLog` (tail lines at most 500, 64 KiB). The host enforces the size caps itself after receiving the
reply: it rejects an oversize status (`response_too_large`), never truncates silently. The admin route
(`GET /api/v1/plugins/executor-providers/:installation/status`, `/inventory`, `/log`, each taking the `provider_key` query selector of AC 001.1) is administrator-only and reuses the
existing plugin admin authorization. Log lines go through the host's secret redaction before they are returned. Inventory
reconciliation is read-only: entries are matched to host records by the triple of installation, `provider_key` and provider ID and shown as `unrecognized` when
absent; no host record changes (AC 001.2).

**Events.** A host callback `PublishProviderEvent` (alongside the existing executor callbacks, same dispatch-generation and
installation binding) validates the closed kind, severity, subject, message code, and parameters, applies a token bucket of
30 per minute per installation and a five-minute dedupe on the dedupe key, and calls the existing notification service for the
instance administrators (an installation is instance-wide; the service has no administrator audience today, so the recipients are `providerOwners` filtered by the same administrator check as the admin routes, and the default user when authentication is not enforced; an event with no resolvable recipient is dropped and counted, never sent to another account). It adds one event type `executor.plugin_event` to `AvailableEvents` and `validateEvents` in `internal/notifications/service/service.go`, with no default subscription and no back-fill; `providerOwners` falls back to the default user when no one owns a provider, so the drop for that case is `dropped_no_subscriber`, and its unresolved-owner branch under enforced authentication is `dropped_no_recipient`. The message is built in Go from a fixed per-kind English table and carries the kind, severity, subject, code, and parameters in the payload for the web client to localize under `executors:pluginEvent.<kind>`. It stores nothing new. Free text is rejected by schema, so an event cannot carry a log line or secret
(AC 001.3, 001.4).

## Maintenance

**New** RPC `ApplyProviderMaintenance` gated by `provider_maintenance` (which requires `provider_operations`). The admin route
`POST /api/v1/plugins/executor-providers/:installation/maintenance` takes `{provider_key, action, subject_id, reason_code, idempotency_key}`
with the closed action set. The host writes the audit record before and after the call (actor, installation, action, subject,
outcome) and keeps a short-lived idempotency map keyed by installation and key whose entry holds the request digest, so a replay
with a different body is rejected (AC 001.7). The audit sink is a structured log line, the owner's answer of 2026-10-28; a persisted audit trail would be a separate change. A replay with the same key and body inside 24 hours returns the recorded outcome (the map entry holds digest and outcome); a different body is `conflict`. This absorbs the former deferred provider-maintenance item.
An unavailable or version-mismatched provider returns `unavailable`; launches are unaffected (AC 001.8).

## Launch policy

Profile field `launch_policy` (`none`|`preferred`|`required`) and `policy_id` are validated by the profile editor path and again in
`validateLaunch` before allocation (AC 001.1, 001.2). The supported policy IDs are the manifest field `launch_policy_ids` (validated like the other closed manifest lists), not a provider report. The agentctl bootstrap result carries `launch_policy_outcome` (applied policy and enforcement result), which the host copies into the envelope. The
workspace descriptor gains `applied_launch_policy`; the host rejects one weaker than the profile requires (AC 001.3). The host
passes `launch_policy` and `policy_id` in the agentctl bootstrap. Enforcement is in agentctl: for `required`, the process
manager refuses to start an agent process without the wrapper and reports `launch_policy_unenforced`; there is no unwrapped
fallback (AC 001.4). The existing `common/launchwrapper` package on main provides the spec and executable handling. Its node-policy files exist only on the fork reference
(`nova28/kandev`, branch `feature/nomad-native-executor`, head `f0c3b0ab18`: `apps/backend/internal/common/launchwrapper/node_policy.go`, `node_policy_unix.go`, `node_policy_other.go`) and are
not ported by this cycle: node-policy files are provider-side (AC LAUNCHPOLICY 001.5), and the host and agentctl read only the wrapper specification that the provider places in the instance environment. The provider hands agentctl the wrapper spec through the
instance environment (AC 001.5). On `preferred`, agentctl tries the wrapper at every agent start and reports `wrapped` or `unwrapped`; the host stores the last outcome. The strength order and
rejection rule are AC 001.3.

## Preparation hints

The repository intent entry (native design) gains `checkout {kind: branch|ref|pull_request, value}`, `sparse_paths` (at most 64),
and `hints` (16 entries, key and value rules in AC 001.2). All of it is in the operation input digest, so changed intent with
the same operation ID is `conflict`. The host never interprets a hint key or a strategy label. Verification is generic and
unchanged by strategy: the returned set equals the intent set, each path is contained in the workspace path, the head commit is
40 or 64 lowercase hex characters, and the applied checkout kind matches the request (AC 001.3, 001.4). Adding a mirror, sparse
checkout, or pull-request strategy therefore needs provider work only. The fixture proves this with two strategies the host has
never seen, selected only through hints.

## Data model

No new table and no DDL migration. Launch policy and preparation settings are profile `config` keys (AC LAUNCHPOLICY 001.1,
PREPARATION 001.1). Maintenance idempotency entries are in memory with a 24 h TTL and a per-key single-flight; only `applied`, `noop`,
and `refused` outcomes are kept, and a backend restart clears them (AC OPS 001.7). The event token bucket and dedupe map are in
memory and reset on a restart. SQLite and PostgreSQL: no schema change, so the replayable-migration tests are unaffected.

## Interleavings and failing-first tests

| Interleaving | Required outcome | Tests (level) |
| --- | --- | --- |
| Same maintenance key while the first call is in flight; provider timeout (OPS 001.7) | waits and shares the outcome; timeout not recorded, replay calls again | route unit with fake clock |
| Inventory and node ordering, cursor edges, page of 0 or 101 (OPS 001.1, 001.2) | byte order; empty cursor starts; defaults clamped | route table test |
| Conflicting or malformed preparation settings (PREPARATION 001.1) | `invalid_config` with a named reason; no allocation | lifecycle table test |
| `required` policy on a provider without the capability or without the ID (LAUNCHPOLICY 001.2) | `launch_policy_unsupported`; no allocation | lifecycle unit |
| Two operators send one maintenance key with different bodies (OPS 001.7) | second rejected `conflict`; same body inside 24 h returns the recorded outcome with one provider call | route unit |
| Event flood from a provider (OPS 001.4) | rate-limited and deduped; others delivered | callback unit with fake clock |
| Oversize status or log response (OPS 001.1, 001.5) | rejected, not truncated | route and lifecycle unit |
| Provider restarts mid-read (OPS 001.8) | `unavailable`; launches unaffected | fixture over the process boundary |
| `required` policy with no wrapper present (LAUNCHPOLICY 001.4) | agentctl refuses; no unwrapped start | agentctl process test |
| `preferred` policy, wrapper absent on one start and present on the next (001.4) | each start reports its own outcome; latest recorded | agentctl process test |
| Provider reports weaker applied policy (001.3) | rejected before ready; cleanup | lifecycle unit |
| New strategy label and hint key (PREPARATION 001.4) | recorded, no host change | fixture over the process boundary |
| Inventory row with the same provider ID under another installation or provider key (OPS 001.2) | not matched to the host row; reported unmatched | route table test |
| Event with empty, over-long or bad-charset key or code; burst of 31 in 2 s (OPS 001.4, 001.6) | `invalid_request` before dedupe; validate, dedupe, then rate limit; bucket resets on restart | callback unit with fake clock |
| Wrapper missing, invalid, or failing to execute (LAUNCHPOLICY 001.4) | `required` refuses, `preferred` reports the outcome; no node-policy files | agentctl process test |
| Sparse list with blanks or whitespace; PR numbers for several repositories; bad label charset (PREPARATION 001.1, 001.3) | normalized or `invalid_config` with a named reason | lifecycle table test |
| Returned repository set differs from the intent (001.3) | `repository_inventory_mismatch` cleanup | lifecycle unit |

Each test is written first and fails on main. Remaining ACs are table tests (hint rules, caps, action set).

## Observability

`executor_plugin_provider_event_total` (`kind`, `outcome`: delivered, rate_limited, deduped, dropped_no_recipient, dropped_no_subscriber, delivery_failed), `executor_plugin_maintenance_total`
(`action`, `outcome`), `executor_plugin_launch_policy_total` (`policy`, `outcome`). No installation, node, or environment label.

## Security

Admin routes require the existing administrator authorization and are audited. Node and environment IDs from the provider
are opaque and are never used to build host paths or commands. Logs are redacted by the host, not trusted to the provider.
`required` launch policy fails closed. Preparation hints and intent are non-secret by validation; credentials stay executor-owned.

## Related decisions

- [Remote executor plugin boundary](../../../decisions/2026-09-26-remote-executor-plugin-boundary.md)
- [Cleanup inventory](../../../decisions/0025-runtime-cleanup-uses-executors-running.md)
