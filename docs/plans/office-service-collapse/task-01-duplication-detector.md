---
id: "01-duplication-detector"
title: "Land the duplication detector and golden inventory"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: none
parallel-safe: true
---

# Task 01: Duplication Detector and Golden Inventory

Every later task's acceptance is "the detector's group count dropped by N".
That check only exists if the detector is in the repo first. This task lands no
production code, so it is risk-free and can merge immediately.

## Inputs

- [`officedup/main.go`](officedup/main.go) — the AST-based detector, already
  written and committed with this plan.
- [`inventory.md`](inventory.md) — the 2026-08-08 baseline it produced.

## Acceptance

- `docs/plans/office-service-collapse/officedup` runs clean from a fresh
  checkout and reproduces **40 Section A groups** and **37 same-name Section B
  pairs** against `fb1d8fdcd`.
- `inventory.md` records the method well enough for someone who has never seen
  it to re-run and reconcile — specifically the receiver-normalization step,
  which is what the original text heuristic lacked.
- The detector lives outside the `apps/backend` Go module (own `go.mod`), so
  `make -C apps/backend test|lint` and `go vet ./...` never see it.

## Verification

```bash
cd docs/plans/office-service-collapse/officedup
GOTOOLCHAIN=local go run . ../../../../apps/backend/internal/office > /tmp/officedup-baseline.txt
grep -c '^- ' /tmp/officedup-baseline.txt          # expect 40 Section A groups
grep -c 'SAME-NAME' /tmp/officedup-baseline.txt    # expect 37

# Confirm it is invisible to the backend module:
make -C apps/backend lint
```

## Files likely touched

- `docs/plans/office-service-collapse/officedup/main.go` (new)
- `docs/plans/office-service-collapse/officedup/go.mod` (new)
- `docs/plans/office-service-collapse/inventory.md` (new)

## Dependencies

None.

## Parallelism

`parallel-safe` — docs-only, disjoint from every other task.

## Rollback position

Revert the commit. Nothing in `apps/backend` depends on these files.

## Output contract

Summary, files changed, the baseline counts, and confirmation that the detector
is outside the backend module.

## Results

Pending.
