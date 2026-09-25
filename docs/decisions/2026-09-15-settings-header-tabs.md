# ADR-2026-09-15-settings-header-tabs: Shared settings header tabs and maintenance grouping

**Status:** accepted
**Date:** 2026-09-15
**Area:** frontend

## Context

Data & Logs combines database actions, Office retention, compaction, backups, and logs in a long page.
The user selected tabs within the existing title/description header, aligned right on desktop.
The user also requested a reusable component for other settings pages.

## Decision

Extend the existing settings header with optional shared tab composition.
Place tabs below the description on phones. Preserve the same control treatment and keyboard behavior across consumers.
Data & Logs groups Database and Logs. Storage groups Host and Office retention.
Canonical page URLs stay stable. The `tab` query identifies the selected group.
Tab clicks replace the current history entry and preserve drafts. Cross-page navigation retains save protection.

This decision supersedes only the no-tabs constraint in
[the page-split decision](2026-09-03-separate-system-data-storage-pages.md).
It preserves the two direct settings destinations and their ownership boundaries.
Implementation is complete in [the plan package](../plans/settings-storage-tabs/plan.md),
including focused desktop and phone verification and the published maintenance documentation.

## Consequences

The shared header owns layout and interaction. Consumers retain domain state and permissions.
Search and health links must select the correct tab before revealing a control.
Inactive forms must retain their save contributors without exposing hidden focus targets.
Future consumers use the shared component instead of copying its styles.

## Alternatives Considered

- A separate tab row below the header uses extra vertical space. The user selected tabs inside the header.
- Page-specific controls can drift in appearance and interaction. A shared component establishes one contract.
- More sidebar destinations add navigation density. The two existing pages remain sufficient.
- A local-only selection loses copied-link and reload context. The query parameter preserves that context.
