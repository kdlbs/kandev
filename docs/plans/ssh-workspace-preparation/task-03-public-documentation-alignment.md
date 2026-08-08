---
id: "03-public-documentation-alignment"
title: "Public documentation alignment"
status: done
wave: 3
depends_on: ["01-ssh-workspace-lifecycle", "02-ssh-container-regression"]
plan: "plan.md"
spec: "../../specs/ssh-executor/spec.md"
---

# Task 03: Public Documentation Alignment

## Acceptance

- The executor guide describes SSH repository preparation, profile prepare/cleanup hooks, failure
  behavior, resume reuse, and retained task directories without overstating unsupported isolation or
  cleanup guarantees.
- The feature-status SSH row matches the shipped behavior and no longer contains the contradictory
  “does not materialize / ignores scripts” boundary.
- Public-doc validation passes.

## Verification

```bash
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/executors.md`
- `docs/public/feature-status.md`

## Dependencies

Tasks 01 and 02.

## Parallelism

Sequential. Documentation records the behavior proven by the runtime and E2E tasks.

## Inputs

- Updated SSH executor spec.
- Exact lifecycle behavior and limitations from Tasks 01 and 02.
- `/docs-maintainer` guidance; `executors.md` is primarily a how-to page and
  `feature-status.md` is reference.

## Risks

- Do not claim automatic remote directory deletion, remote Windows support, password auth, or general
  host isolation.
- Keep prepare failure and cleanup best-effort semantics explicit.

## Output contract

Report changed public pages, their primary Diátaxis types, exact validation results, blockers/risks,
and synchronized task/plan status.

## Results

- Updated `docs/public/executors.md` and `docs/public/feature-status.md` for SSH checkout preparation, profile hooks, failure gating, terminal cleanup, and retained task directories.
- `rtk node --test scripts/validate-public-docs.test.mjs`: 58 passed.
- `rtk node scripts/validate-public-docs.mjs`: 41 published docs pages validated.
