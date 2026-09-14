---
id: "01-wave-identity-primitives"
title: "Wave identity primitives"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-001
acceptance_criteria:
  - AC-OFFICE-WAKE-WAVE-IDENTITY-001.1
  - AC-OFFICE-WAKE-WAVE-IDENTITY-001.2
  - AC-OFFICE-WAKE-WAVE-IDENTITY-001.3
  - AC-OFFICE-WAKE-WAVE-IDENTITY-001.5
  - AC-OFFICE-WAKE-WAVE-IDENTITY-001.6
  - AC-OFFICE-WAKE-WAVE-IDENTITY-001.7
  - AC-OFFICE-WAKE-WAVE-IDENTITY-001.8
  - AC-OFFICE-WAKE-WAVE-IDENTITY-001.9
  - AC-OFFICE-WAKE-WAVE-IDENTITY-001.10
  - AC-OFFICE-WAKE-WAVE-IDENTITY-001.11
  - AC-OFFICE-WAKE-WAVE-IDENTITY-001.13
system_design:
  - ../../specs/office/system-design/parent-wake-wave-identity.md
---

# Task 01: Wave identity primitives

## Summary

Add the pure two-encoding derivation (`WaveString`, `WaveKey`) as a new leaf
package, and add the shared wave-member repository read every producer will
use to obtain the ordered id set those functions consume. No caller changes
yet — this is foundation only.

## In scope

- `internal/office/waveidentity` package: `WaveString(parentID string,
  memberIDs []string) string` and `WaveKey(parentID string, memberIDs
  []string) string`, per the system design's "The two encodings" section
  (separator `|`, digest prefix `task_children_completed:<parentID>:`,
  lowercase hex sha256 of the wave string). No I/O, no error return.
- `internal/office/repository/sqlite.Repository.ListWaveMembers(ctx,
  parentID) ([]ChildState, error)` in `blockers.go`, beside `ListChildStates`:
  `(id, state)` ordered by `id`, filtered to `archived_at IS NULL AND
  is_ephemeral = 0 AND COALESCE(origin, '') != 'automation_run'` — the same
  predicate `task/repository/sqlite.ListChildCompletionRows` already applies.

## Out of scope

- Calling either the derivation or `ListWaveMembers` from any producer
  (Tasks 03-05).
- Persisting the derived values anywhere (Task 02).
- The PostgreSQL-dialect wave-string SQL aggregate used by the backstop
  (Task 06) — that is a separate, SQL-side expression of the same identity,
  not this Go package.

## Acceptance

- `waveidentity.WaveKey(p, ids) == waveidentity.WaveKey(p, ids)` for any
  fixed input (determinism); different `ids` slices (same length, different
  order) that represent a *different* set produce a different key, but two
  calls with the *same* already-sorted slice never differ regardless of the
  slice's original pre-sort arrival order (callers, not the function, sort).
- `ListWaveMembers` and `task/repository/sqlite.ListChildCompletionRows`
  return the identical set of ids for one fixture parent with a mix of
  ordinary, archived, ephemeral, and automation-origin children.
- `go vet ./internal/office/waveidentity/... ./internal/office/repository/sqlite/...`
  is clean.

## Verification

```bash
cd apps/backend
go test ./internal/office/waveidentity/... -run . -v
go test ./internal/office/repository/sqlite/... -run TestListWaveMembers -v
```

## Files likely touched

- `internal/office/waveidentity/waveidentity.go` (new)
- `internal/office/waveidentity/waveidentity_test.go` (new)
- `internal/office/repository/sqlite/blockers.go`
- `internal/office/repository/sqlite/blockers_test.go` (or a new
  `wave_members_test.go` if the existing file is near its line-count limit)

## Dependencies

None.

## Risks

None — pure functions and a single new read-only query, no behavior change
for any existing caller.

## Parallelism

`sequential`

## Inputs

- Requirements: REQ-OFFICE-WAKE-WAVE-IDENTITY-001, full text.
- System design: "Wave identity" and "The two encodings" sections.
- `internal/office/repository/sqlite/blockers.go` (`ListChildStates` for the
  row shape and query idiom).
- `internal/task/repository/sqlite/task.go` (`ListChildCompletionRows`,
  `andNotAutomationOrigin`) for the predicate this must match exactly.
- `internal/office/costs/modelsdev` as the precedent leaf-package shape
  `internal/orchestrator` already imports.

## Results

Done. `internal/office/waveidentity` (`WaveString`, `WaveKey`) plus
`Repository.ListWaveMembers` (excludes archived, ephemeral, and
automation-origin children, ordered ascending by id) landed in
`feat(office): add wave identity derivation and wave-member read`.
`go test ./internal/office/waveidentity/... ./internal/office/repository/sqlite/... -run 'WaveIdentity|WaveMembers'` green.
