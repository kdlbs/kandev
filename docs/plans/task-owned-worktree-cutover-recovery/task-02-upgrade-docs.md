---
id: "02-upgrade-docs"
title: "Document safe cutover recovery"
status: done
wave: 2
depends_on: ["01-cutover-repair"]
plan: "plan.md"
spec: "../../specs/session-delete-resource-cleanup/spec.md"
---

# Task 02: Document safe cutover recovery

## Acceptance

- Public upgrade guidance explains single-initializer rollout and health
  verification.
- Recovery guidance pairs a pre-upgrade database backup with a compatible
  pre-cutover binary and explicitly forbids manual ownership-row deletion.
- SQLite and Kubernetes guidance remains consistent.

## Verification

```bash
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/operations.md`
- `docs/public/k8s.md`

## Dependencies

Task 01.

## Parallelism

Sequential. Documentation must describe the implemented migration behavior.

## Inputs

- `docs/specs/session-delete-resource-cleanup/spec.md`
- `docs/plans/task-owned-worktree-cutover-recovery/plan.md`
- Existing cutover backup and rollback guidance.

## Output contract

Report changed public pages, validation command and result, and synchronized
task/plan status.

## Results

- Updated `docs/public/operations.md` and `docs/public/k8s.md` with
  non-destructive cutover recovery guidance.
- `node scripts/validate-public-docs.mjs` — passed; 41 pages validated.
