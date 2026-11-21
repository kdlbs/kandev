---
created: 2026-11-20
status: implemented
requirements:
  - REQ-WORKSPACES-ORG-UNITS-005
system_design:
  - ../../specs/workspaces/system-design/org-units.md
legacy_specs: []
---

# Workspace access settings fix plan

## Overview

Users can tell which team or department shares a workspace and how to grant
access to an individual. The workspace Overview groups both cards under
Authentication & users after sign-in. One work order covers the group, its
visibility, and the help text.

## Evidence

With authentication off, open Settings > Workspaces > a workspace > Overview.
`WorkspaceEditForm` renders Placement and Team access in all auth modes.
`useOrgUnits` requests `/api/v1/units`, which requires a real identity. The
picker then shows its empty placeholder. `useWorkspaceTeamAccess` requests the
workspace member list and the directory on mount. The screenshot shows the
resulting cards in workspace settings. The source trace establishes the missing
auth check independently of the screenshot's instance settings.

The Placement description uses “unit” without defining it. It says a move is
the only way to change access, although a move preserves direct roles and access
also changes with membership. The
[organization unit requirements](../../specs/workspaces/requirements/org-units.md#req-workspaces-org-units-005-workspace-placement)
now cover the visibility and help described by `AC-WORKSPACES-ORG-UNITS-005.6`
through `.8`.

## Scope

### In scope

- Workspace Overview visibility based on the effective authentication mode.
- Localized headings and help for unit access and roles assigned directly.
- The moving instructions in the [Team Access guide](../../public/team-access.md#moving-a-workspace).

### Out of scope

- Changes to role resolution, stored roles, workspace movement, or backend routes.
- Changes to plugin configuration, workflow settings, or the organization tree editor.

## Technical approach

Use the auth snapshot in `WorkspaceAccessSettings` to mount the Authentication &
users group and its divider together. Use `SettingsGroup` without a frame so
the two existing cards sit beneath the shared heading. A single guard also
prevents the cards' requests from running in single-user mode. Follow the
[Workspace Overview design](../../specs/workspaces/system-design/org-units.md#workspace-overview)
and keep passing the current workspace scopes to both cards.

Use the existing inline descriptions and settings controls. The group heading
matches the system authentication toggle. Its description identifies this
workspace as the scope. The Organization unit card explains who gains access
and what happens when the workspace moves. Team access explains how direct and
unit roles combine. Use the `workspaces` locale namespace. In the guide, name
the group and use a short example in place of the old Placement image.

## ASCII UI preview

### UI-01 Workspace Overview

Entry point is Settings > Workspaces > a workspace > Overview. The same cards
appear in the current desktop and phone settings scroll area.

```text
Current with authentication off        Proposed with authentication off
Resource saving                        Resource saving
--------------------------------       --------------------------------
Placement                              Delete Workspace
[Pick a unit                  v]
--------------------------------
Team access
--------------------------------
Delete Workspace

Proposed after sign-in
Resource saving
--------------------------------
Authentication & users
Manage access to this workspace for signed-in users.

  Organization unit
  Units group people by team or department. Members of the selected
  unit and its parent units can open this workspace.
  Moving the workspace can change this access. People added under
  Team access keep the role you gave them there.
  [Runtime                      v]

  Team access
  Give someone a role on this workspace. If they also have a role
  through an organization unit, their highest role applies.
  Members
  [Select a person v] [Collaborator v] [Add]
--------------------------------
Delete Workspace
```

The signed-in view shows an owner before a move. The current unit appears in
the picker when known. On a phone the help wraps above the existing full-width
picker. The member controls follow the wrapping layout in the current Team
access card. The page remains the scroll owner. The visible help and conditional
sections are required by `.6`–`.8`. Spacing and “Runtime” are illustrative.

## Tests

- Workspace Overview integration coverage mounts the page with authentication
  disabled and synthetic owner scopes. The baseline fails
  `hidesWorkspaceAccessSettingsWhenAuthenticationIsDisabled` because both cards
  mount and request access data. The signed-in case checks that both cards sit
  under Authentication & users. It checks the help and uses the real workspace
  scopes to show or disable actions. Covers `.6`–`.8`.
- Keep the existing `WorkspacePlacementCard` move and rollback checks. The
  workspace save and delete suites must continue to pass after their fixtures
  stop waiting for the access requests on the disabled path.

## E2E tests

- `auth/workspace-access-settings.spec.ts` in `auth` covers both auth modes on
  desktop. The owner sees the heading and both explanations, then moves the
  workspace to Runtime using the unit picker.
- `auth/mobile-workspace-access-settings.spec.ts` in `mobile-chrome` covers the
  same Overview modes. With auth enabled, the owner can read the help and move
  the workspace by touch. With auth disabled, the delete card follows
  resource saving. Both flows cover `.6`–`.8`.

## Work orders

- [x] [Task 01. Clarify and gate workspace access settings](task-01-workspace-access-settings.md)

## Verification results

- The specification catalog, linter, and its 36 tests passed. The local PR
  documentation validator reported `covered` for the planned source change.
- The workspace save and placement suites passed all 23 baseline tests. The
  save fixture waits for all three access reads with authentication off.
- All 37 focused frontend tests passed. The desktop and phone Playwright
  projects passed two cases each. Both proved the hidden section with auth off
  and a successful move through the picker after sign-in.
- Typecheck, changed-file lint, locale checks, and public docs validation passed.

## Risks

- Disabled mode can still supply a synthetic user and owner scopes. Use the
  effective auth mode to distinguish it from signed-in mode.
- The current role resolver checks ancestor memberships for personal units too.
  The help explains the effect of placement and direct roles without promising
  complete privacy based on a unit name.
