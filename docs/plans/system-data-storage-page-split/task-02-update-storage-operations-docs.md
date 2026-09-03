---
id: "02-update-storage-operations-docs"
title: "Update storage operations documentation"
status: pending
wave: 2
depends_on:
  - "01-split-system-pages"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-001
acceptance_criteria:
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.1
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.4
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.7
system_design:
  - ../../specs/system-page/system-design/system-data-storage-pages.md
---

# Task 02: Update Storage Operations Documentation

## Summary

Update the public storage instructions for the new Storage destination.
Recapture screenshots that show the old `Data & Logs` breadcrumb.

## In scope

- Update storage paths and screenshot captions in `docs/public/operations.md`.
- Seed isolated disposable data for the storage examples.
- Recapture the four affected Storage screenshots.
- Check that the images show the Storage route and contain no private data.

## Out of scope

- Changes to storage behavior or API documentation.
- New public documentation pages.
- Changes to unrelated screenshots.

## Acceptance

- Public instructions direct operators to `Settings > System > Storage`.
- Every affected screenshot shows the Storage destination and safe demo data.
- Public documentation validation passes with no broken links or metadata.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs/public docs/screenshots
```

Run these commands from the repository root.

## Files likely touched

- `docs/public/operations.md`
- `docs/screenshots/system-storage.png`
- `docs/screenshots/system-maintenance-policy.png`
- `docs/screenshots/system-quarantine.png`
- `docs/screenshots/system-docker-cleanup.png`

## Dependencies

- Task 01 must complete before screenshot capture.

## Risks

- A screenshot from a developer instance can expose local paths or task names.
- A crop can hide the new route title and fail to show the navigation change.

## Parallelism

`sequential`

## Inputs

- The implemented Storage route from Task 01.
- The public operations how-to guide.
- The product demo seeding and capture procedures.

## Results

Pending.
