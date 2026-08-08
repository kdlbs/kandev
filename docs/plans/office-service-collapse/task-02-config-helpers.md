---
id: "02-config-helpers"
title: "Delete the facade's config sync/export/read mirror"
status: pending
wave: 2
depends_on: ["01-duplication-detector"]
plan: "plan.md"
spec: none
parallel-safe: true
---

# Task 02: Config Sync / Export / Read Mirror

The largest single win and the lowest behavioral risk: 14 of the 40 identical
groups are here, and every one is byte-identical after receiver normalization.
`config_import.go` is deliberately **excluded** — its `apply*`/`preview*`
functions carry the D1/D2 drift and are task 03.

## Scope

Delete from `internal/office/service`:

- `config_sync.go` (134) — `ScanFilesystem` `parseErrorsFromLoader`
  `IncomingDiff` `OutgoingDiff` `ApplyIncoming` `ApplyOutgoing`
- `config_sync_helpers.go` (184) — `deleteRowsMissingFromBundle` `diffBundles`
  and the `deleteMissing*` / `appendDeletions` family
- `config_sync_util.go` (160) — `missingNames` `writeBundleEntities`
- `config_export.go` (244) — `ExportBundle` `bundleToZip` `writeYAMLFile`
  `export{Agents,Skills,Routines,Projects}`
- `config_read.go` (118) — `List{Agents,Skills,Projects,Routines}FromConfig`,
  `Get{Agent,Project,Routine}FromConfig`. **Keep `GetAgentFromConfig` for now** —
  `scheduler` calls it at three sites; task 05 repoints them. Leave it in a
  trimmed `config_read.go` and note the TODO pointing at task 05.

`office/config` and `office/agents` already own equivalents. The `export*` and
`deleteMissing*`/`appendDeletions` pairs read as drifted at 0.918–0.975 but are
cosmetic — the facade calls its own one-line wrapper `ListXFromConfig`, `config`
calls `repo.ListX` directly, and `service/config_read.go:12,30,53,83` show those
are the same call (see [`inventory.md`](inventory.md) §Section B class 3).
Verify that equivalence holds at implementation time before deleting.

## Test migration

`office/config` has **zero test files**. All four facade config test files must
move, minus the import/preview cases that belong to task 03:

| From | To | Note |
| --- | --- | --- |
| `service/config_read_test.go` (76) | `config/read_test.go` | direct move |
| `service/config_sync_test.go` (122) | `config/sync_test.go` | direct move |
| `service/config_write_test.go` (226) | `config/write_test.go` | direct move |
| `service/config_test.go` (152) | `config/service_test.go` | **split** — import/preview cases stay in `service/` for task 03 to migrate |

Adapt receivers (`*Service` → `*ConfigService`) and constructor calls. **No
assertion may be dropped.** If a moved test exercises a facade method that has no
`config` equivalent, that is a finding — stop and report it rather than deleting
the test.

## Acceptance

1. Detector Section A drops by **14** groups; Section B same-name pairs drop by
   **10** (`ApplyImport`, `export{Agents,Skills,Projects,Routines}`,
   `deleteMissing{Agents,Projects,Routines,Skills}`, `appendDeletions`).
   Measure as a delta against the branch point, not an absolute — wave-2 tasks
   may land in any order. See the ledger in [`plan.md`](plan.md).
2. `office/config` has non-zero test coverage for sync, export, and read.
3. `make -C apps/backend test` green; no change to any HTTP route or response.

## Verification

```bash
cd apps/backend && go test ./internal/office/config/... ./internal/office/service/... -count=1
make -C apps/backend test
make -C apps/backend lint
cd apps/backend && golangci-lint run ./... --new-from-rev=main --timeout=5m

cd docs/plans/office-service-collapse/officedup && GOTOOLCHAIN=local go run . \
  ../../../../apps/backend/internal/office | head -3   # expect 14 fewer groups
```

## Files likely touched

- deleted: `internal/office/service/config_sync.go`, `config_sync_helpers.go`,
  `config_sync_util.go`, `config_export.go`
- trimmed: `internal/office/service/config_read.go`
- moved: the four `service/config*_test.go` files → `internal/office/config/`
- possibly: `internal/office/service/service.go` (constructor field cleanup)

## Dependencies

Task 01 (for the group-count check).

## Parallelism

`parallel-safe` with tasks 04 and 06 — disjoint files, no shared schema,
migration, or generated contract.

## Rollback position

Single revert. `office/config` was already the wired owner
(`office/routes.go:60-61`), so reverting restores dead code, not behavior.

## Output contract

Summary, files changed, detector delta 40→26, the test-move table with final
assertion counts per file, and any facade method found to have no `config`
equivalent.

## Results

Pending.
