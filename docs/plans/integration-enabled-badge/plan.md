---
created: 2026-10-05
status: done
requirements:
  - REQ-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001
system_design:
  - ../../specs/integrations/system-design/enable-disable-toggle.md
legacy_specs: []
---

# Implementation Plan: Integration Enabled Badge

## Overview

Correct the Settings tree badge so disabling a connected GitLab integration no
longer leaves it labelled Enabled. One sequential work order covers the shared
projection and focused regression proof.

## Scope

All six built-in badges must respect saved workspace toggles. Preserve row
visibility, draft/Save behavior, provider operations and plugin badge semantics.
No new copy, backend changes, storage identities or layout changes.

## Root cause and technical approach

Source trace confirms `useEnabledIntegrations` badges GitLab and GitHub from
connection status alone. Jira, Linear and Azure DevOps have the same missing
toggle filter; Sentry already combines availability with its toggle. The old
requirement explicitly excluded correcting this convention; the user's request
replaces it through AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.9.

In `apps/web/hooks/domains/integrations/use-enabled-integrations.ts`, intersect
the existing connected set with the subscribed workspace enable reader. Replace
GitLab's active-workspace-only helper with `useGitLabStatus(workspaceId)`.
Update misleading comments in the projection, badge provider and branch hook.

| Provider | Connection input | Intended badge | Evidence |
| --- | --- | --- | --- |
| GitLab | Explicit-workspace status, authenticated OR token configured | Connected AND saved toggle on | Hook, component, desktop E2E |
| GitHub | Explicit-workspace status, authenticated OR token configured | Connected AND saved toggle on | Hook, component, desktop E2E |
| Azure DevOps, Jira, Linear | Existing workspace health predicates | Connected AND saved toggle on | Parameterized hook tests |
| Sentry | Existing workspace availability | Available AND saved toggle on | Hook test |
| Plugins | Registry enabled map | Existing plugin badge behavior | Existing tree suite |

Unknown/unconnected built-ins receive no badge; plugin fallback remains intact.

## ASCII UI preview

UI-01: Settings > Workspaces > workspace > Integrations, hide-disabled off.

```text
Saved GitLab disabled, credentials still connected:
Before: GitLab    [Enabled]
After:  GitLab

Saved GitLab re-enabled, credentials connected:
After:  GitLab    [Enabled]
```

Desktop reuses its existing Settings sidebar composition and scroll owner.
On phones the Settings sidebar is hidden; the `/settings` route is the menu,
without these badge rows. No control moves. The row remains reachable while hide-disabled
is off; when on, existing filtering removes it. Unsaved drafts preserve the
saved badge. Badge removal is required; spacing is illustrative.

## Tests

AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.3/.9:
`use-enabled-integrations.test.ts` proves connected/disabled, re-enable,
unconnected, event synchronization and row-workspace isolation with real stored
preferences and mocked provider probes. The same hook suite renders
the actual provider and badge to prove the false Enabled label disappears.
Run existing reader and tree suites to protect migration, visibility and plugins.

## E2E tests

Extend `integrations-index-enabled-toggle.spec.ts` on `chromium` with connected
GitLab, save disable, badge absent with row retained, then save re-enable and
badge restored. Assert unsaved changes do not alter the saved badge.
State-only normalization qualifies for the mobile-parity component-test
exception. Existing `mobile-settings-sidebar.spec.ts` verifies the changed
sidebar is hidden on phones; no phone badge capture applies.

## Work orders

- [x] [Task 01: Correct badge projection](task-01-badge-projection.md)

## Verification results

Completed 2026-10-05. See [Task 01 results](task-01-badge-projection.md#results)
for exact command scopes and execution evidence.

- Regression RED confirmed before production edits; GREEN: 65 tests across 4 files.
- Changed-file ESLint and TypeScript typecheck passed.
- Fresh managed host build and Chromium E2E: 2 tests passed, including both
  GitHub and GitLab saved toggle/badge flows and the unsaved draft boundary.
- Dependency installation, specification catalog/lint and diff checks passed.
- PR-documentation coverage preflight passed after using the installed Node PATH.
- Public integration guide now explains the saved-toggle badge contract.
- No delegated agent work. PR delivery follows the completed checks.

## Risks

- Toggles are drafts until Save; checking before Save cannot prove persistence.
- GitLab's tree row may belong to a workspace other than the active workspace.
- Connection probes must be mocked independently from enabled preferences.
