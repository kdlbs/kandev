---
id: "01-capability-registry-versioning"
title: "Capability registry, negotiation, and versioning (M7)"
status: pending
wave: 1
depends_on:
  - "00"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NATIVE-WORKER-001
  - REQ-EXECUTORS-PLUGIN-CAPABILITY-001
  - REQ-EXECUTORS-PLUGIN-CAPABILITY-002
  - REQ-EXECUTORS-PLUGIN-CAPABILITY-003
  - REQ-EXECUTORS-PLUGIN-CAPABILITY-004
acceptance_criteria:
  - AC-EXECUTORS-NATIVE-WORKER-001.1
  - AC-EXECUTORS-NATIVE-WORKER-001.2
  - AC-EXECUTORS-NATIVE-WORKER-001.3
  - AC-EXECUTORS-NATIVE-WORKER-001.4
  - AC-EXECUTORS-NATIVE-WORKER-001.5
  - AC-EXECUTORS-NATIVE-WORKER-001.6
  - AC-EXECUTORS-NATIVE-WORKER-001.7
  - AC-EXECUTORS-PLUGIN-CAPABILITY-001.1
  - AC-EXECUTORS-PLUGIN-CAPABILITY-001.2
  - AC-EXECUTORS-PLUGIN-CAPABILITY-001.3
  - AC-EXECUTORS-PLUGIN-CAPABILITY-001.4
  - AC-EXECUTORS-PLUGIN-CAPABILITY-001.5
  - AC-EXECUTORS-PLUGIN-CAPABILITY-002.1
  - AC-EXECUTORS-PLUGIN-CAPABILITY-002.2
  - AC-EXECUTORS-PLUGIN-CAPABILITY-002.3
  - AC-EXECUTORS-PLUGIN-CAPABILITY-002.4
  - AC-EXECUTORS-PLUGIN-CAPABILITY-002.5
  - AC-EXECUTORS-PLUGIN-CAPABILITY-003.1
  - AC-EXECUTORS-PLUGIN-CAPABILITY-003.2
  - AC-EXECUTORS-PLUGIN-CAPABILITY-003.3
  - AC-EXECUTORS-PLUGIN-CAPABILITY-003.4
  - AC-EXECUTORS-PLUGIN-CAPABILITY-003.5
  - AC-EXECUTORS-PLUGIN-CAPABILITY-003.6
  - AC-EXECUTORS-PLUGIN-CAPABILITY-004.1
  - AC-EXECUTORS-PLUGIN-CAPABILITY-004.2
  - AC-EXECUTORS-PLUGIN-CAPABILITY-004.3
  - AC-EXECUTORS-PLUGIN-CAPABILITY-004.4
  - AC-EXECUTORS-PLUGIN-CAPABILITY-004.5
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-capability-versioning.md
---

# Task 01: Capability registry, negotiation, and versioning (M7)

## Summary

Add the `hostcaps` registry, the `{name, version}` manifest syntax, dependency closure, the profile-validation echo, the allocation gate, the inventory snapshot, `state_schema`, the upgrade and rollback readability check, and the deprecation and retired-pair rules. Contract version stays 1.

## Size

M: ~550 source, ~900 test lines. Built first; every later order is a registry entry.

## In scope

- Registry package, manifest validator, host constant comparison at install, upgrade, and enable.
- Additive `host_capabilities` and `implemented_capabilities` fields; `capability_mismatch` availability.
- `checkCapabilityAgreement` in `validateLaunch`; `negotiated_capabilities` and `provider_state_version` in the envelope.
- `checkStateReadable` for upgrade, rollback, and retirement; append-only retired list with a golden-file test.

## Out of scope

- Any capability behavior itself (later orders); SDK module publication.

## Acceptance

- A manifest naming an unknown, duplicate, out-of-range, or dependency-missing capability is rejected before activation with the working install kept.
- An upgrade or rollback that strands recorded state is rejected with a count; a non-declaring provider is byte-for-byte unchanged.

## Failing-first tests

- Validator table: unknown, duplicate, zero or non-integer version, version outside range, missing dependency (fails: syntax absent).
- Provider echo differs from the manifest: unavailable with `capability_mismatch`; no echo: treated as declaring none.
- Allocation gate runs before the first provider call on first launch, delayed launch, and resume (call-site enumeration test).
- Upgrade activation racing a launch that records an old state version (injected ordering; sqlite and PostgreSQL).
- Retired pair removed or reused: golden-file test fails. A record with only the old `required_capabilities` key decodes as version 1.
- Existing fixture conformance test passes unmodified.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/plugins/... ./pkg/pluginsdk ./cmd/plugin-fixture -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

## Likely files and risks

- `apps/backend/internal/plugins/hostcaps (new)`
- `apps/backend/internal/plugins/manifest/executor_provider.go`
- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin.go`
- Risk: concurrency on both dialects; add the PostgreSQL race run (`KANDEV_TEST_POSTGRES_DSN`) before marking done.
