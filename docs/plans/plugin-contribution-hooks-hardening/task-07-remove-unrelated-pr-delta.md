---
id: "07-remove-unrelated-pr-delta"
title: "Remove unrelated PR delta"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/plugins/requirements/plugins.md"
---

# Task 07: Remove unrelated PR delta

## Acceptance

- `apps/web/scripts/lib/git-base.mjs` has no diff from the current PR base after the
  branch is refreshed immediately before implementation.
- No plugin behavior, i18n ratchet behavior inherited from `main`, or contributor
  changes outside that one unrelated delta are removed.
- The final patch has no whitespace errors.

## Verification

```bash
git fetch origin main
git diff --exit-code origin/main -- apps/web/scripts/lib/git-base.mjs
git diff --check
```

## Files likely touched

- `apps/web/scripts/lib/git-base.mjs`

## Dependencies

None, but refresh `origin/main` first so cleanup uses the current PR base.

## Parallelism

`parallel-safe` with Tasks 01–03; the file is disjoint.

## Inputs

- Plan: **Contract and documentation**, PR-scope cleanup bullet.
- Current PR diff against `origin/main`; do not use the pre-merge historical base.

## Risks

`origin/main` can advance. Resolve against the actual base at execution time and do not
blindly restore other files.

## Output contract

Report the exact base SHA, removed diff, verification output, and synchronize task/plan
status/results.

## Results

- Refreshed `origin/main`; current base SHA is `82951e3848e4046781a08aacb34d2ae402cf622d`.
- Removed the unrelated in-progress-merge ratchet delta from
  `apps/web/scripts/lib/git-base.mjs`.
- `rtk git diff --exit-code origin/main -- apps/web/scripts/lib/git-base.mjs` —
  passed with no diff.
- `rtk git diff --check` — passed.
