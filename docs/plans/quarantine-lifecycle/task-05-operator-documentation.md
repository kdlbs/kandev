---
id: "05-operator-documentation"
title: "Operator documentation"
status: done
wave: 5
depends_on: ["04-responsive-quarantine-ui"]
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 05: Operator documentation

Update the public Operations guide for the completed quarantine lifecycle.

## Acceptance

- The guide explains the retention deadline, eligible automatic purge, scheduling-disabled
  behavior, **Clear eligible**, and the irreversible **Force clear all** override.
- It distinguishes the earliest safe deletion timestamp from an exact promised cleanup time and
  states that force bypasses only retention.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/operations.md`

## Dependencies

Task 04 so published labels match the final UI.

## Parallelism

Sequential.

## Inputs

- Spec: quarantine behavior and out-of-scope timing guarantees
- Plan: Public Documentation
- ADR: `docs/decisions/2026-07-29-quarantine-retention-override.md`

## Risks

- Documentation must not imply a dedicated sweeper runs when scheduling is disabled.

## Output contract

Report public wording, links if changed, exact validation results, blockers/risks, and update this
task plus `plan.md` status.
