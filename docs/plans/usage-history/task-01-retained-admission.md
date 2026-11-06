---
id: "01-retained-admission"
title: "Capture retained usage and ownership"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-002
  - REQ-COSTS-USAGE-HISTORY-003
  - REQ-COSTS-USAGE-HISTORY-007
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-002.1
  - AC-COSTS-USAGE-HISTORY-002.2
  - AC-COSTS-USAGE-HISTORY-002.3
  - AC-COSTS-USAGE-HISTORY-002.4
  - AC-COSTS-USAGE-HISTORY-003.1
  - AC-COSTS-USAGE-HISTORY-003.2
  - AC-COSTS-USAGE-HISTORY-003.3
  - AC-COSTS-USAGE-HISTORY-003.4
  - AC-COSTS-USAGE-HISTORY-003.5
  - AC-COSTS-USAGE-HISTORY-003.6
  - AC-COSTS-USAGE-HISTORY-003.7
  - AC-COSTS-USAGE-HISTORY-007.3
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 01: Capture retained usage and ownership

## Summary

Add retained subjects and scope ownership without changing existing read contracts.
Commit a retained native observation inside the existing accepted-event transaction.

## In scope

- Add portable scope/subject schema and additive retained bindings to the current measurement tables.
- Capture org/workspace and actor evidence at execution/turn admission. Preserve unknown/mixed evidence and single-user identity.
- Persist immutable turn/execution binding IDs through admission, usage publication, and writer lookup. Cover shared sessions, replay, and child attribution.
- Normalize native and existing external records through one internal accounting contract with checked integer and category semantics.
- Add a narrow transactional retention capability without a task/analytics import cycle or another native writer.
- Preserve native insert, session-rollup, duplicate, FK-retry, transient-retry, and shutdown behavior.
- Register durable dirty-partition work in the observation transaction for later projection.

## Out of scope

- Legacy backfill/deletion cutover, public Host extensions, statistics query cutover, and rendered UI.

## Acceptance

- An accepted native transaction contains its retained observation and correct trusted ownership, or rolls back atomically.
- Old callers remain compatible. Counts, units, nullable fields, provenance, and integer transport never change meaning silently.
- Scope/actor tests cover Auth off, Auth on, MultiTenancy, unknown/mixed actors, and forged collector identities.

## Verification

Run from the Kandev root. New `TestUsageHistory*` cases must exist before these commands count as evidence.

```bash
(cd apps/backend && go test -race -trimpath ./internal/analytics/... ./internal/task/usage/... -run 'TestUsageHistory|TestSessionUsage' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/task/repository/sqlite ./internal/backendapp -run 'TestUsageHistory|TestCreateTaskUsageEvent|TestTaskUsage' -count=1)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test DSN}" && go test -trimpath ./internal/analytics/... ./internal/task/repository/sqlite -run 'TestUsageHistoryPostgres' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/analytics/{models/usage.go,repository/interface.go,service/service.go}`
- `apps/backend/internal/analytics/repository/sqlite/{usage_schema.go,usage.go}` and new `usage_history_*` files/tests
- `apps/backend/internal/task/{usage,models}` and `repository/sqlite/usage_events.go`
- `apps/backend/internal/backendapp/task_usage_wiring.go`
- Existing execution/turn admission paths that carry `ExecutorActorUserID` and authenticated identity

## Dependencies

None. Use the accepted requirements and current source baseline from the plan.

## Risks

The native writer already owns transaction retries and rollup atomicity. A post-commit callback cannot be its only retained-data path.
Do not treat a session's current actor as proof for every earlier turn.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/costs/requirements/usage-history.md), REQ-002/003/007 and listed ACs.
- [Design](../../specs/costs/system-design/usage-history.md), Retained identity, Ownership, Measurement contract, Native admission.
- [Plan](plan.md), compatibility matrix and existing writer/ledger references.

## Results

Implementation/test handoff accepted by the coordinator. Integrated semantic review remains a package-level gate after all six workers finish.

### Verification

- RED confirmed before implementation: `TestUsageHistoryNativeAtomic` failed with retained native observation count `0`; `TestUsageHistoryBindingSchema` found `0` retained binding tables. Logs: `/tmp/usage-history-agent1-red.log`, `/tmp/usage-history-agent1-binding-red.log`.
- PASS (exit 0): `cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath ./internal/analytics/... ./internal/task/usage/... -run 'TestUsageHistory|TestSessionUsage' -count=1`. Analytics repository tests cover trusted external scope/actor clearing, normalized values, SQLite migration, and retained-history deletion behavior. `internal/task/usage` and analytics handlers/service reported no tests selected by this regex; writer/binding propagation is covered by the focused check below. Log: `/tmp/usage-history-agent1-exact-race-analytics-final.log`.
- PASS (exit 0): with `/tmp/usage-history-pr3660-postgres.env` sourced silently, `cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath ./internal/task/repository/sqlite ./internal/backendapp -run 'TestUsageHistory|TestCreateTaskUsageEvent|TestTaskUsage' -count=1`. Task repository tests cover admission, unknown/shared actor, native atomic/rollback/duplicate, task usage compatibility, and PostgreSQL retention; `internal/backendapp` reported no tests selected by this regex. Log: `/tmp/usage-history-agent1-exact-race-task-final.log`.
- PASS (exit 0, no skip): with the PostgreSQL env sourced and `KANDEV_TEST_POSTGRES_DSN` required, `cd apps/backend && GOMAXPROCS=2 go test -p 1 -trimpath -v ./internal/analytics/... ./internal/task/repository/sqlite -run 'TestUsageHistoryPostgres' -count=1`. `TestUsageHistoryPostgresNativeAdmissionAndRetention` passed against the isolated PostgreSQL 18 schema, including legacy-FK migration and task-deletion survival. Log: `/tmp/usage-history-agent1-exact-postgres-final.log`.
- PASS (exit 0): `cd apps/backend && GOMAXPROCS=2 go test -p 1 -trimpath ./internal/task/usage ./internal/orchestrator -run 'Test(BuildRow_CarriesAdmittedUsageBinding|DecodePayload_ValidEvent_PopulatesAllFields|PublishNativeUsageObservationUsesStableProviderIdentity|PublishPromptUsage_RepublishedCompletionReusesUsageEventID)$' -count=1`. Log: `/tmp/usage-history-agent1-wire-publish-final.log`.
- PASS (exit 0): `python3 scripts/list-docs.py validate` (`Validated 417 decisions and 1828 specifications.`), `python3 scripts/lint-spec-files.py --all` (`All specification files passed.`), and `git diff --check`.

The first analytics race run found an old test that expected task/session deletion to cascade retained rows. It now asserts the accepted retained-history policy and durable subject instead of that superseded behavior; the final race run passes. Go checks used `GOMAXPROCS=2` and `-p 1` to limit concurrent compilation.

An additional execution-binding immutability regression passed its focused check after those results.
A redundant final broad race rerun was interrupted by the coordinator handoff and is not claimed as passed.
Task 02's race run includes that predecessor regression again on the integrated code.

### Handoff

Task 02 can use `internal/common/usagehistory.Retainer` (`AdmitBindingTx`, `RetainNativeUsageTx`) and the stable `usage_scopes`, `usage_subjects`, `usage_execution_bindings`, `usage_dirty_partitions`, and `session_usage_buckets` schema. Task 02 still owns bounded legacy backfill, lifecycle deletion fencing, live-link detachment, and erasure registration. No transaction-seam blocker remains. Public collector APIs, projection summaries, SQL/UI cutover, and plugin adoption remain with their later work orders.

### Predecessor lint cleanup (2026-10-29)

Extracted immutable binding comparison/read helpers, native normalization/persistence helpers, and schema upgrade helpers from the three Worker1-owned files. Added `usage_history_binding_admission.go`, `usage_history_native_admission.go`, and `usage_schema_helpers.go`. Preserved the current Task02 incarnation/erasure/live-link hooks and Task03 identity/index/mutation hooks and their order; reused the existing shared cost-source and driver constants. No feature changes, new tests, or lint suppressions were added.

`gofmt` on the six owned Go files and scoped `git diff --check` passed (exit 0). Static comparison against the cleanup snapshot confirmed unchanged ordered bootstrap/additive SQL and retained scope, dirty-partition, and digest helpers. No compile, lint, race, or PostgreSQL run was started for this cleanup. Combined changed-file lint and targeted race/real PostgreSQL verification remain pending with Worker3 after all writers freeze.


### Native accounting E/H integration (2026-10-29)

Integrated the nine explicitly granted caller files with the six new accounting helper/test files. Native response events no longer invent an additive turn count. Canonical mapping publishes an explicit native reasoning-output subset when raw thought is absent/zero, preserves ordinary thought, and leaves unsupported positive thought/subset combinations unavailable. Raw accepted counters, native digests, authoritative totals, ledger events, rollups, and non-native Host turns are preserved. Gob-persisted `NativeAccountingVersion` makes selection/staging/read normalization idempotent. Bounded source repair uses captured scopes, erasure/publication locks, ordinary before/after mutations, and a durable per-row version; old SQL values remain unavailable until corrected publication. Worker3's shared attribution and versioned work/source repair hooks were preserved.

Focused verification from `apps/backend`, with the supplied PostgreSQL env sourced silently and required before both test commands:

- `GOMAXPROCS=1 go test -p 1 -trimpath -v ./internal/analytics/repository/sqlite -run '^TestUsageHistoryNativeAccounting' -count=1`: PASS, exit 0; all 13 tests including two real PostgreSQL cases ran with no skips. Log `/tmp/usage-history-agent1-EH-integrated-normal.log`.
- `GOMAXPROCS=1 go test -p 1 -race -trimpath -v ./internal/analytics/repository/sqlite -run '^TestUsageHistoryNativeAccounting' -count=1`: PASS, exit 0; all 13 tests, no skips or race reports. Log `/tmp/usage-history-agent1-EH-integrated-race.log`.
- The focused tests cover fresh admission/report JSON, repeated canonical reads, same-root multiple responses/distinct-root/missing-root cases, legacy published guards before and after repair/publication, mixed token/cost provenance, unknown versus zero reasoning, 206-row repair with a 200-row cap, cancellation/reopen/resume, rollback, schema replay, captured-org erasure, real projection/rebuild, duplicate replay, and task-deletion preservation.
- `GOMAXPROCS=1 go vet -p 1 -trimpath ./internal/analytics/models ./internal/analytics/repository/sqlite`: PASS, exit 0; `/tmp/usage-history-agent1-EH-vet.log`.
- `GOMAXPROCS=1 GOFLAGS="${GOFLAGS:+$GOFLAGS }-p=1 -trimpath" xargs go run ./cmd/sqlguard -exemptions /tmp/usage-history-agent1-EH-sqlguard-exemptions.json < /tmp/usage-history-agent1-EH-sqlguard-files.txt`: PASS, exit 0, all 11 granted production files, zero exemptions; `/tmp/usage-history-agent1-EH-sqlguard-scoped.log`. The temporary registry contains only existing exemptions matching scanned files (none); no exemption was added. The first scoped invocation with the global registry exited 1 because that registry requires out-of-scope exemptions to be used.
- From repo root, `GOMAXPROCS=1 GOFLAGS="${GOFLAGS:+$GOFLAGS }-p=1 -trimpath" xargs bash scripts/lint-go-changed < /tmp/usage-history-agent1-EH-owned-files.txt`: wrapper exit 123 (lint exit 1), seven findings, all outside the write grant. No finding was reported in the 15 granted Go files. Parent follow-up: cyclop in `usage_history.go:RetainNativeUsageTx`, `usage_history_contract.go:upsertObservationTx`, `usage_history_contract_schema.go:ensureUsageContractSchema`, `usage_history_legacy_scope.go:provenLegacyNativeScopeTx`, and `usage_history_targets.go:ListUsageCollectionTargets`; nestif in `provenLegacyNativeScopeTx`; unused `usage_history_index.go:backfillSelectionRowTx`. Log `/tmp/usage-history-agent1-EH-scoped-lint.log`. No suppressions or limit changes were made.
- `xargs gofmt -l < /tmp/usage-history-agent1-EH-owned-files.txt`: exit 0, no paths. Direct SQL version probe confirmed `18.6 (Debian 18.6-1.pgdg13+2)`; `/tmp/usage-history-agent1-EH-postgres-version.log`.

All checks used inherited caches and one Go process/build worker. No broad suite, browser check, commit, stage, or push was performed. All nine caller files were released early after normal greens and lint, and remained byte-identical through race/vet/guard verification. Final source freeze hashes: `/tmp/usage-history-agent1-EH-final-freeze.sha256`; structured receipt: `/tmp/usage-history-agent1-EH-final-receipt.json`; integration delta against the pre-integration snapshot: `/tmp/usage-history-agent1-EH-integration.diff`. All 15 granted Go files are frozen/released. Coordinator owns status and final review; outside-grant lint cleanup remains with the parent.
