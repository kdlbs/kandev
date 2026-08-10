---
id: "05-storage-temp-artifact-docs"
title: "Temporary-artifact operations documentation"
status: done
wave: 5
depends_on: ["04-storage-temp-artifact-e2e"]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 05: Temporary-artifact operations documentation

Reconcile public operator guidance with the new narrow cleanup action. Documentation must make the
distinction between registered service artifacts and shared/unowned temporary data unmistakable.

## Acceptance

- `docs/public/operations.md` explains the manual-only **Clean stale artifacts** action, quarantine
  behavior, 24-hour stale rule, and the exact categories it will not touch (legacy, caches, preview/
  CI/dev harness, and arbitrary shared `/tmp`).
- The public documentation continues to state that inherited `TMPDIR`/`TMP`/`TEMP` are unchanged and
  does not promise that Storage maintenance cleans all folders whose names start with `kandev-`.
- Public-doc tests/validation and `git diff --check` pass, and the task/plan/spec/ADR links remain
  internally consistent.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `docs/public/operations.md`
- `docs/specs/system-page/storage-maintenance.md` (only if implementation reveals wording drift)
- `docs/plans/storage-temp-artifacts/plan.md`
- `docs/plans/storage-temp-artifacts/task-*.md`

## Dependencies

Task 04.

## Parallelism

`sequential`; documentation is the final reconciliation of shipped behavior and verification.

## Inputs

- ADR-2026-08-08-owned-temp-artifact-cleanup.
- `docs/public/operations.md` Storage maintenance section.
- `/docs-maintainer` public-doc validation conventions.

## Output contract

Report changed public-doc sections, exact validation output, any wording/spec drift found, and
synchronized task/plan status. Do not broaden the implementation scope to unregistered temp paths.

## Results

- Updated `docs/public/operations.md` to document the manual-only cleanup, 24-hour stale rule,
  quarantine behavior, and exclusions for arbitrary/shared temp data, caches, legacy roots, and
  preview/CI/dev-harness data.
- Public documentation verification passed: `node --test scripts/validate-public-docs.test.mjs`
  (58 passed), `node scripts/validate-public-docs.mjs` (41 published pages), and `git diff --check`.
