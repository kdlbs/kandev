---
status: draft
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-SYSTEM-PAGE-001
created: 2026-05-18
owners:
  - tbd
---
# System pages System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-SYSTEM-PAGE-SYSTEM-PAGE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-SYSTEM-PAGE-SYSTEM-PAGE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Out of scope (v1)

- Live log tail (WS streaming of new lines as they're written).
- Importing or analyzing logs outside the diagnostic bundle's retained
  three-day and byte/profile limits.
- Inline editing of `kandev_meta` or other system tables.
- A separate top-level "System" nav entry alongside "Settings" (chose nested-in-Settings instead).
- Tasks/Cron page (Radarr-equivalent) — kandev's office routines surface already covers this.
- Events page — covered by the office activity feed.

## Future scope

- Live log tail via WS subscription.
- Selective reset (drop only tasks, only sessions, etc.) — only if users actually need finer-grained recovery than Restore-from-snapshot.
- Backup scheduling (e.g., daily snapshot) beyond the existing pre-migration trigger.
- Backup files outside the resolved home are not included in System Status disk-usage totals.
- Health-issue → action wiring (e.g., a "VACUUM" CTA inline on the "Database is XYZ MB" health issue).

## Open questions

- Should the Updates page show only the latest release entry, or the full changelog history with the running version highlighted? Default: **full history, running version highlighted**, matches the existing `/settings/changelog` page.
