---
id: "02-provider-protocol"
title: "Add compatible major 2 provider contracts"
status: done
wave: 2
depends_on: 
  - "01-sdk-runtime"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-SANDBOX-002
acceptance_criteria:
  - AC-PLUGINS-SANDBOX-002.1
  - AC-PLUGINS-SANDBOX-002.2
  - AC-PLUGINS-SANDBOX-002.3
  - AC-PLUGINS-SANDBOX-002.5
system_design:
  - ../../specs/plugins/system-design/sandbox-provider-v2.md
---

# Task 02: Add compatible major 2 provider contracts

## Summary

Add explicit major 2 dispatch, typed operation/evidence shapes, and bounded audit chunks while preserving major 1 wire behavior.

## In scope

- Manifest and standalone SDK/protobuf definitions, generated code, package binding, strict versioned validators, and second-provider fixture.
- Core-owned callback/route identity and public/private field boundaries; no production capability activation.

## Out of scope

- No Nono process implementation or UI authority decisions.

## Acceptance

- Old-provider/new-host and new-provider/old-host tests demonstrate preservation or explicit refusal, with no downgrade.
- Chunk substitution, unknown fields, false channel binding, cancellation, and untrusted verification results fail closed.

## Verification

Run from the host repository root. New test files/scripts named below are implementation deliverables. A command that runs zero relevant tests or skips real-runtime coverage does not satisfy this order.

```bash
set -e
make -C pluginsdk proto
(cd pluginsdk && go test -race ./...)
(cd apps/backend && umask 022 && go test -trimpath -race ./internal/plugins/... ./cmd/plugin-fixture/...)
# After intentional generated edits, capture and compare a second regeneration.
git diff --binary -- pluginsdk/proto > /tmp/nono-proto-before.patch
sha256sum pluginsdk/proto/kandev/plugin/v1/*.pb.go > /tmp/nono-proto-before.sha256
make -C pluginsdk proto
git diff --binary -- pluginsdk/proto > /tmp/nono-proto-after.patch
sha256sum pluginsdk/proto/kandev/plugin/v1/*.pb.go > /tmp/nono-proto-after.sha256
cmp /tmp/nono-proto-before.patch /tmp/nono-proto-after.patch
cmp /tmp/nono-proto-before.sha256 /tmp/nono-proto-after.sha256
```

## Files likely touched

- `pluginsdk/sandbox_provider.go`
- `pluginsdk/proto/kandev/plugin/v1/plugin.proto`
- `apps/backend/internal/plugins/manifest/sandbox_provider.go`
- `apps/backend/internal/plugins/sandbox_provider_dispatch.go`
- `apps/backend/internal/plugins/sandbox_provider_v2_test.go (new)`
- `apps/backend/cmd/plugin-fixture/`

## Dependencies

01-sdk-runtime.

## Risks

The proto regeneration command must compare against a captured generated baseline after intentional edits; the final diff check cannot reject the intended new schema.

## Parallelism

`sequential`. Keep work in the primary session.

## Inputs

- Linked requirement IDs and system designs above.
- Existing sandbox policy, provider, and execution UI tests; upstream source evidence in [the plan](plan.md#source-evidence).

## Results

Completed 2026-11-02. The existing six RPCs now carry explicit major 2 payloads. Major 1 rejects known extension fields as well as unknown wire fields. Request/reply versions cannot downgrade. Installation bindings and private attempt snapshots retain protocol identity; absent historical majors retain major 1.

Major 2 accepts unchanged schema 1 policies or explicit schema 2 policies, never mixed shapes. This separates transport upgrade from an operator policy migration. New operations and runtime-service actors use typed fields. Runtime facts and audit trust remain separate from current confinement evidence.

RED receipts cover missing schema fields, rejected major 2 manifests/contexts, major 1 field smuggling, service/channel mismatch, unsupported runtime observations, chunk substitution, endpoint port zero, lost protocol persistence, schema 1 compatibility, and escaped/unbound status sources. GREEN tests use the packaged generic fixture with two provider identities across all six RPCs. No Nono-specific host branch was added.

- `go test -trimpath -race ./...` in `pluginsdk`: passed.
- Protobuf regeneration: deterministic, including SHA-256 comparison of newly generated untracked files. Receipt: `/tmp/nono-proto-before.sha256` and `/tmp/nono-proto-after.sha256`.
- Full `internal/plugins/...` and `cmd/plugin-fixture/...` race suites passed with a process-local `umask 022`, required by the existing package-mode assertion (`/tmp/nono-v2-plugin-final.log`).
- Affected sandbox policy/runtime race suites passed separately with private fixture mask `077` (`/tmp/nono-v2-private-race.log`). Combining these fixtures under mask 022 rejects their runtime file permissions; no production permission rule was weakened.
- The final major 2 regression subset passed after status-source binding was added (`/tmp/nono-v2-last-source-tests.log`).
- Scoped Go lint passed with zero issues. A verified task-local golangci-lint 2.12.2 binary replaced the unavailable former Nix path. Refactoring addressed complexity and named protocol values without changing permission meanings.
- Public docs: 63 validator tests and 54 published pages passed. Specification catalog, structure, and whitespace checks passed.

The new capability is not enabled in the production Nono manifest in this order. No live-attestation or new real-runtime approval claim is made. Task 03 owns the next integration slice.
