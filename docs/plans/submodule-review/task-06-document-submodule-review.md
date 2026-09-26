---
id: "06-document-submodule-review"
title: "Document nested submodule review"
status: done
wave: 3
depends_on: ["03-present-submodule-review-hierarchy"]
plan: "plan.md"
spec: "../../specs/ui/requirements/submodule-review.md"
---

# Task 06: Document nested submodule review

## Acceptance

- Public Review guidance explains that initialized direct and nested submodule files appear with repository scope and that unavailable children fall back to the gitlink.
- Public feature status names nested initialized submodule support without implying that Kandev creates or coordinates pull requests for submodule remotes.
- Public docs validation passes and internal spec/ADR/plan links resolve.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
rg -n "submodule|Nested Submodule Review" docs/public docs/specs docs/decisions docs/plans/submodule-review
```

## Files likely touched

- `docs/public/sessions-and-review.md`
- `docs/public/feature-status.md`

## Dependencies

Task 03 for final visible terminology.

## Parallelism

`parallel-safe` with Task 05 after Task 03 because public docs and E2E files are disjoint.

## Inputs

- Spec **What**, **Failure modes**, and **Out of scope**.
- ADR consequences for separate Git-host workflows.
- Existing public Review section and feature-status table.

## Output contract

Report public pages and Diataxis classifications, wording boundaries, exact validation results, files changed, blockers/risks, and synchronized task/plan status.

## Results

Updated the public Sessions and Review guide and Feature Status reference with initialized direct/nested submodule scope behavior, anchored comparisons, unavailable-child fallback, and separate submodule PR workflows.

Verification:

- `node --test scripts/validate-public-docs.test.mjs` — 58 passed.
- `node scripts/validate-public-docs.mjs` — 41 published pages validated.
- `rg -n "submodule|Nested Submodule Review" docs/public docs/specs docs/decisions docs/plans/submodule-review` — references present.
