---
id: "12-inherited-temp-docs"
title: "Update inherited-temp operations docs"
status: complete
wave: 8
depends_on: ["10-inherit-service-temp", "11-e2e-temp-cleanup"]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 12: Update inherited-temp operations docs

Make public operations guidance match the service-inherited temp boundary and legacy cleanup limits.

## Acceptance

- Public docs explain that host-local agents inherit service `TMPDIR`/`TMP`/`TEMP`, persistent
  `GOCACHE` is separate, and host temp policy owns shared scratch cleanup.
- Docs do not advertise an agent-temp Storage row/action or automatic deletion of legacy
  `/tmp/kandev-agent/*`; they explain that confirmed-inactive legacy data needs deliberate host
  cleanup.
- Public docs validation passes.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/operations.md`
- affected spec/ADR/plan wording discovered by docs audit

## Dependencies

Tasks 10 and 11.

## Inputs

- `/docs-maintainer`
- Updated Storage spec and ADR 0045

## Output contract

Report public/internal docs updated, validation results, stale terminology removed, blockers, and
update this task plus the serialized plan checkbox when complete.

## Completion

- Updated the public operations guide to document service-inherited `TMPDIR`, `TMP`, and `TEMP`,
  platform defaults when those variables are unset, and shared cache behavior for temp-derived
  tooling.
- Clarified that the default and managed Go build caches are governed by `GOCACHE`, with Kandev's
  managed path remaining opt-in and independent from temporary-directory policy.
- Removed the superseded agent-temp Storage analysis/action, marker, cleanup-record, and
  archive/delete directory-removal guidance. Shared scratch cleanup now belongs to host temp policy.
- Documented deliberate host-administrator cleanup for confirmed-inactive legacy
  `/tmp/kandev-agent/*` directories, with no automatic name- or age-based removal.
- Public-doc tests and validation passed; `git diff --check` reported no whitespace errors.
