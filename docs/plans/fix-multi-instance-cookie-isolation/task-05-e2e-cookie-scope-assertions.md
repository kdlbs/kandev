---
id: "05-e2e-cookie-scope-assertions"
title: "E2E cookie-name assertions"
status: done
wave: 3
depends_on: ["02-session-cookie-port-scope-backend", "04-workspace-cookie-port-scope-frontend"]
plan: "plan.md"
spec: "../../specs/auth/requirements/fix-multi-instance-cookie-isolation.md"
---

# Task 05: E2E cookie-name assertions

## Acceptance

- The auth e2e project asserts that the login response's `Set-Cookie` names
  the session cookie `kandev_session_<port>` (port = the fixture's backend
  port), and that after login the app renders authenticated content.
- The workspace-switch e2e spec (or a new focused spec) asserts that
  selecting a workspace in the sidebar writes `kandev-active-workspace_<port>`
  rather than the unprefixed name. **E2E covers the Kanban cookie only**: the
  existing `workspace-switch-sidebar-isolation.spec.ts` runs with Office
  disabled, so the `office-active-workspace_<port>` write is asserted at the
  unit level (task 04 sidebar test), not in E2E. An Office-enabled
  workspace-switch E2E setup is recorded as a follow-up, not a blocker.
- Both assertions fail pre-change (the cookies are unprefixed today).
- Scope limitation (from review round 1): the Playwright fixture runs one
  backend per worker (`apps/web/e2e/fixtures/backend.ts`, `restart()`
  replaces that same process), so e2e here proves the browser-observable
  cookie **naming** contract only. The two-instance behavioral regression
  (independent sessions and workspace state per port) is proven by the Go
  integration tests in tasks 02 and 03. A two-backend Playwright fixture is
  explicitly out of scope; note it as a follow-up in the task output.
- The e2e files live in `apps/web/e2e/tests/` following the existing auth
  and workspace-switch spec conventions; no new infrastructure.

## Verification

```bash
cd apps/web && pnpm e2e:raw --project=auth
# plus the workspace-switch spec's exact command from apps/web/e2e/README.md
```

## Files likely touched

- `apps/web/e2e/tests/auth/login.spec.ts` (or the auth project's login spec)
- `apps/web/e2e/tests/task/workspace-switch-sidebar-isolation.spec.ts` (or a
  sibling spec asserting cookie names)

## Dependencies

Shipped behavior of tasks 02 and 04.

## Parallelism

Sequential with task 06? No — both are Wave 3 and disjoint (e2e specs vs
docs); parallel-safe.

## Inputs

- Spec: `API surface` (cookie names observable via Set-Cookie / document
  cookie), `Scenarios` (first, second, fourth).
- Existing helpers: `apps/web/e2e/helpers/api-client.ts`,
  `apps/web/e2e/helpers/causal-waits.ts`.

## Risks

- The session cookie is HttpOnly, so assertions must read the response
  `Set-Cookie` header, not `document.cookie`.
- The auth project gates on `KANDEV_FEATURES_AUTH=true` per-spec via
  `backend.restart(env)`; the workspace-switch spec runs auth-off and only
  asserts the workspace cookie names.

## Output contract

Report the asserted cookie names, spec files, exact commands and results,
the noted two-backend-e2e follow-up, blockers/risks, then mark this task
`done` and update `plan.md`.
