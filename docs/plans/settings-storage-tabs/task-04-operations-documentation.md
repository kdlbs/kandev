---
id: "04-operations-documentation"
title: "Document the settings destinations"
status: done
wave: 4
depends_on: ["03-temporary-files-compaction"]
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-002
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-003
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-004
acceptance_criteria:
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.4
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.5
system_design:
  - ../../specs/ui/system-design/settings-header-tabs.md
  - ../../specs/system-page/system-design/system-data-storage-pages.md
---

# Task 04: Document the settings destinations

## Summary

Update public maintenance instructions after the UI work passes. Synchronize the specification lifecycle with the implemented behavior.

## In scope

- Update operations navigation for Database, Logs, Host, and Office retention.
- Explain Temporary Kandev files with the same eligibility and quarantine wording as the UI.
- Audit authentication docs, README, and screenshot catalog for affected paths and captions.
- Resolve planned presentation notes in Office and tool-payload designs when the new allocation ships.
- Record work-order results and promote matching draft designs to current only after their checks pass.

## Out of scope

- New retention algorithms, provider eligibility, persistence, or API payloads.
- Unrelated settings-page migration, release flags, and task delegation.

## Acceptance

- Public operations instructions lead to the implemented tabs and use the final visible labels.
- Documentation preserves permissions, cleanup eligibility, database space-reuse explanations, and backend retention semantics.
- All work-order results and specification statuses accurately represent actual implementation and verification.

## Verification

Run from the repository root. New test paths below are implementation deliverables.
Use TDD for changed logic. Run focused failing tests before implementation, then the complete block after changes.
The E2E runner rebuilds current sources; do not pass `--no-build` for changed UI.

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs
```

## Files likely touched

- docs/public/operations.md and authentication.md if navigation wording changes
- README.md and docs/screenshots.md only where the audit finds stale content
- Owning requirements/designs in this package and the linked Office/tool-payload presentation notes
- docs/plans/settings-storage-tabs/plan.md and work-order Results sections

## Dependencies

03-temporary-files-compaction

## Risks

Public documentation must describe shipped behavior. Existing historical test counts must not be presented as evidence for this package.

## Parallelism

`sequential`

## Inputs

- [Header-tab requirements](../../specs/ui/requirements/settings-header-tabs.md) and [design](../../specs/ui/system-design/settings-header-tabs.md).
- [Data/storage requirements](../../specs/system-page/requirements/system-data-storage-pages.md) and [design](../../specs/system-page/system-design/system-data-storage-pages.md).
- Existing SystemPageShell, SettingsPageHeader, settings save tests, and SleepInhibitionInfoTooltip touch pattern.

## Results

Updated public operations and authentication documentation with the Database,
Logs, Host, and Office retention destinations, plus the temporary-file and
retention semantics. Updated the linked requirements and system designs to
reflect the implemented tab allocation and presentation contract.

Validation passed with public-doc tests and validation, the specification
catalog and linter, i18n checks, and documentation diff validation.
