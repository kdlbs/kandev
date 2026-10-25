---
id: "01-frontend-disable-last-admin"
title: "Disable last-admin toggles in users table"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/auth/requirements/self-actions-guard.md"
---

# Task 01: Disable last-admin toggles in users table

## Intent

Grey out the role and status toggles on any active-admin row when no other
active admin exists, mirroring the backend's existing last-admin guard. No
backend change.

## Inputs

- The spec `What` and `Scenarios` sections.
- `apps/web/components/settings/system/users-table.tsx` (`UsersTable`,
  `useUsersList`, `UsersTableList`, `UserRow`).
- The backend condition to mirror:
  `apps/backend/internal/auth/service_users.go` `ensureAnotherAdmin`: reject
  when no user other than the target has `role = "admin"` AND
  `status = "active"`.

## Change

- In `UsersTable` (or `UsersTableList`), derive
  `activeAdminCount = users.filter((u) => u.role === "admin" && u.status === "active").length`
  from the loaded list.
- Thread the count (or a per-row `isLastActiveAdmin` boolean) into `UserRow`.
- In `UserRow`, compute
  `isLastActiveAdmin = user.role === "admin" && user.status === "active" && activeAdminCount === 1`
  and add `disabled={isLastActiveAdmin}` to both the role toggle
  (`users-table-toggle-role`) and status toggle (`users-table-toggle-status`).
- Do not read the current user id: the guard is target-based, not caller-based,
  and applies to every row identically.
- Keep existing labels, `data-testid` attributes, and styling. The shadcn
  button variant already applies `disabled:opacity-45` and
  `disabled:cursor-not-allowed`.
- No new user-facing copy; no i18n key changes. No backend files change.

## Acceptance

- When the loaded list contains exactly one active admin, that row's role and
  status toggles carry the `disabled` attribute; all other rows' toggles do
  not.
- When the list contains two or more active admins, no row's toggles are
  disabled.
- Member rows and disabled-admin rows are never disabled by this change.
- `make typecheck` and web lint pass; `go test ./internal/auth/... -run
  TestAdminUserManagement` stays green untouched (the backend guard is
  unchanged).

## TDD sequence

Per project convention, React component markup is not unit-tested; the
behavior is asserted by the Task 02 E2E spec (run after this task). Verify by
`typecheck`/lint here and the E2E spec in Task 02.

## Verification

```bash
cd "$(git rev-parse --show-toplevel)" && make typecheck
cd "$(git rev-parse --show-toplevel)/apps/backend" && go test ./internal/auth/... -run TestAdminUserManagement -count=1
```

(The behavioral check is `pnpm e2e:run --project auth
tests/auth/users-self-actions.spec.ts` from Task 02.)

## Dependencies

None.

## Parallelism

`sequential`. Single component file; the E2E task depends on it.

## Output contract

Report the files changed, verification results, blockers, and risks. Update
this task and `plan.md` in the same conversation.

## Results

- Red: the E2E spec (written before the change) failed as expected on the
  sole-admin own row: `expect(locator).toBeDisabled()` received "enabled" for
  `[data-user-id="default-user"]` `users-table-toggle-role`.
- Green: `pnpm e2e:run --project auth tests/auth/users-self-actions.spec.ts`
  passed 1 test (7.6s) covering the sole-admin disabled state, the member-row
  enabled baseline, re-enablement after adding a second admin, and the
  unchanged 409/200 self-PATCH backend outcomes.
- `make typecheck` passed; `make lint` passed (eslint `--max-warnings 0`,
  golangci-lint 0 issues).
- `go test ./internal/auth/... -run TestAdminUserManagement -count=1` passed
  untouched, proving the backend guard is unchanged.
- Only `apps/web/components/settings/system/users-table.tsx` changed.
