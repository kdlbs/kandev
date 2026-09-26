---
spec: docs/specs/auth/requirements/self-actions-guard.md
created: 2026-08-15
updated: 2026-08-15
status: complete
---

# Implementation Plan: Self-Actions Guard in System Users

## Overview

Settings > System > Users lets an admin change the role or status of any listed
account, including their own. The backend already rejects demoting or disabling
the last active admin (409 `ErrLastAdmin`), but the page shows the toggle
buttons as enabled, so the rejection surfaces only as a toast after the click.
The fix greys out exactly those buttons the backend would reject: on any row
that is an active admin, the role toggle ("Make member") and status toggle
("Disable") are disabled when no other active admin exists.

This is a UI-only change. The backend keeps its current last-admin guard
verbatim; self-demotion and self-disable remain possible while another active
admin exists, exactly as today.

## Current behavior

- Backend: `ensureAnotherAdmin` in
  `apps/backend/internal/auth/service_users.go` rejects a change that would
  leave zero users with `role = admin` and `status = active` (excluding the
  target). `AdminSetRoleStatus` applies it to any target, self included; the
  `updateUser` handler documents that self-demotion/disable is allowed when
  another active admin remains. This behavior is correct and stays unchanged.
- Frontend: `UserRow` in
  `apps/web/components/settings/system/users-table.tsx` renders the role and
  status toggle buttons for every row with no guard awareness. The user list
  (`useUsersList`) is the same data source the backend guard scans
  (`ListUsers`), so the disabled state can be derived from the rendered rows.
- E2E: auth-enabled specs live in `apps/web/e2e/tests/auth/*.spec.ts`, run in
  the `auth` Playwright project (`KANDEV_FEATURES_AUTH=true` backend restart;
  helpers in `e2e/helpers/auth.ts`). No functional spec exists for the Users
  page today (only `auth-screenshots.spec.ts`).

## Frontend

- `apps/web/components/settings/system/users-table.tsx`:
  - Derive `activeAdminCount` from the loaded users:
    `users.filter((u) => u.role === "admin" && u.status === "active").length`,
    mirroring `ensureAnotherAdmin` exactly.
  - In `UsersTableList` / `UserRow`, disable a row's role and status toggles
    when the row is an active admin and `activeAdminCount === 1`
    (`isLastActiveAdmin`). This is precisely the set of actions the backend
    rejects today (demote or disable of the last active admin); every other
    button stays enabled.
  - No current-user identity is needed: the guard is target-based, not
    caller-based, and the own row is just an active-admin row.
  - Keep existing labels and `data-testid` attributes
    (`users-table-toggle-role`, `users-table-toggle-status`). Shadcn button
    already styles the disabled state
    (`disabled:opacity-45 disabled:cursor-not-allowed`).
  - No new user-facing copy; no i18n key changes.

## Tests

- No backend tests change: the backend is untouched.
- No frontend unit tests: the change is button state in component markup,
  which this repo does not unit-test. The behavior is covered by the E2E
  spec below plus the existing `TestAdminUserManagement` backend guard tests
  (which stay green and prove the guard still exists).

## E2E

- New `apps/web/e2e/tests/auth/users-self-actions.spec.ts` in the `auth`
  project: serial describe, `backend.restart({ KANDEV_FEATURES_AUTH: "true" })`
  in `beforeAll`, restart to baseline in `afterAll` (pattern from
  `auth-screenshots.spec.ts`).
- Setup admin + login via `e2e/helpers/auth.ts`; create a member via
  `POST /api/v1/users` with the admin context (`context.request` shares the
  session cookie).
- Assertions:
  - Sole admin (fresh setup): own row (`[data-user-id="{adminId}"]`) has both
    toggles `toBeDisabled()`; the member row has both `toBeEnabled()`.
  - Existing guard proof: `PATCH /api/v1/users/{self}` with
    `{ "role": "member" }` as the sole admin returns 409 `ErrLastAdmin`
    (backend unchanged).
  - Create a second active admin via the API, reload the page: the own row's
    toggles are now `toBeEnabled()` (guard passes; button state tracks it).
  - No-new-behavior proof: `PATCH /api/v1/users/{self}` with
    `{ "role": "member" }` now returns 200 (self-demotion stays allowed when
    another active admin exists).
- Run: `cd apps/web && pnpm e2e:run --project auth
  tests/auth/users-self-actions.spec.ts`.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Disable last-admin toggles in users table](task-01-frontend-disable-last-admin.md)

Wave 2:

- [x] [Task 02: E2E coverage for guard-matched buttons](task-02-e2e-guard-mirror.md)

Execution is sequential. Task 02 depends on the frontend change from Task 01.

## Verification

Focused, per task:

```bash
cd "$(git rev-parse --show-toplevel)" && make typecheck
cd "$(git rev-parse --show-toplevel)/apps/backend" && go test ./internal/auth/... -run TestAdminUserManagement -count=1
cd apps/web && pnpm e2e:run --project auth tests/auth/users-self-actions.spec.ts
```

Final gate (per AGENTS.md): `make fmt`, then `make typecheck test lint`. E2E
is not part of `make test`; the auth-project spec runs via `make test-e2e` in
CI.

## Mobile parity

The change adds a `disabled` attribute to two buttons in the shared
`users-table.tsx` component; layout, scrolling, navigation, and touch targets
are unchanged, and the state derivation is shared markup. Mobile coverage is
still provided: `apps/web/e2e/tests/auth/mobile-users-self-actions.spec.ts`
runs in the `mobile-chrome` project (Pixel 5) with the same per-file isolated
auth database and asserts the sole-admin own row is disabled while member and
disabled-admin rows stay enabled, plus re-enablement after a second active
admin. It is routed out of the desktop `auth` project via its `testIgnore`
(`e2e/playwright.config.ts`) so it runs exactly once, on the mobile viewport.

## Recorded Results

- E2E: `cd apps/web && pnpm e2e:run --project auth
  tests/auth/users-self-actions.spec.ts` — RED first (sole-admin toggle
  enabled, expected disabled), then 1 passed after the component change.
- `make fmt` passed (unrelated pre-existing gofmt drift in four backend files
  was restored, not committed).
- `make typecheck` passed.
- `make lint` passed: golangci-lint 0 issues, eslint `--max-warnings 0`
  clean, 118 harness files passed, architecture lint passed.
- `go test ./internal/auth/... -run TestAdminUserManagement -count=1` passed
  (backend guard untouched).
- `make test` is not fully green in this environment for pre-existing
  reasons, all proven unrelated by stashing this change: harness-injected
  `KANDEV_VERSION=v0.88.0-dirty` breaks two launcher metadata tests; the host
  `~/.profile` references a missing `/tmp/cargo/env` (agentctl config test);
  two agentctl process-lifecycle tests are timing-sensitive under host load;
  `lib/http-git-server.test.ts` requires a Docker bridge gateway (no Docker
  daemon on this host); `test-scripts` needs the `unzip` binary (absent, no
  sudo to install). CI has none of these gaps.

## Risks and limits

- Button state is a mirror of the last loaded user list, not a live
  authorization decision. A stale list (an admin demoted or disabled in
  another session since the fetch) can show an enabled button whose request
  the backend rejects; the existing 409 `ErrLastAdmin` toast covers that path
  unchanged.
- The frontend count must match `ensureAnotherAdmin`'s condition exactly
  (`role = admin` AND `status = active`, excluding the target row). Any drift
  between the mirror and the guard would show either a clickable-rejected
  button or a needlessly disabled one; the E2E asserts both directions
  (sole-admin disabled, two-admin enabled).
- The `users` snapshot used for the count is the same `ListUsers` data the
  backend guard scans, so the mirror and the guard evaluate the same set
  modulo request-time staleness.
