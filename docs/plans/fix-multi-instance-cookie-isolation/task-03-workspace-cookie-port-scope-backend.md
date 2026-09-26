---
id: "03-workspace-cookie-port-scope-backend"
title: "Port-scope workspace cookie reads (backend)"
status: done
wave: 2
depends_on: ["01-shared-port-cookie-helper"]
plan: "plan.md"
spec: "../../specs/auth/requirements/fix-multi-instance-cookie-isolation.md"
---

# Task 03: Port-scope workspace cookie reads (backend)

## Acceptance

- `readActiveWorkspaceCookie(req)` in
  `apps/backend/internal/backendapp/boot_state_routes.go` (not
  `boot_state.go`) reads **only the general family**: scoped
  `kandev-active-workspace_<port>` first, then the legacy unprefixed general
  name — the office-family cookies are removed from this reader. This
  aligns the backend generic boot with the frontend generic reader (which
  never consults office cookies): with only an office cookie present, both
  sides resolve settings/first instead of the backend picking an office
  workspace. Value validation (`firstValidID` against the instance's
  workspace set) is unchanged.
- New `readOfficeWorkspaceCookie(req)` reads **only the office family**:
  scoped `office-active-workspace_<port>` first, then the legacy unprefixed
  office name — never the general cookie. Parity tests: backend and
  frontend generic readers ignore office-family cookies; the office reader
  ignores the general family.
- **Office resolver-specific precedence** (fixes a pre-existing gap on the
  touched surface): `officeWorkspaces` today passes ONE value
  (`readActiveWorkspaceCookie(req)`) into `resolveActiveOfficeWorkspaceID`,
  so when the general cookie names a kanban workspace the retained office
  cookie is never consulted and office boot falls back to the first office
  workspace. Add `readOfficeWorkspaceCookie(req)` (office-scoped first, then
  legacy office name — never the general cookie) and pass THREE named
  candidates into the office resolver — general, office, **settings** — each
  validated against the office workspace set: general (when it names an
  office workspace) → office cookie → settings → first office workspace. The
  settings candidate is wired explicitly: `officeWorkspaces` loads
  `b.userSettings(ctx)` (the same source `settingsWorkspaceID` uses, via
  `b.p.userCtrl.GetUserSettings`) and passes `settings.Settings.WorkspaceID`.
  The kanban/settings resolvers keep `readActiveWorkspaceCookie` as-is.
  **Scope of the precedence contract**: `route` is a client/page-only
  candidate — the backend boot payload and server layout are **cookie-first
  by design** (no query-param candidate; `officeWorkspaces` does not read
  query params today and that is unchanged). The route-first behavior
  belongs to the frontend page bootstrap (`getActiveWorkspaceId`) and the
  client `OfficeRoutes` effect, which converges the store after the
  cookie-first server paint; the ADR 2026-08-15 query-param exception is
  scoped to those page/bootstrap paths. Backend office resolution passes
  exactly the three candidates (general, office, settings).
- Regression tests, failing pre-change
  (`boot_state_workspace_test.go` / `helpers_test.go`): a request with Host
  `127.0.0.1:8443` carrying `kandev-active-workspace_8443=<id>` returns that
  id; carrying only the unprefixed `kandev-active-workspace=<other-id>`
  falls back only when no scoped cookie exists. **Family separation**: the
  general reader ignores `office-active-workspace_8443` and the legacy
  office name (pre-change the backend generic reader consults the office
  name — fails pre-change); the office reader ignores the general family.
- Per-Host isolation: **model the browser jar** — each request carries BOTH
  scoped cookies (`kandev-active-workspace_8443` = distinct non-first
  workspace A and `kandev-active-workspace_9443` = distinct non-first
  workspace B), and Host `127.0.0.1:8443` resolves A while Host
  `127.0.0.1:9443` resolves B. Failing-before: pre-change the reader ignores
  all scoped names, so both requests resolve the first workspace — with
  non-first A/B the assertion fails. (A request carrying only the other
  port's cookie would pass pre-change too, so that shape is not the
  regression.)
- **Downstream boot-state/resolver regression** (scenario 3, observable
  behavior). Two classes: **(invariant/back-compat, pass both sides)** with
  no scoped cookie present, a **valid** unprefixed legacy workspace id is
  selected by the boot-state resolver (`firstValidID` picks it — current
  behavior, guarded against regression) and a **foreign or invalid** legacy
  id falls through to the instance-owned fallback (first workspace) — also
  current behavior; **(failing-before)** when both a scoped and a legacy
  cookie exist, the scoped value wins (pre-change the scoped name is never
  read). A raw-reader-only test could pass while boot-state selection
  mishandles fallback, so the assertions run through
  `bootPayload`/`readActiveWorkspaceCookie` + resolution, not just the raw
  reader.
- **Downstream generic-route regression** (family separation, observable):
  table-driven full boot-state tests through `bootPayload`/`bootInitialState`
  over the generic routes **that resolve an active workspace id** (`webapp.RouteName`: Unknown, Home, Tasks, Settings, GitHub, GitLab, Jira, Linear, Stats — Office and TaskDetail excluded) with only a scoped `office-active-workspace_<port>` cookie and with only a legacy office cookie: the resolved active id is settings/first, never the office id; assert `initialState` and `routeData` where applicable, and cover the quick-chat boot fallback (`boot_state.go` quick-chat path) separately. Classification: the **legacy-office row fails before the change** (the old generic reader consults the legacy office name); the **scoped-office row is an invariant guard** (the scoped name never existed pre-change — it already fell through) and is paired with the legacy row or another changed call-site assertion for the failing-before signal. A raw reader test alone does not prove the call sites in `boot_state_routes.go` and `boot_state.go` stopped selecting office ids.
- **TaskDetail is asserted separately, not in the settings/first matrix**:
  `bootInitialState` has no task-detail workspace resolver — task detail state is emitted as `routeData` by `taskDetailRouteData` and quick-chat deliberately follows the task's workspace, so there is no activeId/settings-or-first value to assert for a normal TaskDetail request. The test asserts task-detail `routeData`/task workspace is task-derived (not cookie-derived) and the quick-chat boot follows the task's workspace with only office cookies present; the missing-task fallback path is covered as part of the Home case, not as TaskDetail coverage.
  The office-route test (next bullet) separately asserts office cookies still select office workspaces there.
- **Office precedence regression** (through `officeWorkspaces` /
  `addOfficeRouteState`, the full contract, with REAL user settings via
  `b.userSettings`). Pre-change `readActiveWorkspaceCookie` reads only the
  **unprefixed** names and never the scoped ones, so each row's behavior
  depends on which cookie names it uses:
  - **Rows with SCOPED names (`kandev-active-workspace_8443` /
    `office-active-workspace_8443`) are all failing-before** — the scoped
    names do not exist pre-change, so the reader returns empty and
    `resolveActiveOfficeWorkspaceID` falls back to the first office
    workspace. Assert the post-change winner and **require non-first
    fixtures** (the winner must differ from the pre-change first-office
    fallback): (a) scoped general = valid kanban id + scoped office = O2 →
    O2; (b) scoped general = valid office O1 + scoped office = O2 (distinct)
    → O1; (c) scoped office = O1 + settings = different valid office O2 →
    O1; (d) scoped office absent + scoped general = valid office O1 → O1.
  - **Rows with LEGACY (unprefixed) names**: (a) legacy general = kanban +
    legacy office = O2 → O2 (failing-before — pre-change the kanban id is
    returned and the office cookie never consulted → first office); (b)
    legacy general = valid office O1 + legacy office = O2 (distinct) → O1
    (invariant — general-first both before and after); (c) legacy office =
    O1 + settings = different valid office O2 → O1 (invariant — office beats
    settings both before and after); (d) legacy office absent + legacy
    general = valid office O1 → O1 (invariant).
  - Settings must be the real `userSettings` value, not a fake.

## Verification

```bash
cd apps/backend && go test ./internal/backendapp/... && make lint
```

## Files likely touched

- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/boot_state_workspace_test.go`
- `apps/backend/internal/backendapp/helpers_test.go` (if a fixture needs the
  scoped name)

## Dependencies

`httpcookie` from task 01.

## Parallelism

Disjoint files from `task-02`; parallel-safe after Wave 1.

## Inputs

- Spec: `What` (workspace cookie scoping + legacy fallback), `Scenarios`
  (second and third).
- Plan: `Backend > Workspace cookie scoping`.
- Existing pattern: `readActiveWorkspaceCookie` currently iterates
  `[]string{activeWorkspaceCookie, legacyOfficeWorkspaceCookie}`; the
  amended design removes the office name from the generic reader (parity
  with the frontend) and adds `readOfficeWorkspaceCookie` for the office
  family.

## Risks

- Keep the legacy fallback read-only (never write unprefixed names); new
  writes are the frontend task's job.
- The office boot-state path now resolves with named candidates —
  `resolveActiveOfficeWorkspaceID(officeItems, generalCookieID,
  officeCookieID, settingsID)` using `readActiveWorkspaceCookie` +
  `readOfficeWorkspaceCookie` + `b.userSettings` — never
  `resolveActiveOfficeWorkspaceID(officeItems, readActiveWorkspaceCookie(req))`
  (which dropped the office cookie entirely). Preferring the scoped
  `office-active-workspace_<port>` value inside the office resolver is
  preserved.

## Output contract

Report the scoped read behavior, changed files, exact commands and results,
blockers/risks, then mark this task `done` and update `plan.md`.
