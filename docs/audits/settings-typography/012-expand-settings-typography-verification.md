# TYPO-012: Expand typography verification beyond notifications

- Priority: P2
- Status: In progress
- Scope: Settings E2E and rendered visual verification
- Related: TYPO-001 through TYPO-011

## Finding

The repository already has useful computed-style coverage for Notifications, but most other settings typography is not tested by semantic role. Existing tests usually verify visibility, route changes, state, or geometry. That makes it possible to change a shared primitive and miss a hierarchy regression on another settings page.

## Evidence

- `apps/web/e2e/tests/settings/notifications-type-scale.spec.ts:24-35` measures desktop computed font sizes.
- `apps/web/e2e/tests/settings/mobile-notifications-type-scale.spec.ts:28-43` measures mobile computed font sizes.
- `apps/web/e2e/tests/settings/mobile-settings-index.spec.ts` focuses on navigation visibility, route behavior, and bounds.
- `apps/web/e2e/tests/settings/mobile-settings-discovery.spec.ts` focuses on search geometry, navigation, and focus.
- `apps/web/e2e/tests/settings/mobile-agent-profile-layout.spec.ts` and `apps/web/e2e/tests/settings/mobile-workspace-settings-tabs.spec.ts` focus on layout and bounds, not type roles.
- `apps/web/components/settings/notification-events-table.tsx:149-165` leaves mobile event titles and provider names to inherited `Card` typography, while sibling group headings explicitly use `text-sm` in `apps/web/components/settings/notifications-settings.tsx:216-221,415-420`.
- The existing desktop/mobile notification type-scale tests measure descriptions, table rows, and provider rows, but do not assert the event-title/label hierarchy (`apps/web/e2e/tests/settings/notifications-type-scale.spec.ts:26-34`, `mobile-notifications-type-scale.spec.ts:34-41`).
- System/account routes currently have mostly geometry/state coverage but no shared typography assertions for SSH card headers, Users/Backups/Licenses rows, Account token/security tables, Storage Policy cards, or Terminal Editors sections.

## User impact

Typography regressions are found by manual review only, and the existing notifications test encodes one page's numeric values rather than a reusable settings contract.

## Proposed direction

After the role map is finalized, add a small typography fixture or helper that asserts semantic roles, not arbitrary descendants. Cover at least one page from each settings group, plus one technical-value surface and one mobile navigation surface. Extend Notifications with shared event/group title and description assertions. Keep screenshots for perceived hierarchy and computed-style assertions for deterministic values.

## Verification

- Add desktop and mobile coverage for page title, section title, card title, field label, helper, control, and technical value roles.
- Assert notification event titles/provider labels in both mobile and desktop, alongside the existing description/table-row checks.
- Add representative system/account/editor fixtures for card titles, descriptions, labels, dense tables, errors, and mobile action stacking.
- Update the notifications tests to use the shared role helper.
- Run the focused mobile project and a desktop settings project after each migration phase.

## Progress

- 2026-08-16: Opened from the settings source audit.
