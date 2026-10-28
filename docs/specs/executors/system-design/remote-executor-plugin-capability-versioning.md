---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-PLUGIN-CAPABILITY-001
  - REQ-EXECUTORS-PLUGIN-CAPABILITY-002
  - REQ-EXECUTORS-PLUGIN-CAPABILITY-003
  - REQ-EXECUTORS-PLUGIN-CAPABILITY-004
---

# Remote executor plugins: capability negotiation and versioning system design

## Purpose and boundaries

One registry, one negotiation point, one pinned snapshot. This design is built first: every other capability in the
[native worker design](remote-executor-plugin-native-workers.md), the [admission and placement design](remote-executor-plugin-admission.md),
and the [operations design](remote-executor-plugin-operations-migration.md) is a registry entry. Names
marked **new** do not describe existing APIs. The base [plugin design](remote-executor-plugins.md) keeps the RPCs,
leases, and inventory; the provider contract version stays 1.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-EXECUTORS-PLUGIN-CAPABILITY-001 | Registry; Manifest syntax |
| REQ-EXECUTORS-PLUGIN-CAPABILITY-002 | Negotiation points |
| REQ-EXECUTORS-PLUGIN-CAPABILITY-003 | Snapshot; State schema versions; State budget |
| REQ-EXECUTORS-PLUGIN-CAPABILITY-004 | Deprecation and retirement |

## Registry

**New** package `internal/plugins/hostcaps` holds a table, one row per capability: `name`, `min`, `max`, `deprecated_in`
(empty), `retire_in` (empty), `introduced_in`, `requires` (names). It sits beside `CurrentExecutorProviderContractVersion`
and has no imports from the lifecycle or store packages, so the manifest validator, lifecycle, and status route all read it
without a cycle. Entries are code, not data: adding a capability is a table row, a validator test, and a proto field. A
`Retired` list of `(name, version)` pairs is append-only; a test compares it with the previous release's list (a committed
golden file) and fails on removal or reuse, mirroring the runtime-flag retired identities.

## Manifest syntax

`requires_host_capabilities` becomes a list of `{name, version}` objects in `executor_providers[]`, validated in
`internal/plugins/manifest/executor_provider.go`. Rules: closed names, no duplicates, integer version in the registry
range, dependency closure (`provider_maintenance` needs `provider_operations`; `pending_placement` needs `destroy_checkpoints`, `provision_refusal`, and `durable_bootstrap` (a queued launch outlives the backend process, so its bootstrap nonce must be persisted); `launch_policy` needs `workspace_descriptor`;
`preparation_hints` needs `repository_intent`; `typed_profile_values` and `secret_references` need none; `provision_refusal` needs `destroy_checkpoints`). A manifest declaring `launch_policy` may carry `launch_policy_ids`, a closed list of at most 16 IDs `[a-z0-9_.-]{1,64}` with no duplicates; a profile `policy_id` outside it is rejected before allocation. `state_schema` is `{current, min_readable}`, absent meaning `1/1`. `min_kandev_version` must be at least the highest `introduced_in` release among the declared capabilities (AC 002.6); the existing generic minimum-version check then makes an older host refuse the package with a clear message. Cross-field
rules from the native design (preparation mode, API URL mode, destroy timeout) stay and consult the same registry.

## Negotiation points

1. **Install, upgrade, enable:** the validator output plus the registry check, before activation (AC 002.1). A failure leaves
   the working installation active, reusing the upgrade-rollback path from the base design.
2. **Profile validation:** `ValidateExecutorProfile` gains additive request field `host_capabilities` (the registry rows) and
   response field `implemented_capabilities`. The echo is captured from every `ValidateExecutorProfile` response: the
   activation probe in `validateExecutorProviderContracts` (`internal/plugins/executor_providers.go`) and each profile save or launch validation. The host keeps the last accepted echo in memory per
   (installation, dispatch generation). A second echo of the same generation that differs from the first, or an echo that differs from the manifest, marks the
   provider `unavailable` with reason `capability_mismatch` until the generation changes (AC 002.2). An absent or empty echo is
   treated as declaring none (AC 002.4). After a plugin restart the new generation has no echo, so the gate in step 3 runs one
   profile validation first, with a 10 second deadline; a manifest that declares no capability skips the echo gate entirely (no echo required, no validation run for it) and keeps launching. Concurrent first validations of one generation share one call (single-flight per installation and generation) and the first accepted echo stays. A validation that errors, times out, or returns `unavailable` is not an empty echo: nothing is cached, the launch is refused as `unavailable` with no allocation, and the next launch tries again (no negative cache). Single-flight is keyed by (installation, generation, provider key), the same key as the stored echo, and each echo is compared with the declaration of its own `executor_providers[]` entry.
3. **Allocation gate:** the local aggregate `profile_too_large` measure of the profile contract (one shared function, AC PROFILE 001.8; the schema-based checks of PROFILE 001.7 need the provider's schema and stay in profile validation) runs first in `validateLaunch` and in `attachRetainedPluginExecutor` and never calls the provider, so an oversized profile is refused before the post-restart echo validation of step 2; then **new** `checkCapabilityAgreement` runs, before the admission hook and before any
   provider call, on first launch, delayed launch (the continuation of a pending placement, against the recorded negotiated set), and resume. The measure and `checkCapabilityAgreement` are two functions the two entry points call directly: neither lives inside `admitLaunch` and neither depends on `admission_context` or on any admission step existing, so a provider that requires only other capabilities (for example `pending_placement` or `launch_policy`) is still measured and gated on first launch and on resume. The order is fixed: measure, then capability gate, then (when the provider requires `admission_context`) the admission hook of the admission design. It compares manifest, registry, and the in-memory echo (AC 002.3). Only the manifest-and-registry half is a pure
   function (AC 002.5); **new** `checkStateWritable` runs in the same step and is described under Data model.

   **Resume generation fence (new; main has no stale-generation error).** On main the only ownership check on the retained path is the read inside `ExecutorProviderProfileForRecovery`, called from `loadRetainedPluginExecutor`, which returns `ErrExecutorProviderUnavailable` ("recorded environment ownership changed"). `attachRetainedPluginExecutor` shall re-read the recorded dispatch generation of the environment row once more immediately before its first provider call (`InspectExecutorEnvironment`), after the measure, the capability gate and any later step that can block (the admission hook, when present). A changed generation, or a row that no longer exists, ends the resume with the same `ErrExecutorProviderUnavailable` (provider state `unavailable`, no provider call made, no cleanup confirmed, the record retained); no new error code is introduced. The re-read is a read of the row under the existing per-environment recovery claim, not a new lock, and its result is final for that attempt: a later change is caught by the generation fence on the provider response (AC CAPABILITY-003) as for any other call.

## Snapshot

At admission the host writes `negotiated_capabilities` (sorted `name@version` strings) and the existing `state_version` into the
`plugin_executor` envelope, in the same position as the existing `capabilities` key, which keeps its meaning. A record without `negotiated_capabilities` reads as the empty set (AC 003.6). Recovery, resume, destroy, lease
validation, and response validation read the snapshot. A host that no longer supports a recorded version maps the
provider to `unavailable` for that environment through the existing availability path (AC 003.5).

## State schema versions

Main already carries the mechanism: the manifest field `supported_state_versions` (`internal/plugins/manifest/manifest.go`), the descriptor
`state_version`, the envelope fields `supported_state_versions` and `state_version`, the upgrade guard
`validateExecutorProviderUpgradeInventory` in `executor_lifecycle_guard.go`, and the attach rule that rejects a changed version. This design
maps onto it and adds no parallel record: `state_schema {current, min_readable}` is a second spelling of the list (expanded by the validator,
never both, AC 003.2); the guard also runs on rollback and range changes (AC 003.3); a version rises only through a provision, recovery, or
checkpoint response, never on attach, never downward, and never outside the readable list (AC 003.4). The host never reads the state body.
The same query blocks retirement of a state version that retained environments record (AC 004.4).

## State budget

The envelope cap `pluginExecutorInventoryMaxBytes` is 32768 bytes on the marshaled JSON and stays. For a provider that declares any capability the
envelope is split into a host reserve of 20480 bytes and provider state of at most 12288 bytes. Every host-written field has a per-field maximum
enforced where it is written, so the reserve cannot be exceeded by data the provider influences:

| Host field | Maximum (bytes) |
| --- | --- |
| existing fields, with `profile_config` (all provider-facing keys including the document-set keys of AC PROFILE 001.8) plus `secret_references` at most 4096 together (checked at profile save and launch validation as the field error `profile_too_large`, AC PROFILE 001.8) | 5888 |
| workspace descriptor (`workspace_path` at most 1024, identity 128, preparation, platform), stored once in the envelope; the resource descriptor copy is stripped before persisting | 1792 |
| `negotiated_capabilities`, `state_version`, `profile_snapshot_digest` (64) | 1024 |
| `repositories`: 32 entries of `id` (at most 36), `relative_path` (at most 96), `head_commit` (at most 64) only; the branch is verified but not stored | 8192 |
| unallocated headroom (no field is written here; reserved so that a later capability, such as the deferred shared environments, does not move the cap) | 2048 |
| `placement`, `admission` (`admission_scope`, `reservation`, identity snapshot), drain fields, `launch_policy` outcome, claim fields (kind, last sequence, digest), `cleanup_requested`, `handshake_started`, `phase` | 1536 |

The maxima sum to exactly 20480. Every envelope field the host writes has an entry in **new** `hostFieldBudget` (a host table introduced by this change) (a test fails when a field has no entry); `supported_state_versions` is capped at 16 entries inside the first row, and the strategy label of AC PREPARATION 001.3 is stored once as `preparation_strategy` (at most 64 bytes) inside the workspace descriptor row. A checkpoint's provider state replaces the recorded state and is measured against `state_budget_bytes` exactly like a provision response (`state_too_large` otherwise). The budget table lives in code (`hostFieldBudget`), and a unit test asserts that its sum is at most 20480. A relative path over 96 bytes or an `id` over 36 bytes in a provider response
is rejected with `state_too_large` before persisting. Default `state_budget_bytes` is 8192; the maximum 12288 plus the 20480 reserve equals the cap.
Every maximum in the table is counted on the marshaled, JSON-escaped form of the field, including key names and separators, because escaping inflates `&`, `<`, `>`, and control characters up to six times; the per-entry limits on `repositories` apply to the escaped value of `id`, `relative_path`, and `head_commit` (a value whose escaped form exceeds its limit is rejected with `state_too_large`). At 32 entries and the entry limits (36, 96, 64) the field is at most 7872 bytes with keys and fits its 8192 maximum. The state is measured on its JSON-escaped form for the same reason.

The code for every rejection is `state_too_large`. It is a host-originated failure code, not a member of the closed provider error set
(`invalid_config`, `denied`, `unsupported`, `not_found`, `expired`, `conflict`, `unavailable`, `rate_limited`, `cleanup_pending`): a provider cannot return
it, and the launch path reports it as the failure code. Cleanup after a rejection writes only the fixed-size terminal transition (phase and claim
fields, at most 256 bytes) onto the last persisted envelope, so a rejected write cannot recurse; if even that write fails, the record is retained as
`cleanup_pending`. A host-side write (placement, drain outcome, admission snapshot) that would exceed the cap despite the maxima is refused whole,
logged, and leaves the stored envelope unchanged; launch paths then run the cleanup above, and ownership writes return `conflict` to their caller.

The worst-case test builds an envelope with every field at its maximum and a 12288-byte state made of bytes that escape to the longest form, marshals
it, and asserts at most 32768 bytes; a second test does the same at the default budget. Whether a real provider's state fits is a vendor-repository check;
the fixture proves a representative 12 KiB state (D7).

## Deprecation and retirement

Marking `deprecated_in` makes install warn and name the replacement; the status read lists it with `retire_in`. The window is
two stable releases (AC 004.1), enforced by a registry test that rejects `retire_in` earlier than `deprecated_in` plus two
releases. Retirement moves the pair to `Retired`; a manifest requiring it fails with `capability_retired` and the release name.

## Data model

No new table and no DDL. The envelope field `negotiated_capabilities` is added and `state_version` is reused as the provider state version; decode tolerates
absence. The readability count scans `executors_running` rows whose `runtime` column is `plugin_remote` (the `executors_running` table has no executor-type column; the same predicate as `ListExecutorsRunningPluginRemote`), filters by the envelope's installation, includes every row not yet confirmed destroyed (rows in `creating`, pending placement, and retained phases count too), and reads the
envelope's `state_version`, falling back to the highest entry of `supported_state_versions`, and to 1 when both are absent (SQLite `json_extract(metadata, '$.plugin_executor.state_version')`, PostgreSQL `(metadata::jsonb -> 'plugin_executor' ->> 'state_version')`); no index is
added because the scan is bounded by active and retained plugin environments.

**Fence between activation and launch.** **New** `checkStateWritable` runs in the admission write that creates the envelope and re-reads the installation's current readable list. Both the admission
write and the activation write (which runs **new** `checkStateReadable` and switches the manifest) take the per-installation lock: on SQLite the immediate-mode writer, on PostgreSQL
`pg_advisory_xact_lock(hashtextextended('plugin_state_readability:'||installation_id, 0))`. Whichever commits first wins: an admission that commits first is counted and the upgrade is
rejected with the count; an activation that commits first makes the later admission fail its writability check with `capability_mismatch` and no allocation. A response that arrives
under a replaced dispatch generation is already rejected by the generation fence, so a version can never be recorded outside the active range.

## Interleavings and failing-first tests

| Interleaving | Required outcome | Tests (level) |
| --- | --- | --- |
| Upgrade activation racing a new launch that records an old state version (003.3) | the per-installation lock decides: admission first, so the upgrade is rejected with the count; activation first, so the launch fails `capability_mismatch` with no allocation; never an unreadable record | lifecycle unit with injected ordering; sqlite and PostgreSQL multi-connection |
| Plugin process restarts with a different implemented set (002.2) | provider unavailable; retained environments kept | fixture over the process boundary |
| Manifest declares version 2 of a name the host supports at 1 (002.1) | install rejected naming the range | validator table |
| Older provider returns no echo (002.4) | treated as declaring none; manifest requiring any rejected | lifecycle unit |
| Plugin restart, then a launch before any validation (002.3) | one profile validation runs first; a provider declaring nothing still launches | lifecycle unit |
| Two validations of one generation return different echoes (002.2) | `capability_mismatch` until the generation changes | lifecycle unit |
| Rollback to a plugin whose range excludes recorded state (003.3) | rejected before activation; working install active | upgrade unit |
| Manifest lowers `min_kandev_version` below a capability's release (002.6) | install rejected naming both | validator table |
| State above budget, or host fields plus state above the cap (003.7) | `state_too_large`, resource cleaned, nothing persisted; the cleanup write is fixed-size | lifecycle unit; combined worst-case test at the maximum budget and at the default |
| Host-side write over the cap (003.8) | refused whole, stored envelope unchanged | store unit |
| Retired pair removed from the list (004.3) | golden-file test fails | registry test |
| Record with only the existing `capabilities` and `state_version` keys (003.6) | reads as the empty negotiated set and keeps working | store decode test |
| Manifest declares both `state_schema` and `supported_state_versions` (003.2) | install rejected | validator table |
| Attach response with a raised state version (003.4) | rejected; recorded version unchanged | lifecycle unit |

All tests are written first and fail on current main (the field, the echo, and the registry do not exist). Remaining ACs are
validator and registry table tests plus the unchanged fixture conformance test.

## Observability

`executor_plugin_capability_rejected_total` (`capability`, `phase`: install, upgrade, enable, allocate, echo), `executor_plugin_state_too_large_total` (`source`: provider_state, host_field, host_write),
`executor_plugin_state_readability_blocked_total` (`action`: upgrade, rollback, retire). No installation or environment label.

## Security

Capability names and versions are non-secret. The echo is data from a trusted-binary plugin and is checked against the manifest,
never used to widen it. A provider cannot raise its own recorded set after admission.

## Related decisions

- [Remote executor plugin boundary](../../../decisions/2026-09-26-remote-executor-plugin-boundary.md)
- ADR for the owner decisions (work order 00).
