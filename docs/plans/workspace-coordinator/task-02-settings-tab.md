---
id: "02-settings-tab"
title: "Coordinators settings tab"
status: pending
wave: 2
depends_on:
  - "01-shared-interface"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COORDINATORS-004
acceptance_criteria:
  - AC-COORDINATOR-COORDINATORS-004.1
  - AC-COORDINATOR-COORDINATORS-004.2
  - AC-COORDINATOR-COORDINATORS-004.3
  - AC-COORDINATOR-COORDINATORS-004.4
  - AC-COORDINATOR-COORDINATORS-004.5
  - AC-COORDINATOR-COORDINATORS-004.6
  - AC-COORDINATOR-COORDINATORS-004.7
system_design:
  - ../../specs/coordinator/system-design/coordinators.md
---

# Task 02: Coordinators Settings Tab (WP-1b)

## Summary

Add the Coordinators workspace settings tab and the coordinator page (UI-06) on
task 01's client. Runs in parallel with tasks 03, 04 and 07.

## In scope

- Tab in `lib/settings/workspace-settings-tabs.ts` after **Secrets** with the
  flag parameter; routes in `src/settings-routes.tsx`; list and coordinator
  pages in `app/settings/workspace/[id]/coordinators/`; the settings discovery
  entry; six locales.
- Save through `useSettingsSaveContributor`; delete through the shared
  confirmation dialog; reader gating through `canManageWorkspace` from
  `useWorkspaceTeamAccess`.
- CLI-passthrough profiles listed disabled with their reason.
- The settings-page half of `AC-COORDINATOR-COORDINATORS-005.1` to `005.3`
  (owned by task 03): warnings under the Agent profile and Executor fields
  mapped from task 01's `agent_profile_status` and `executor_profile_status`
  as the
  [coordinators design](../../specs/coordinator/system-design/coordinators.md#validation)
  tables say. A component test covers agent `missing`, agent `passthrough`
  (the passthrough message, not the removed one), executor `missing`, and
  agent `passthrough` with executor `missing` (each under its own field).
- **Open** links unconditionally to the Coordinator route
  `/workspaces/:id/coordinator/:coordinatorId` (declared in the
  [needs-you design](../../specs/coordinator/system-design/needs-you.md#screens),
  built by task 04). A component test asserts the href regardless of whether
  task 04 has merged into this branch; the route itself is task 04's to build.

## Out of scope

- Conversation, sidebar, Coordinator screens and copilot (tasks 03 to 06).

## ASCII UI preview

From [plan UI-06](plan.md#ui-06-the-coordinators-settings-tab):

```text
home > Settings > Workspaces > Software Factory > Coordinators
Overview Repositories Workflows Canvases Integrations Automations Secrets [Coordinators]
(o) Coordinators                                            [+ Add coordinator]
Coordinators read this workspace's boards, tell you what needs you and why, and propose work you approve.
+--------------------------------------+
| Planner                              |
| Agent profile: Claude . worktree     |
| Relay ships consent features; ...    |
| [Open]  Configure                    |
+--------------------------------------+
Configure / + Add coordinator -> the coordinator's page:
  < All coordinators
  Name [____]  Agent profile [v]  Executor [v]  Context [____]
  Note: the profile's auto-approve is ignored for coordinators.
  new: [Add coordinator]     existing: settings save bar (Discard, Save) + [Delete coordinator]
```

## Mockup screenshots and scenarios

Screenshots (visual reference; the acceptance criteria govern):

- [`docs/plans/workspace-coordinator/assets/p1-04-settings-coordinators-list.png`](assets/p1-04-settings-coordinators-list.png)
- [`docs/plans/workspace-coordinator/assets/p1-06-settings-coordinator-editor.png`](assets/p1-06-settings-coordinator-editor.png)

Mockup scenario specs to port (in the workspace-coordinator analysis
mockup's `mockup/e2e/tests/`, outside this repository; see the plan's [Mockup scenario to repo test](plan.md#mockup-scenario-to-repo-test)):

- `18-v21-copilot-anywhere.spec.ts`, settings test: add, edit, delete and reader permissions, ported to `apps/web/e2e/tests/coordinator/settings.spec.ts`.

## Acceptance

- Coordinators are added, edited through the save bar and deleted through the
  confirmation dialog; validation errors show on their fields.
- With the flag off the tab is absent.
- A `workspace.read` member sees the list without Add, and the page with
  disabled fields and no Save or Delete.
- At 390px the list and page stack in one column with 44px targets.

## Verification

```bash
cd apps/web && pnpm test -- lib/settings/workspace-settings-tabs.test.ts app/settings/workspace
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/settings.spec.ts
cd apps/web && pnpm e2e:run --project=auth tests/coordinator/settings-reader.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/coordinator/settings.spec.ts
```

## Likely files

- `apps/web/lib/settings/workspace-settings-tabs.ts` and test
- `apps/web/src/settings-routes.tsx`
- `apps/web/app/settings/workspace/[id]/coordinators/`
- `apps/web/lib/settings-discovery/catalog/workspaces.ts`
- `apps/web/src/locales/*/`
- `apps/web/e2e/tests/coordinator/settings.spec.ts`, `settings-reader.spec.ts`

## Dependencies

- Task 01 (flag, CRUD routes, profile statuses, client). While G0 is open the
  branch starts from task 01's branch and rebases onto main after each
  predecessor merges.

## Risks

- The reader case needs the `auth` project with `KANDEV_FEATURES_AUTH=true`;
  without it the local user holds every scope and the test proves nothing.
