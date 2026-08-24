---
id: "03-update-operator-docs"
title: "Update operator documentation"
status: done
wave: 3
depends_on: ["01-defer-retained-rotation", "02-close-missing-payload"]
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 03: Update Operator Documentation

## Intent

Document the retained-generation and missing-payload behavior in the existing Storage maintenance
how-to guide.

## Acceptance

- The guide states that one active Go-cache quarantine generation defers another rotation without
  failing maintenance.
- The guide states that permanent deletion can close an absent payload without changing the live
  cache, while restore remains fail-closed.
- Public-document validation passes, and the plan records all task results.

## Files Likely Touched

- `docs/public/operations.md`
- `docs/plans/go-cache-quarantine-lifecycle/plan.md`
- `docs/plans/go-cache-quarantine-lifecycle/task-01-defer-retained-rotation.md`
- `docs/plans/go-cache-quarantine-lifecycle/task-02-close-missing-payload.md`
- `docs/plans/go-cache-quarantine-lifecycle/task-03-update-operator-docs.md`

## Dependencies

- Task 01: final deferral result fields and semantics.
- Task 02: final deletion and byte-accounting semantics.

## Parallelism

`sequential`.

## Inputs

- Spec sections: **Go build cache**, **Failure modes**, and **Scenarios**.
- Plan section: **Public Documentation**.
- Public page type: how-to guide.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs && git diff --check
```

## Output Contract

Report the documentation changes, exact validation outcomes, public-doc type, cleanup evidence,
remaining risks, and synchronized task and plan status.

## Results

- Updated `docs/public/operations.md` with the one-active-generation deferral result and the
  missing-payload deletion, zero-byte accounting, and fail-closed restore behavior.
- Public page type: how-to guide.
- `node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs &&
  git diff --check` passed (60 validation tests and 41 published pages).
