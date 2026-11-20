---
id: "01-workspace-access-settings"
title: "Clarify and gate workspace access settings"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-ORG-UNITS-005
acceptance_criteria:
  - AC-WORKSPACES-ORG-UNITS-005.6
  - AC-WORKSPACES-ORG-UNITS-005.7
  - AC-WORKSPACES-ORG-UNITS-005.8
system_design:
  - ../../specs/workspaces/system-design/org-units.md
---

# Task 01. Clarify and gate workspace access settings

## Summary

Group Organization unit and Team access under Authentication & users on the
workspace Overview. Explain who can open the workspace and how the two ways of
sharing it work.

## In scope

- Gate the Authentication & users group and its divider in
  `WorkspaceAccessSettings` using the authenticated mode in the store. Keep the
  existing scope checks on actions.
- Use the proposed headings and descriptions in [UI-01](plan.md#ui-01-workspace-overview).
  Localize the picker text, help, and title. Use the existing locale workflow,
  including `pnpm run i18n:zh-hant` for Traditional Chinese.
- Update the moving instructions and replace the obsolete Placement image in
  the [Team Access how-to guide](../../public/team-access.md#moving-a-workspace)
  with the Runtime example.

## Out of scope

- Changes to backend access checks or data, account provisioning, and settings on other pages.

## Acceptance

1. With authentication off, the heading, both cards, and the divider are absent. The
   workspace Overview makes no unit, membership, or directory requests for them.
2. A signed-in user sees both cards under Authentication & users on desktop and
   phone. The heading describes workspace access. The user's workspace
   permissions determine which actions are available.
3. The rendered help distinguishes access through units from direct roles and
   states what a move changes. Both descriptions remain visible on a phone.

## ASCII UI preview

### UI-01 Workspace Overview

Excerpt from the [full preview](plan.md#ui-01-workspace-overview), covering `.6`–`.8`.

```text
Authentication off                    Signed in
Resource saving                       Resource saving
--------------------------------      --------------------------------
Delete Workspace                      Authentication & users
                                      Manage access to this workspace
                                      for signed-in users.

                                        Organization unit
                                        [Runtime                  v]

                                        Team access
                                        [Existing member controls]
                                      --------------------------------
                                      Delete Workspace
```

The phone uses the same scroll area with wrapping help and member controls. Use
the existing workspace Overview settings layout and touch sizing.

## Verification

First run `hidesWorkspaceAccessSettingsWhenAuthenticationIsDisabled` on the
baseline. The test must reach the Overview, then fail on card presence and
access reads. Reuse the real store in the save test helper and allow the auth
snapshot to vary. Flush the mount effects before asserting that no requests ran.
Exercise signed-in manage and read-only scopes. Remove the obsolete access
read waits from the save and delete fixtures.

The Playwright checks use isolated databases and the fixtures' backend restart
methods. Set the auth flag per mode. Use the Pixel 5 viewport for the phone test.
Check that the group heading appears only after sign-in and that both cards
appear under it. Check that the help fits without horizontal overflow and that
the owner can move the workspace by touch. Build with the desktop command and
reuse the same artifacts for the phone command.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run app/settings/workspace/workspace-access-settings.integration.test.tsx app/settings/workspace/workspace-edit-save.integration.test.tsx app/settings/workspace/workspace-delete-catalogue.integration.test.tsx components/settings/workspaces/workspace-placement-card.test.tsx)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet && pnpm run typecheck)
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project auth tests/auth/workspace-access-settings.spec.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/auth/mobile-workspace-access-settings.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/web/app/settings/workspace/workspace-edit-client.tsx`
- `apps/web/app/settings/workspace/workspace-access-settings.integration.test.tsx`
- `apps/web/app/settings/workspace/workspace-edit-save.test-helpers.tsx`
- `apps/web/app/settings/workspace/workspace-delete-catalogue.test-helpers.tsx`
- `apps/web/components/settings/workspaces/workspace-placement-card.tsx`
- `apps/web/components/settings/workspaces/workspace-team-access-card.tsx`
- `apps/web/src/locales/*/workspaces.json`
- `apps/web/e2e/tests/auth/workspace-access-settings-tests.ts`
- `apps/web/e2e/tests/auth/workspace-access-settings.spec.ts`
- `apps/web/e2e/tests/auth/mobile-workspace-access-settings.spec.ts`
- `docs/public/team-access.md`

## Dependencies

None.

## Risks

- Test the disabled mode with owner scopes. A permission check alone also
  returns true for the synthetic user.
- A role inherited from a parent unit can exceed a role listed in Team access.
  Use the highest-role rule in the help.

## Parallelism

`sequential`

## Inputs

- [Workspace placement requirements](../../specs/workspaces/requirements/org-units.md#req-workspaces-org-units-005-workspace-placement).
- [Workspace Overview design](../../specs/workspaces/system-design/org-units.md#workspace-overview).
- `apps/web/e2e/tests/auth/org-units.spec.ts` and
  `apps/web/e2e/tests/auth/mobile-users-self-actions.spec.ts` for fixture setup.

## Results

- The new integration cases and both browser specs failed on the old UI. The
  cards rendered with auth off. The shared heading was missing after sign-in.
- Dependency install passed. All 37 tests across the four focused Vitest suites
  passed. The integration cases also verified the access requests and scopes.
- The managed desktop runner built the updated backend and SPA. The initial
  run exposed a fixture assumption about workspace creation. The final desktop
  run reused that build with `--no-build` and passed both cases. The phone run
  used the same frontend build and passed both cases.
- `pnpm run typecheck`, focused Oxlint, formatting, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` passed. Both public docs validators, the specification
  catalog and linter, and `git diff --check` passed.
- The review follow-up added a reload assertion for saved unit placement. The
  desktop and phone specs each passed both cases. Splitting the access settings
  into `workspace-access-settings.tsx` keeps the workspace editor at 584 lines;
  focused Oxlint and typecheck passed.
- The focused frontend run passed 178 tests across workspace access, save,
  delete, placement, and transcript settings.
