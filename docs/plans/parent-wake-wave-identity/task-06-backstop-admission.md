---
id: "06-backstop-admission"
title: "Backstop admission compares wave identity"
status: pending
wave: 4
depends_on: ["01-wave-identity-primitives", "02-wave-identity-persistence"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-003
system_design:
  - ../../specs/office/system-design/parent-wake-wave-identity.md
---

# Task 06: Backstop admission compares wave identity

## Summary

Rewrite `ListStuckParents`' staleness arm to compare the parent's current
wave string (computed in SQL) against `runs.wake_wave_string`, add a
wave-member existence gate so a parent with no possible wave is never
listed, and keep a parent-scoped timestamp fallback for pre-upgrade rows
that carry no wave identity at all.

## In scope

- New `internal/db/dialect` helper for an ordered, comma-joined id
  aggregate (SQLite `GROUP_CONCAT` over an ordered subquery; PostgreSQL
  `string_agg(... ORDER BY ...)`), added beside `JSONExtract`.
- `ListStuckParents`: a wave-string CTE column
  (`p.id || '|' || COALESCE(<helper>, '')`) over the wave-member predicate.
- A new `EXISTS` gate (wave-member predicate) beside the existing
  archived-only `EXISTS`, removing a parent with no wave members from
  candidacy (AC-...-003.9). Additive and narrowing only.
- Rewrite the second `NOT EXISTS` arm into three clauses: queued/claimed
  blocks unconditionally; a terminal run whose `wake_wave_string` matches
  the parent's current one blocks; only when the parent has **no**
  `task_children_completed` run with `wake_wave_key <> ''` at all does the
  existing `requested_at >= newest_child_updated_at` timestamp rule apply
  (evaluated once per parent, not per row — AC-...-003.6).

## Out of scope

- The first `NOT EXISTS`/receipt-comparison arm (unchanged — receipts stay
  state-inclusive, AC-...-004.6).
- Porting this query's existing `json_extract`/unbranched `GROUP_CONCAT` to
  dialect form (`53c24173` — do not widen or close that gap here; only the
  new wave-string fragment is dialect-branched).
- The whole-second comparison in the queued-or-claimed arm (`b7e29d7c`).

## Acceptance

- A parent with a delivered, unchanged wave is not a candidate; a
  non-state child edit after delivery does not make it one; a real
  wave-member change does.
- A parent whose only children are archived/ephemeral/automation-origin is
  never listed.
- A parent with a pre-upgrade terminal run (`wake_wave_key = ''`) is judged
  by the timestamp rule; once any keyed run exists for that parent, the
  timestamp rule is never consulted for it again, even for an older
  pre-upgrade row on the same parent.
- A failed/cancelled run whose wave string matches the current one keeps
  blocking across an unrelated child edit and is unblocked only by a
  wave-member change.

## Verification

```bash
cd apps/backend
go test ./internal/db/dialect/... -run TestOrderedIDConcat -v
go test ./internal/office/repository/sqlite/... -run TestListStuckParents -v
KANDEV_TEST_POSTGRES_DSN=... go test ./internal/office/repository/sqlite/... -run TestListStuckParents -v
```

## Files likely touched

- `internal/db/dialect/json.go` (or a new file beside it for the aggregate
  helper, if it doesn't fit `json.go`'s theme)
- `internal/db/dialect/dialect_test.go`
- `internal/office/repository/sqlite/wake_receipts.go`
- `internal/office/repository/sqlite/wake_receipts_test.go`

## Dependencies

Task 01 (wave-member predicate, matched exactly), Task 02 (`wake_wave_key`/
`wake_wave_string` columns must exist to query against).

## Risks

- **Ordering divergence between Go and PostgreSQL collation** — the
  requirements' AC-...-001.13 names this as a silent, PostgreSQL-only
  failure mode. The gated Postgres test must assert byte-identical ordering
  with a fixture using real UUID-shaped ids, not sequential test ids that
  would happen to sort the same everywhere.

## Parallelism

`sequential`

## Inputs

- System design: "Backstop admission" section in full, including its
  worked SQL fragments.
- `internal/office/repository/sqlite/wake_receipts.go` (current
  `ListStuckParents`, `child_set_key` aggregate idiom to mirror for the new
  wave-string aggregate).
- `internal/db/dialect/json.go` (signature/doc-comment style to match).

## Results

Pending.
