---
status: active
system: auth
created: 2026-08-15
owners:
  - kandev
---
# Self-Actions Guard in System Users Requirements

## Overview

Settings > System > Users lets an admin change the role or status of any listed
account, including their own. The backend already enforces one guard on these
actions: an active admin cannot be demoted or disabled when no other active
admin exists (`ErrLastAdmin` -> 409). The page does not reflect that guard, so
the last active admin can click "Make member" or "Disable" on a row and only
then discover the rejection in an error toast. The buttons should show the
guard up front: greyed out exactly when the backend would reject the action.

## Requirements

### REQ-AUTH-SELF-ACTIONS-GUARD-001: Self-Actions Guard in System Users

**Intent:** Settings > System > Users lets an admin change the role or status of any listed account,
including their own. The backend already enforces one guard on these actions: an active admin cannot
be demoted or disabled when no other active admin exists (`ErrLastAdmin` -> 409). The page does not
reflect that guard, so the last active admin can click "Make member" or "Disable" on a row and only
then discover the rejection in an error toast. The buttons should show the guard up front: greyed
out exactly when the backend would reject the action.

#### Acceptance criteria

- **AC-AUTH-SELF-ACTIONS-GUARD-001.1:** On any row that is an active admin, the role toggle ("Make member") and the status toggle ("Disable") are disabled when no other active admin exists in the user list. This mirrors the backend's existing `ensureAnotherAdmin` condition exactly: another user with `role = admin` and `status = active`.
- **AC-AUTH-SELF-ACTIONS-GUARD-001.2:** All other buttons keep their current behavior. "Make admin" on member rows, "Enable" on disabled rows, and every action on rows where another active admin exists stay enabled.
- **AC-AUTH-SELF-ACTIONS-GUARD-001.3:** The API behavior is unchanged. The backend keeps its current last-admin guard for any target, including the caller's own account; self-demotion and self-disable remain possible when another active admin exists.
- **AC-AUTH-SELF-ACTIONS-GUARD-001.4:** **GIVEN** the signed-in admin is the only user with `role = admin` and `status = active`, **WHEN** the Users page renders, **THEN** the own row's role toggle ("Make member") and status toggle ("Disable") are disabled; no other row is affected.
- **AC-AUTH-SELF-ACTIONS-GUARD-001.5:** **GIVEN** the only active admin, **WHEN** the page shows another row (a member or a disabled admin), **THEN** that row's toggles remain enabled (the guard only fires on active admins).
- **AC-AUTH-SELF-ACTIONS-GUARD-001.6:** **GIVEN** at least one other user with `role = admin` and `status = active`, **WHEN** the Users page renders any active-admin row (including the own row), **THEN** both toggles on that row are enabled.
- **AC-AUTH-SELF-ACTIONS-GUARD-001.7:** **GIVEN** the last active admin calls `PATCH /api/v1/users/{self}` with `{ "role": "member" }` or `{ "status": "disabled" }`, **THEN** the API responds 409 `ErrLastAdmin` (existing behavior, unchanged).
- **AC-AUTH-SELF-ACTIONS-GUARD-001.8:** **GIVEN** an active admin with another active admin present calls `PATCH /api/v1/users/{self}` with `{ "role": "member" }`, **THEN** the API applies the change (existing behavior, unchanged).

## Migrated source detail

## Why

Settings > System > Users lets an admin change the role or status of any listed
account, including their own. The backend already enforces one guard on these
actions: an active admin cannot be demoted or disabled when no other active
admin exists (`ErrLastAdmin` -> 409). The page does not reflect that guard, so
the last active admin can click "Make member" or "Disable" on a row and only
then discover the rejection in an error toast. The buttons should show the
guard up front: greyed out exactly when the backend would reject the action.

## What

- On any row that is an active admin, the role toggle ("Make member") and the
  status toggle ("Disable") are disabled when no other active admin exists in
  the user list. This mirrors the backend's existing `ensureAnotherAdmin`
  condition exactly: another user with `role = admin` and `status = active`.
- All other buttons keep their current behavior. "Make admin" on member rows,
  "Enable" on disabled rows, and every action on rows where another active
  admin exists stay enabled.
- The API behavior is unchanged. The backend keeps its current last-admin
  guard for any target, including the caller's own account; self-demotion and
  self-disable remain possible when another active admin exists.

## API surface

No change. `PATCH /api/v1/users/:id` keeps its current semantics: role/status
normalization, the last-admin guard (409 `ErrLastAdmin`), and session/token
revocation on disable. The guard applies to any target, self included.

## Permissions

Unchanged. The route stays admin-only (`RequireRealIdentity` +
`RequireAdmin`).

## Failure modes

- Button state is computed from the last loaded user list. If the list is
  stale (an admin was demoted elsewhere since the fetch), the backend still
  rejects with 409 and the existing `usersLastAdminGuard` toast appears; the
  guard is authoritative at request time.
- No new failure mode is introduced on the backend.

## Scenarios

- **GIVEN** the signed-in admin is the only user with `role = admin` and
  `status = active`, **WHEN** the Users page renders, **THEN** the own row's
  role toggle ("Make member") and status toggle ("Disable") are disabled; no
  other row is affected.
- **GIVEN** the only active admin, **WHEN** the page shows another row (a
  member or a disabled admin), **THEN** that row's toggles remain enabled
  (the guard only fires on active admins).
- **GIVEN** at least one other user with `role = admin` and `status = active`,
  **WHEN** the Users page renders any active-admin row (including the own
  row), **THEN** both toggles on that row are enabled.
- **GIVEN** the last active admin calls `PATCH /api/v1/users/{self}` with
  `{ "role": "member" }` or `{ "status": "disabled" }`, **THEN** the API
  responds 409 `ErrLastAdmin` (existing behavior, unchanged).
- **GIVEN** an active admin with another active admin present calls
  `PATCH /api/v1/users/{self}` with `{ "role": "member" }`, **THEN** the API
  applies the change (existing behavior, unchanged).

## Out of scope

- No backend change: no new self-action rejection, no signature changes, no
  new error codes or messages.
- No change to the last-admin guard itself.
- No change to invite creation, member restrictions, or the account page.
- Button disablement only mirrors the existing guard; it is not a new
  authorization rule.
