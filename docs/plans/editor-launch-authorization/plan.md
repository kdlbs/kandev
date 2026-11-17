---
created: 2026-11-16
status: done
requirements:
  - REQ-AUTH-ROLES-AND-SCOPES-001
  - REQ-WORKSPACES-EDITOR-COMMAND-001
system_design:
  - ../../specs/auth/system-design/editor-authorization-boundary.md
  - ../../specs/workspaces/system-design/editor-command-arguments.md
legacy_specs: []
---

# Implementation plan: Editor launch authorization

## Overview

With authentication enabled, any signed-in user can create a `custom_command`
editor and then make the backend start that command on its host. This package
closes the hole without changing behavior for anyone who is meant to have
access, and without changing anything when authentication is off.

- Only callers holding `org.config.manage` (org admins and owners) can create,
  change or delete editors. Listing stays open.
- Opening an editor or folder for a session requires `session.exec` on that
  session's workspace. Viewers and callers without reach are refused.
- Placeholder values are substituted after the template is split into
  arguments, so a crafted file name cannot add or split arguments.
- The settings page hides the add, edit and remove controls for custom editors
  from non-admins, so they do not reach a 403.

Host-launching editors (built-in desktop editors, `custom_command`, open
folder) keep working with authentication on. Disabling them was considered and
rejected because it would regress installs that enable authentication on a
local machine.

Disclosure: the defect was reported privately. Implement and review this
package in the advisory's private fork, and publish the advisory together with
the release that contains it.

## Confirmed root cause

1. **Management routes lack the declared scope.**
   `apps/backend/internal/editors/handlers/handlers.go:44-53` registers all
   editor routes on a bare `router.Group("/api/v1")`. The scope registry
   (`apps/backend/internal/authz/scopes.go:61`) and
   `AC-AUTH-ROLES-AND-SCOPES-001.1` place editors under `org.config.manage`.
   Prompts, agent settings and agent capabilities apply
   `authz.RequireOrgScope(authz.ScopeOrgConfigManage)`; editors never did. The
   settings-catalog write path for editors is already correct
   (`backendapp/settings_operations.go:133-136` maps `editor.manage` to
   `org.config.manage`), so only the REST routes are affected. The scope
   completeness test (`AC-AUTH-ROLES-AND-SCOPES-001.3`) did not catch this
   because `org.config.manage` has other call sites.
2. **Open routes bypass workspace authorization.** `Service.OpenEditor` and
   `Service.OpenFolder` (`apps/backend/internal/editors/service/service.go:80`,
   `:212`) read the session straight from the task repository. No reach or
   scope check runs, so any signed-in user can open any session, and a viewer
   can trigger a host launch despite `session.exec` being the scope for
   "VS Code" in the registry (`AC-AUTH-ROLES-AND-SCOPES-001.7`).
3. **Placeholder values are re-tokenized.** `expandCommandPlaceholders`
   shell-escapes values into the template string, then `launchCommand` splits
   the whole string. For a quoted placeholder such as `code "{file}"`, a file
   named `a" --evil "b` became three arguments (`'/w/a`, `--evil`, `b'`). This
   was reproduced with a temporary test against the real functions on
   2026-11-16 and then deleted. Ordinary names in a quoted placeholder also get
   literal quote characters.

Combined effect before the fix: a workspace viewer with authentication enabled
could register `/bin/sh -c "..."` as an editor and run it on the backend host
as the backend user.

## Specification reconciliation

| Defect | Classification | Source |
| --- | --- | --- |
| 1 | Implementation violates an active criterion | `AC-AUTH-ROLES-AND-SCOPES-001.1`, `.7` in [roles and scopes](../../specs/auth/requirements/roles-and-scopes.md) (registry table: `org.config.manage` includes editors); see the [auth design](../../specs/auth/system-design/editor-authorization-boundary.md) |
| 2 | Implementation violates an active criterion | `AC-AUTH-ROLES-AND-SCOPES-001.1`, `.7` (registry table: `session.exec` includes VS Code); see the [auth design](../../specs/auth/system-design/editor-authorization-boundary.md) and backend scoping rules in `apps/backend/AGENTS.md` |
| 3 | Intended behavior missing from requirements | New [`REQ-WORKSPACES-EDITOR-COMMAND-001`](../../specs/workspaces/requirements/editor-command-arguments.md) and its [system design](../../specs/workspaces/system-design/editor-command-arguments.md) |

No ADR is needed: the scopes and the service-boundary access-checker pattern
already exist.

## Design

### Management scope (task 01)

In `editors/handlers.RegisterRoutes`, build
`manageConfig := authz.RequireOrgScope(authz.ScopeOrgConfigManage)` and mount
it on `POST /editors`, `PATCH /editors/:id` and `DELETE /editors/:id`.
`GET /editors` stays open: every user needs the list to pick a default editor
and to open files. A synthetic identity (authentication off) is unscoped and
passes, so local behavior is unchanged.

### Session execution scope (task 02)

Follow the workflow service's access-checker pattern
(`backendapp/services.go:538-547`):

- `editors/service.Service` gains
  `SetSessionAccessChecker(func(ctx context.Context, sessionID string) error)`
  and a new sentinel `ErrSessionForbidden`.
- `OpenEditor` and `OpenFolder` call the guard as their first statement after
  the empty-ID check, before any repository read, including the embedded
  VS Code early return.
- Unwired checker: allow callers with no identity or a synthetic identity;
  refuse a real identity with `ErrSessionForbidden`. A missing wire therefore
  fails closed with authentication on and changes nothing with it off.
- `backendapp/services.go` wires it next to the workflow checkers. The closure
  reads the session with `repos.Task.GetTaskSession`, then calls
  `taskSvc.AuthorizeTaskScope(ctx, session.TaskID, authz.ScopeSessionExec)`.
  It maps `taskservice.IsForbidden(err)` to `editorservice.ErrSessionForbidden`
  and other lookup/authorization errors to `editorservice.ErrWorkspaceNotFound`.
- Do not use `AuthorizeSessionScope` here: its session-execution contract also
  admits access to the session's executor configuration. Editor launches run
  on the backend host and do not use that executor. Requiring executor access
  would reject collaborators on historical sessions after their executor
  profile is deleted, and would change auth-off behavior. Task-scope
  authorization preserves workspace reach without coupling editor access to
  the executor lifecycle.
- `handlers.serviceErrorStatus` maps `ErrSessionForbidden` to 403. Missing
  reach stays 404, matching `ProcessHandlers.denySessionExecutionAccess`.

### Argument boundary (task 03)

`launchCommand` splits the template first and substitutes raw placeholder
values into each token, per the
[system design](../../specs/workspaces/system-design/editor-command-arguments.md).
`expandCommandPlaceholders` is replaced by a per-token substitution helper; its
existing test is rewritten to the new contract.

### Settings controls (task 04)

`CustomEditorsList` and `CustomEditorRow` in
`apps/web/components/settings/editors-settings.tsx` read `useIsAdmin()`. A
non-admin sees the custom editor list read-only. The default-editor picker,
supported-editor list and LSP sections are unchanged because they write user
settings, not editors. No new copy is needed.

## ASCII UI preview

`UI-01: Settings > Editors, custom editors block`. Entry point: Settings,
Editors section. Structural requirement: the Add, Edit and Remove buttons are
absent for a non-admin; rows and the empty state remain. Spacing is
illustrative. Desktop and phone share the composition: the header row stacks
on phone exactly as today (`md:flex-row`).

```text
Admin (unchanged)                         Non-admin with auth on (after)
+---------------------------------------+ +---------------------------------------+
| Custom editors   [Add custom editor]  | | Custom editors                        |
| +-----------------------------------+ | | +-----------------------------------+ |
| | Zed (command)       [Edit][Remove]| | | | Zed (command)                     | |
| +-----------------------------------+ | | +-----------------------------------+ |
+---------------------------------------+ +---------------------------------------+
```

Empty state for a non-admin keeps the existing "No custom editors yet" text
without the Add button.

## Work orders

| # | Work order | Depends on | Result |
| --- | --- | --- | --- |
| 01 | [Editor management requires org.config.manage](task-01-editor-management-scope.md) | none | Non-admins get 403 on editor create, update and delete |
| 02 | [Opening editors and folders requires session.exec](task-02-open-session-exec-scope.md) | none | Viewers get 403, callers without reach get 404 |
| 03 | [Placeholder values stay inside their argument](task-03-command-argument-boundary.md) | none | Argument count never depends on the file name |
| 04 | [Hide custom editor management from non-admins](task-04-settings-admin-controls.md) | 01 | Non-admins see a read-only list |

Implement in order 01, 02, 03, 04 in one PR. 01 to 03 are independent backend
changes; 04 only makes sense once 01 returns 403.

## Verification strategy

- Every denial test has a positive witness: the identical request as a caller
  who holds the scope succeeds. Without it a denial can pass because of a
  broken fixture.
- Every denied open asserts that no process started. Tests replace the
  process starter through a package-level seam rather than running real
  programs.
- Authentication-off behavior is covered by a synthetic-identity case in each
  backend work order.
- Mutation check: remove each new guard in turn; its test must fail and name
  itself.
- End-to-end evidence: task 01's registered-route test and task 02's
  wired-service test exercise the real HTTP routes, session lookup and
  `AuthorizeTaskScope`. A browser E2E is not needed: the only UI change is
  hiding controls, covered by a component test.

## Commands

Run from the repository root.

```bash
(cd apps/backend && go test -count=1 ./internal/editors/... ./internal/authz/...)
(cd apps/backend && CGO_ENABLED=1 go test -tags fts5 -count=1 -run 'Editor' ./internal/backendapp/...)
git ls-files --modified --others --exclude-standard -z -- 'apps/backend/**/*.go' | xargs -0 -r bash scripts/lint-go-changed
(cd apps/web && pnpm exec vitest run components/settings/editors-settings && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
(cd apps/backend && make build)
(cd apps/web && pnpm run build:vite)
(cd apps/web && node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs)
```

## Risks

- **Error-message disclosure.** The 403 for editor management names the scope,
  like prompts. That is existing, intended behavior.
- **Existing editors planted by non-admins.** Instances that ran with
  authentication on may already hold custom editors created by non-admins. The
  fix does not delete them. The release notes and advisory must tell admins to
  review Settings > Editors for unknown `custom_command` entries.
- **Admin trust.** An admin can still author any command, and anyone with
  `session.exec` can trigger it. That matches the scope model: admins already
  configure host executors, and `session.exec` already grants a shell.
- **Docker and remote executors.** `session.exec` grants a shell inside the
  executor, while an editor launch runs on the backend host. With this package
  the opener controls only argument values of an admin-authored command, which
  bounds the difference.

## Exclusions

- Disabling or restricting host-launching editors when authentication is on.
- Notification provider routes. They are user-scoped in the service and only
  start fixed programs, so they are not an execution path; aligning their
  route guard with the registry is a separate change.
- Executor routes. They already enforce ownership and admin-only host types in
  the service.
- Strengthening the scope completeness test to check per-route coverage.
- Deleting or migrating existing editors.
- Public documentation beyond the note in task 04.

## Results

All four work orders are complete. Editor management now requires
`org.config.manage`; opening an editor or folder requires `session.exec`;
custom command templates are tokenized before values are substituted; and
non-admins see custom editor settings read-only. The new editor-command
requirement is active and its system design is current. The editor access
check authorizes task reach directly, without requiring the session executor
configuration to remain present.

Verification passed: focused backend tests and real authorization wiring
tests, changed-Go lint, backend build, editor settings component tests,
frontend typecheck, i18n check, focused Oxlint, Vite production build, public
documentation validation, and specification catalog and lint validation.
The phone and desktop use the existing shared editor-settings composition, so
the work-order component test covers the only UI behavior change; no browser
E2E was required by the package's verification strategy.
