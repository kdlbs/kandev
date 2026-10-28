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
[native worker design](remote-executor-plugin-native-workers.md), the [shared, admission, and placement design](remote-executor-plugin-shared-admission-placement.md),
and the [operations and migration design](remote-executor-plugin-operations-migration.md) is a registry entry. Names
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
range, dependency closure (`provider_maintenance` needs `provider_operations`; `workspace_sharing` needs
`workspace_descriptor`; `pending_placement` needs `destroy_checkpoints`; `launch_policy` needs `workspace_descriptor`;
`preparation_hints` needs `repository_intent`; `typed_profile_values` and `secret_references` need none; `provision_refusal` needs `destroy_checkpoints`). `state_schema` is `{current, min_readable}`, absent meaning `1/1`. `min_kandev_version` must be at least the highest `introduced_in` release among the declared capabilities (AC 002.6); the existing generic minimum-version check then makes an older host refuse the package with a clear message. Cross-field
rules from the native design (preparation mode, API URL mode, destroy timeout) stay and consult the same registry.

## Negotiation points

1. **Install, upgrade, enable:** the validator output plus the registry check, before activation (AC 002.1). A failure leaves
   the working installation active, reusing the upgrade-rollback path from the base design.
2. **Profile validation:** `ValidateExecutorProfile` gains additive request field `host_capabilities` (the registry rows) and
   response field `implemented_capabilities`. The host stores the last accepted echo in memory per dispatch generation. A
   mismatch marks the provider `unavailable` with reason `capability_mismatch` (AC 002.2). An older provider returns no echo,
   so it is treated as declaring none (AC 002.4).
3. **Allocation gate:** `checkCapabilityAgreement` runs first in `validateLaunch`, before the admission hook and before any
   provider call, on first launch, delayed launch, and resume. It is a pure function over manifest, registry, and echo
   (AC 002.3, 002.5).

## Snapshot

At admission the host writes `negotiated_capabilities` (sorted `name@version` strings) and `provider_state_version` into the
`plugin_executor` envelope, replacing the earlier `required_capabilities` list from the native design (same position, richer
content; a row with only the old key reads as version 1 for each name, AC 003.6). Recovery, attach, destroy, lease
validation, and response validation read the snapshot. A host that no longer supports a recorded version maps the
provider to `unavailable` for that environment through the existing availability path (AC 003.5).

## State schema versions

The provider returns `state_schema_version` beside `state_json` in every accepted provision, recovery, attach, and checkpoint
response; the host records it and rejects a decrease within one generation (AC 003.4). The host never reads the
state body. Upgrade and rollback run `checkStateReadable`: one indexed query counts non-terminal plugin environments of the
installation whose recorded version is outside the new `[min_readable, current]`; a non-zero count rejects the change before
activation and reports only the count (AC 003.3). The same query blocks retirement of a version that retained environments
record (AC 004.4).

## State budget

The 32 KiB envelope holds host fields (capability snapshot, owners, placement, repositories, descriptor) and provider state together. The
manifest `state_budget_bytes` (default 16 KiB, at most 24 KiB) reserves the rest for host fields. The host measures the state body on
every accepted response, rejects an over-budget one as `state_too_large` before it writes (AC 003.7), and records the largest size per
installation for the status read (AC 003.8). A combined worst-case test (16 owners, 32 repositories, pending placement, maximum
snapshot, 16 KiB state) proves the host fields never push a valid state over the cap. Whether a real provider's state fits is a
vendor-repository check; the fixture proves a representative 12 KiB state (D7).

## Deprecation and retirement

Marking `deprecated_in` makes install warn and name the replacement; the status read lists it with `retire_in`. The window is
two stable releases (AC 004.1), enforced by a registry test that rejects `retire_in` earlier than `deprecated_in` plus two
releases. Retirement moves the pair to `Retired`; a manifest requiring it fails with `capability_retired` and the release name.

## Data model

No new table and no DDL. Envelope fields `negotiated_capabilities` and `provider_state_version` are added; decode tolerates
their absence. The readability count uses the existing `executors_running` rows filtered by installation and the envelope's
state version. SQLite and PostgreSQL run the same query; the count is advisory at the moment of the check, and a launch that
records an old version between the check and activation is caught by the allocation gate (AC 002.3).

## Interleavings and failing-first tests

| Interleaving | Required outcome | Tests (level) |
| --- | --- | --- |
| Upgrade activation racing a new launch that records an old state version (003.3) | launch fails the gate or the upgrade is rejected; never an unreadable record | lifecycle unit with injected ordering; sqlite and PostgreSQL |
| Plugin process restarts with a different implemented set (002.2) | provider unavailable; retained environments kept | fixture over the process boundary |
| Manifest declares version 2 of a name the host supports at 1 (002.1) | install rejected naming the range | validator table |
| Older provider returns no echo (002.4) | treated as declaring none; manifest requiring any rejected | lifecycle unit |
| Rollback to a plugin whose range excludes recorded state (003.3) | rejected before activation; working install active | upgrade unit |
| Manifest lowers `min_kandev_version` below a capability's release (002.6) | install rejected naming both | validator table |
| State above budget, or host fields plus state above the cap (003.7) | `state_too_large`, resource cleaned, nothing persisted | lifecycle unit; combined worst-case test |
| Retired pair removed from the list (004.3) | golden-file test fails | registry test |
| Record with only the old `required_capabilities` key (003.6) | reads as version 1 and keeps working | store decode test |

All tests are written first and fail on current main (the field, the echo, and the registry do not exist). Remaining ACs are
validator and registry table tests plus the unchanged fixture conformance test.

## Observability

`executor_plugin_capability_rejected_total` (`capability`, `phase`: install, upgrade, enable, allocate, echo),
`executor_plugin_state_readability_blocked_total` (`action`: upgrade, rollback, retire). No installation or environment label.

## Security

Capability names and versions are non-secret. The echo is data from a trusted-binary plugin and is checked against the manifest,
never used to widen it. A provider cannot raise its own recorded set after admission.

## Related decisions

- [Remote executor plugin boundary](../../../decisions/2026-09-26-remote-executor-plugin-boundary.md)
- ADR for the owner decisions (work order 00).
