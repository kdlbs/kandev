---
id: "04-update-public-logging-documentation"
title: "Update public logging documentation"
status: done
wave: 3
depends_on:
  - "02-implement-bounded-log-segments"
  - "03-extend-diagnostic-bundle-discovery"
plan: "plan.md"
spec: "../../specs/platform/diagnostic-logging.md"
---

# Task 04: Update Public Logging Documentation

## Intent

Explain the bounded rolling behavior so operators know which evidence remains
available during high-volume days and how much disk space Kandev can use.

## Acceptance

- Operations and deployment pages describe the active file, numbered segment
  names, 16 MiB segment bound, 256 MiB total budget, and three-day maximum age.
- The text states that high volume keeps the newest bounded evidence and can
  shorten the available time window. Configuration text states that segment
  and retention settings are not configurable.
- Public-document validation passes.

## Files Likely Touched

- `docs/public/operations.md`
- `docs/public/configuration.md`
- `docs/public/docker.md`
- `docs/public/k8s.md`

## Dependencies

Tasks 02 and 03 must confirm the final filenames, byte limits, upgrade behavior,
and bundle ordering before this task updates operator guidance.

## Parallelism

`sequential` after Tasks 02 and 03.

## Inputs

- The final writer and bundle behavior from Tasks 02 and 03.
- The diagnostic-logging specification.
- The accepted bounded-log ADR.
- The repository public-document style and validation rules.

## Implementation

1. Update each existing backend-log section in place. Keep its current page
   type and operator focus.
2. Use the same filenames, byte limits, UTC-day terms, and newest-evidence
   explanation on every page.
3. Preserve existing permissions, deployment path, and disk-sizing guidance.
4. Run the public-document validator and correct all reported issues.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Output Contract

Report the updated pages, exact validation results, blockers, and remaining
risks. Update this task and `plan.md` in the same conversation.

## Results

- Updated `docs/public/operations.md`, `configuration.md`, `docker.md`, and
  `k8s.md` with the active and numbered filenames, 16 MiB segment bound,
  256 MiB total budget, three-day maximum age, and newest-evidence behavior.
- Preserved deployment paths, persistence guidance, permissions, and existing
  disk-budget context.
- Verification: `node --test scripts/validate-public-docs.test.mjs` passed all
  61 tests; `node scripts/validate-public-docs.mjs` validated 41 pages.
- Blockers: none.
- Remaining risk: none known for the documented contract.
