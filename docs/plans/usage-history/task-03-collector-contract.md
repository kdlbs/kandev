---
id: "03-collector-contract"
title: "Normalize collector identities and indexed selection"
status: in_progress
wave: 3
depends_on: ["02-lifecycle-migration"]
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-002
  - REQ-COSTS-USAGE-HISTORY-003
  - REQ-COSTS-USAGE-HISTORY-004
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-002.1
  - AC-COSTS-USAGE-HISTORY-002.3
  - AC-COSTS-USAGE-HISTORY-002.5
  - AC-COSTS-USAGE-HISTORY-003.1
  - AC-COSTS-USAGE-HISTORY-003.2
  - AC-COSTS-USAGE-HISTORY-003.3
  - AC-COSTS-USAGE-HISTORY-003.4
  - AC-COSTS-USAGE-HISTORY-003.5
  - AC-COSTS-USAGE-HISTORY-003.7
  - AC-COSTS-USAGE-HISTORY-004.1
  - AC-COSTS-USAGE-HISTORY-004.2
  - AC-COSTS-USAGE-HISTORY-004.3
  - AC-COSTS-USAGE-HISTORY-004.4
  - AC-COSTS-USAGE-HISTORY-004.5
  - AC-COSTS-USAGE-HISTORY-004.6
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 03: Normalize collector identities and indexed selection

## Summary

Expose a versioned measurement contract independent of collector tools.
Replace workspace-wide overlap loops with bounded identity/coverage matching and deterministic source selection.

## In scope

- Separate authenticated collector provenance, core-approved underlying work identity, provider, model, client, and serving platform.
- Add subject-binding admission and authorized retained corrections after live task deletion.
- Preserve nullable fields, cost/token semantics, integer precision, source versions, and revision/digest concurrency results.
- Support explicit same-source supersession for model/provider corrections. Missing partial-report entries never imply deletion or zero.
- Add immutable snapshot/keyset collection-target discovery, including concurrent addition/deletion/resume tests.
- Index scope/work/model/provider/semantic partitions and coverage intervals. Use ordered matching inside affected partitions.
- Resolve proven work equivalence before dimension partitions become additive, including unknown-to-known provider/model/semantic transitions.
- Use immutable observation item IDs or explicit complete-set supersession/tombstones. Invalidate old and new partitions after dimension corrections.
- Select tokens and cost independently only for equivalent coverage. Preserve ambiguous overlap and separate lifetime/day representations.
- Keep existing DTOs working through a compatibility mapper. Regenerate protobufs and update SDK/Host documentation together.
- Add a replacement-collector fixture without integrating another real external tool.

## Out of scope

- Plugin process changes, global query aggregation, summary rebuilds, and arbitrary source-priority claims from plugins.

## Acceptance

- Different collectors reporting the same proven work yield one contribution. Unproven aliases remain incomplete and do not silently merge.
- Concurrent first inserts and later corrections return accurate accepted rows. Stale/conflicting writes cannot roll back a newer value.
- Forged tenant/actor/binding claims fail, including tombstoned work. Older supported clients keep their wire behavior.

## Verification

```bash
make -C apps/backend proto
(cd apps/backend && go test -race -trimpath ./internal/analytics/... ./internal/plugins/... -run 'TestUsageHistory|TestSessionUsage' -count=1)
(cd pluginsdk && go test -race ./... -count=1)
(cd pluginsdk && go vet ./...)
(cd pluginsdk && golangci-lint run --config ../apps/backend/.golangci.yml ./...)
node scripts/pluginsdk/check-proto-generation.mjs
node scripts/pluginsdk/consumer-smoke.mjs --mode local --backend-only
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test DSN}" && go test -trimpath ./internal/analytics/... ./internal/plugins/... -run 'TestUsageHistoryPostgres' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/analytics/{models/usage.go,repository/interface.go,service/service.go}`
- `apps/backend/internal/analytics/repository/sqlite/usage.go` and new selection/index tests
- `apps/backend/internal/plugins/{host_data.go,host_data_queries.go}` and Host contract tests
- `pluginsdk/{usage.go,host_usage.go,host.go}` (the extracted Go SDK module)
- `pluginsdk/proto/kandev/plugin/v1/plugin.proto` and its existing generated output
- `docs/public/plugins-authoring.md`, `docs/plans/plugins/GRPC-CONTRACT.md`

## Dependencies

Task 02. Retained bindings and lifecycle authorization are prerequisites for post-deletion corrections.

## Risks

Different tools can name one provider thread differently. Unsupported aliases must remain unknown rather than guessed.
Do not use model prefixes or collector IDs as proof of underlying work identity.
Contract additions must not change the existing proto numbers or numeric semantics.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/costs/requirements/usage-history.md), REQ-002/003/004 and listed ACs.
- [Design](../../specs/costs/system-design/usage-history.md), Measurement contract, Source identity and indexed selection.
- Existing capability gates, revision tests, native observation identities, and [Host ADR](../../decisions/0043-plugin-host-data-api.md).

## Results

Implementation and test handoff accepted. Full semantic review follows the remaining worker implementations and tests.

- Added the optional root SDK v1 usage-history extension, Host RPCs, protobuf output, transport tests, and public/GRPC documentation. Existing DTO fields and wire numbers remain compatible.
- Added immutable collector item identities, verified payload digests, revision CAS receipts, explicit same-source supersession/tombstones, and atomic bounded complete-set revisions. Partial absence never retires an item.
- Added immutable snapshot/keyset discovery with retained binding versions and collector grants. Core supplies scope and actor evidence. Issued bindings authorize corrections after deletion; forged bindings and transferred scopes fail closed.
- Added nullable trusted session/coverage dates and canonical `PageInfo` projection status, generation, and pending fields. Unknown dates and token semantics remain absent. Explicit source totals retain `int64` precision. Collector message counts do not become Kandev turns.
- Added indexed selection by retained scope and work before mutable dimensions. Facts have stable keys, nullable measurements, coverage kinds, and independent token/cost provenance and actors. Native priority requires a core-native source. Replacement-collector fixtures cover equivalent work, dimension corrections, currencies, overlap, and partial dated imports with lifetime totals.
- Retained writes now feed the existing canonical usage reader. Selection visits at most 201 candidates per bounded lookup. An oversized indivisible equivalence group returns preparing state. No projection generation is invented.
- Connected Task02 backfill and preservation to selection indexing. Final verification found and fixed a dated legacy measurements row that incorrectly selected the buckets table during index catch-up. The existing migration test still requires completion within ten pages. The admission test now requires both distinct semantics partitions to be dirty.
- Completed behavior-preserving lint cleanup with predecessor helpers preserved. Registered two exact SQLite-only migration boundaries in the SQLguard registry, as required by the [dialect safety design](../../specs/platform/system-design/postgres-domain-store-parity.md). The separate PostgreSQL migration path passed real database tests.

### Task04 integration seams

- `MutateUsageObservationsTx` in `usage_history_contract.go` accepts the caller's transaction and returns mutations with old/new measurements, payloads, and affected partition IDs.
- `recordUsageMutationTx` in `usage_history_mutations.go` is the shared transaction hook for native, legacy, and versioned observations. Mutation and dirty invalidations commit together. Task04 owns the ordered commit log and replay sequence.
- `SelectUsagePartitionTx` in `usage_history_selection.go` returns bounded canonical facts and a continuation cursor. `ResolveUsagePartitionsTx` expands legacy dirty keys into work groups. `UsageSourceUpperBoundsTx` captures source bounds for Task04 to pair with its replay sequence.
- `ListCanonicalUsagePage` and the service/Host page seam carry projection state, including on empty pages. Task04/05 own committed generations and projection-backed reporting.
- Public selector/source-bound authorization uses the workspace's current organization. Task04 owns its private fenced path for captured scopes after transfer. No public bypass flag was added.

### Verification receipts

Go commands used `GOMAXPROCS=2`, `-p 1`, and `-trimpath`; race suites ran sequentially. PostgreSQL commands silently sourced the isolated PostgreSQL18 environment.
The four environment-gated PostgreSQL skips in the SQLite run were followed by four actual PostgreSQL passes with no skips.

All logs below use `/tmp/usage-history-task03-final-<name>.log`, except the separately named SDK lint receipt.

| Check | Result | Log name |
| --- | --- | --- |
| Approved changed-package backend lint, including all history helpers and task lifecycle package | Zero issues | `backend-lint` |
| Analytics, plugins, and task repository `TestUsageHistory\|TestSessionUsage` race suites | 61 top-level passes | `backend-race` |
| Full root SDK race suite | 118 top-level passes; no skips | `sdk-race` |
| Full root SDK vet | Passed | `sdk-vet` |
| Full root SDK lint | Zero issues | `/tmp/usage-history-task03-sdk-lint.log` |
| Deterministic generated-byte comparison and backend proto wrapper | Passed | `proto-generation`, `proto-wrapper` |
| SDK artifact script tests and local Go consumer smoke | Passed | `sdk-artifact-tests`, `consumer-smoke` |
| Real PostgreSQL race cases | Four passes; no skips | `postgres-race` |
| SQL portability and SQLite/PostgreSQL persistence conformance, including previous-stable upgrades | Passed | `sqlguard`, `store-conformance` |
| Documentation, specification, and whitespace checks | Passed | `docs`, `specs`, `diff-check` |

Meaningful cases include `TestUsageHistoryPostgresConcurrentIndependentWriters`, `TestUsageHistoryPostgresNativeAdmissionAndRetention`, `TestUsageHistoryPostgresTaskDeletePreservesLegacySources`, and `TestUsageHistoryPostgresCapturedOrgErasureAfterWorkspaceTransfer`.
Host wire cases include `TestUsageHistoryHostRetainedWireAdmissionCanonicalAndDeletion` and `TestUsageHistoryHostEmptyCanonicalPagePreservesProjectionState`.

The initial failing backend receipt remains in `/tmp/usage-history-task03-final-backend-race.failed-1.log`; the migration diagnosis and passing regression remain in `/tmp/usage-history-task03-migration-{failure,fix}.log`.
No released SDK/Host floor was claimed. `pluginsdk/sdk-release.json` retains null Host release and minimum-version fields. No projection implementation, sibling plugin change, staging, commit, or push was performed by this worker.
