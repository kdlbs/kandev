---
created: 2026-10-02
status: implemented
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-001
system_design:
  - ../../specs/agents/system-design/runtime-update-notifications.md
legacy_specs: []
---

# Implementation Plan: Remove the Floating Agent Runtime Update Indicator

## Overview

Remove the app-wide runtime update button shown beside Archive in the user's
screenshot. Deliver the component removal, desktop/phone regression coverage
and public guide correction in one sequential work order. Agents owns this
contract because the control projects agent runtime availability. This package
updates the existing requirement and design rather than creating a UI-owned
copy. The user authorized implementation and delivery through PR merge on 2026-10-02. Work remains sequential in this session.

## Scope

### In scope

- AC-AGENTS-RUNTIME-NOTIFY-001.7: absence of the floating count/control on both viewports.
- Preserve 001.2 background awareness, 001.3 notification delivery and 001.6 direct-link reveal.
- Replace obsolete indicator assertions and public navigation guidance.

### Out of scope

- Disabling runtime updates, notifications or automatic policies; backend changes.
- Archive layout, replacement badges, feature flags, new preferences or dependencies.
- Changes to manual version selection, ownership, save/reload or retained outcomes.

## Technical approach

Remove the count/button JSX and presentation-only imports from
`components/update-available-toast-bridge.tsx`; return null after its existing
hooks and effects. Keep the component mounted, `useUpdateAvailableToast`, shared
status fetching, refresh interval/cleanup and notification refresh intact.

Update `e2e/tests/settings/agent-runtime-notifications-helpers.ts` so its existing
two-update fixture asserts the indicator is absent outside Settings, then
navigates to Settings directly to preserve group-fragment reveal coverage. Keep
the subsequent runtime toast, row-fragment reveal, policy save/reload, manual
guidance, unknown version, retained failure and version dialog/drawer checks.
Prove absence after the toast is dismissed as well. Do not weaken those flows
when replacing the former positive indicator interaction.

The public `agents-and-profiles.md` how-to guide must remove the persistent
indicator claim and describe Settings and notification links. No new locale
copy is needed. Remove the unused `runtimeIndicator_one`/`runtimeIndicator_other`
keys from every shipped catalog and regenerate pseudo after confirming no
remaining consumers. Keep the i18n guard migration record unchanged.

The original notification and compact-settings plans link this package as the
owner of the follow-up. Their old statuses/counts remain historical evidence;
this package records its own final validation.

## ASCII UI preview

### UI-01: Desktop app view, newer runtimes available

```text
Before: [Download: 2 agent runtime updates]  [Archive]
After:                                     [Archive]
```

### UI-02: Phone app view, newer runtimes available

```text
[App content]
[Existing phone navigation]
```

The floating runtime update control is absent on both surfaces. Existing app
content, archive and navigation retain their positions. Settings > Agents is
the persistent management entry; a runtime toast's Review runtime link still
opens its row. These are structural requirements for
AC-AGENTS-RUNTIME-NOTIFY-001.7 and 001.6; spacing is illustrative. No new overlay,
scroll owner, touch target or loading/error presentation is introduced. The
nearest shipped mobile precedents are SettingsPageTemplate, InstalledAgentCard
and AgentRuntimeUpdateSurface's existing version drawer. Existing page scrolling,
safe areas and 44px phone settings/notification targets remain authoritative.

## Tests

Add `components/update-available-toast-bridge.test.tsx` using available update
statuses. Assert no rendered indicator/DOM, notification consumption, periodic
refresh and cleanup, and refresh on a runtime notification. Cover zero and
nonzero availability and route independence. Existing toast and status hook
suites preserve 001.2/001.3; avoid duplicating backend delivery tests.

## E2E tests

Run the shared helper via `agent-runtime-notifications.spec.ts` in chromium and
`mobile-agent-runtime-notifications.spec.ts` in mobile-chrome. Cover the available
updates fixture, absence before/after toast dismissal, normal Settings entry
and deep links, and retained management flows (001.7, 001.3, 001.6). Existing
390px fine-pointer and 767/768px cases remain in the desktop suite. Use `prCapture`
for a focused desktop and phone app screenshot with updates available; compare
the screenshots to UI-01/UI-02. No horizontal overflow or displaced navigation.
The managed runner rebuilds the web/backend before testing; run projects
sequentially using their configured resource budgets.

## Work orders

- [x] [Task 01: Remove the floating indicator](task-01-remove-floating-indicator.md) (done, wave 1, no dependencies).

## Verification commands

```bash
# From the repository root. Bootstrap dependencies once in this worktree.
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/update-available-toast-bridge.test.tsx hooks/use-update-available-toast.test.ts hooks/domains/settings/use-agent-runtime-update-statuses.test.tsx)
(cd apps/web && pnpm exec eslint components/update-available-toast-bridge.tsx components/update-available-toast-bridge.test.tsx e2e/tests/settings/agent-runtime-notifications-helpers.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-runtime-notifications.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-runtime-notifications.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Verification results

Design checks passed on 2026-10-02: `python3 scripts/list-docs.py validate`
(342 decisions, 1304 specifications), `python3 scripts/lint-spec-files.py --all`,
`git diff --check`, and the PR documentation validator `validateCoverage` with
the planned bridge change and new work order (`status: covered`, no errors).
Referenced existing test paths were checked; the new bridge test is explicitly
planned. `git status --short` confirms six documentation artifacts, including
the two new untracked plan/order files, with no production/test edits.
Implementation is complete under the user's explicit request. The [work-order results](task-01-remove-floating-indicator.md#results) record all commands: 18 unit tests, lint/typecheck/i18n, 5 desktop and 2 phone E2E tests, public-doc/spec validators and diff checks passed. Focused fresh captures passed one additional case per viewport; desktop/phone screenshots match UI-01/UI-02 and all manifest entries resolve. External PR/review/merge evidence remains in the Kandev task plan.

## Risks

- Removing the bridge itself would also remove toast consumption and refresh effects.
- The existing shared E2E helper positively asserts/clicks the removed control;
  changing it must preserve notification and settings coverage on both projects.
- Public copy and obsolete locale keys must agree with the final presentation.
