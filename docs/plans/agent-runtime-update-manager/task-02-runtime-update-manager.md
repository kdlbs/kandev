---
id: "02-runtime-update-manager"
title: "Add the responsive runtime update manager"
status: done
wave: 2
depends_on:
  - "01-direct-runtime-details"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-MANAGER-001
  - REQ-AGENTS-RUNTIME-MANAGER-002
acceptance_criteria:
  - AC-AGENTS-RUNTIME-MANAGER-001.1
  - AC-AGENTS-RUNTIME-MANAGER-001.2
  - AC-AGENTS-RUNTIME-MANAGER-001.3
  - AC-AGENTS-RUNTIME-MANAGER-001.4
  - AC-AGENTS-RUNTIME-MANAGER-001.5
  - AC-AGENTS-RUNTIME-MANAGER-001.6
  - AC-AGENTS-RUNTIME-MANAGER-001.7
  - AC-AGENTS-RUNTIME-MANAGER-002.1
  - AC-AGENTS-RUNTIME-MANAGER-002.2
  - AC-AGENTS-RUNTIME-MANAGER-002.3
  - AC-AGENTS-RUNTIME-MANAGER-002.4
system_design:
  - ../../specs/agents/system-design/runtime-update-manager.md
---

# Task 02: Add the responsive runtime update manager

## Summary

Replace the bottom disclosure with a visible page action and one responsive manager.
Keep version details, automatic-update drafts, manual guidance, and notification destinations within that surface.
Deliver the desktop and phone compositions together, without bulk controls until Task 03.

## In scope

- Header action, available count, explicit status groups, source/ownership information, retained outcomes, and direct agent details.
- One root overlay with overview-to-detail Back navigation and preserved list state.
- Persistent policy contributor controllers, explicit in-surface Save/Reset, save failures, and dirty-draft retention.
- Registry action targets for group/runtime fragments, repeated and delayed requests, and unchanged installed-card element targets.
- Shared status refresh, failed/unknown states, accessible focus handling, and responsive geometry.
- All locale catalogs and the public guide's runtime entry, policy, notification, and version-management sections.
- Updated notification/settings helpers and compatibility assertions through TDD.

## Out of scope

- Bulk checkboxes, bulk job coordination, review preconditions, backend changes, and unrelated toolbar redesign.
- Notification URL, timing, preference, and delivery changes.
- A second Settings save provider, immediate policy persistence, or a new general mobile picker system.

## Acceptance

1. Desktop and phone users open overview or agent details from visible page/card actions and existing notification fragments within one overlay.
2. Policies retain one route contributor per runtime, explicit reachable Save/Reset, authoritative failure behavior, and drafts across close/reopen and viewport changes.
3. Focus, groups, unknown/error states, controls, scrolling, locales, and the public guide match UI-01 through UI-05 and preserve individual capability boundaries.

## ASCII UI preview

Excerpts from [UI-01 through UI-05](plan.md#ascii-ui-preview).
Applies to this work order's listed 001 and 002 criteria.

```text
UI-01: Desktop page
Agents                         [Runtime updates (4)] [Install agents]
Installed agents
  Claude   Runtime 0.100.0   [Update available]        [New profile]
  ... existing profiles ...

UI-02: Desktop overview, before bulk delivery
+------------------------------------------------------------------+
| Runtime updates                                              [X] |
| Applies to new sessions.                                         |
|------------------------------------------------------------------|
| Available updates                              [Check for updates]|
| Claude    0.100.0 -> 0.103.0           [Update] [Manage versions]  |
| Codex     2.5.0 -> 2.6.0              [Update] [Manage versions]  |
| > Up to date                                                     |
| > Unknown checks                                                 |
| > Managed externally                                             |
| > Unavailable registrations                                      |
|------------------------------------------------------------------|
| [Close]                                                          |
+------------------------------------------------------------------+

UI-03: Dirty policy footer, in overview or detail
Unsaved policy changes                       [Reset] [Save changes]
Save or reset this policy before updating this runtime.

UI-04: Phone
+------------------------------------+
| Agents            [Install agents] |
| [       Runtime updates (4)      ] |
| ... cards and profiles ...         |
+------------------------------------+

+------------------------------------+
|                ----                |
| Runtime updates                [X] |
|------------------------------------|
| Claude                             |
| 0.100.0 -> 0.103.0                 |
| [Update] [Manage versions]         |
| ... one scrolling body ...         |
|------------------------------------|
| [Reset] [Save changes]  (if dirty) |
| [Close]                            |
|            safe area               |
+------------------------------------+

UI-05: Failed check or delayed target
Could not refresh. Last known information remains visible. [Retry]
Current: Unknown   Latest: 0.103.0                [Manage versions]
```

The page and overlay have separate scroll owners while the overlay is open. Background scrolling stays locked.
The manager itself has one scroll body, a fixed header/footer, and no second update overlay.
Phone detail replaces the body and supplies Back. Touch targets are at least 44px, with a nominal 48px primary action.
Spacing and example data are illustrative. No disabled placeholder Update all button appears in this slice.

## Verification

Add failing header, group, policy, and target tests before implementation.
Run this complete block from the repository root after the implementation:

```bash
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm exec vitest run app/settings/agents/page.test.tsx app/settings/agents/page.agent-list-snapshot.test.tsx components/settings/agent-runtime-update-manager.test.tsx hooks/domains/settings/use-agent-runtime-update-manager.test.tsx components/settings/agent-runtime-policies.test.tsx components/settings/use-runtime-auto-update-policy.test.ts components/settings/settings-target-provider.test.tsx components/settings/settings-save-provider.test.tsx components/settings/settings-save-exclusive.test.tsx lib/settings-discovery/target.test.ts lib/agents/runtime-update-manager.test.ts lib/agents/runtime-update-statuses.test.ts)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings app/settings/agents components/settings/agent-runtime-update-manager.tsx components/settings/agent-runtime-policies.tsx components/settings/agent-runtime-update-content.tsx components/settings/agent-runtime-update-surface.tsx components/settings/use-runtime-auto-update-policy.ts components/settings/settings-target-provider.tsx hooks/domains/settings/use-agent-runtime-update-manager.ts lib/settings-discovery/target.ts lib/agents/runtime-update-manager.ts e2e/tests/settings/agent-runtime-settings-helpers.ts e2e/tests/settings/agent-runtime-notifications-helpers.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-runtime-notifications.spec.ts tests/settings/agent-runtime-update.spec.ts tests/settings/agent-self-update.spec.ts tests/settings/agent-runtime-update-copilot.spec.ts tests/settings/agent-runtime-identity.spec.ts tests/settings/profile-capability-discovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-runtime-notifications.spec.ts tests/settings/mobile-agent-runtime-update.spec.ts tests/settings/mobile-agent-self-update.spec.ts tests/settings/mobile-agent-runtime-update-copilot.spec.ts tests/settings/mobile-agent-runtime-identity.spec.ts tests/settings/mobile-profile-capability-discovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The manager component, selector, and their tests are new files created by this work order.
Keep any newly extracted view or hook in changed-file lint and its corresponding focused unit command.
Add phone geometry and pointer cases to the existing notification suites, including 767/768px, narrow fine-pointer, and coarse tablet.
Preserve named notification, consent/reload, native guidance, fallback, unknown, summary, and interrupted-outcome assertions when replacing disclosure steps.
Capture desktop overview/detail and phone overview/detail screenshots. Compare their action order and scroll boundaries with the assigned previews.
Run each E2E command separately and retain its process handle until completion.

## Files likely touched

- `apps/web/app/settings/agents/page.tsx` and its tests.
- `apps/web/components/settings/agent-runtime-update-manager.tsx` and its tests (new).
- `apps/web/hooks/domains/settings/use-agent-runtime-update-manager.ts` and its tests.
- `apps/web/lib/agents/runtime-update-manager.ts` and its tests (new view-model selectors).
- `apps/web/components/settings/agent-runtime-policies.tsx` and its tests, replacing the old presentation rather than retaining a hidden second UI.
- `apps/web/components/settings/use-runtime-auto-update-policy.ts` and its tests.
- `apps/web/components/settings/agent-runtime-update-content.tsx`, surface, and installed-card controls.
- `apps/web/lib/settings-discovery/target.ts` and its tests.
- `apps/web/components/settings/settings-target-provider.tsx` and its tests.
- Existing desktop/phone notification, runtime update, identity, self-update, Copilot, and profile-discovery helpers/specs affected by surface selectors.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/agents.json`.
- `docs/public/agents-and-profiles.md`.
- This work order and its plan for actual results/status.

## Dependencies

Task 01 supplies the shared detail view and page-owned open controller.

## Risks

- Conditional policy mounting can lose dirty contributors. Keep controllers above the overlay's conditional presentation.
- The global floating Save is outside the modal focus boundary. The manager needs explicit reachable controls through the same route coordinator.
- Re-registering action targets during every render can reopen a dismissed view. Only pending or new explicit requests activate them.
- Duplicate action/element target identities or an independent hash listener can produce conflicting reveal behavior.
- An availability badge must not count external guidance or equate unknown versions with an empty successful check.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/runtime-update-manager.md), REQ-AGENTS-RUNTIME-MANAGER-001 and 002.
- [Design](../../specs/agents/system-design/runtime-update-manager.md), Composition through Target navigation and Responsive behavior.
- The manual-save contract and current `useRuntimeAutoUpdatePolicy` tests.
- Existing target registry tests, notification helpers, `AgentRuntimeUpdateSurface`, and mobile UI language.

## Results

Completed 2026-11-10. Added the header entry and available count, one responsive overview/detail manager, persistent policy controllers, explicit in-surface Save/Reset, action target registrations, status groups, refresh failure handling, locale catalogs, and public guide updates. Focused Vitest suites passed 115 tests across 15 files, including review-routing, refresh-gate, read-only details, and overview-state regressions. Typecheck, i18n check, i18n ratchet, changed-file Oxlint, public-doc validation, list-docs validation, and specification lint passed. Managed Playwright: desktop notifications 7/7, phone notifications 4/4, desktop runtime/self-update/Copilot/identity/profile group 27/27, and the phone group passed 20/20 after the drawer scroll-owner assertion was corrected.
