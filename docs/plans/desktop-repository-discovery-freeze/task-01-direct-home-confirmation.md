---
id: "01-direct-home-confirmation"
title: "Confirm Home without a picker"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-002
acceptance_criteria:
  - AC-WORKSPACES-LOCAL-REPOSITORIES-002.15
  - AC-WORKSPACES-LOCAL-REPOSITORIES-002.4
  - AC-WORKSPACES-LOCAL-REPOSITORIES-002.13
system_design:
  - ../../specs/workspaces/system-design/local-repositories.md
---

# Task 01: Confirm Home without a picker

## Summary

Make the upgrade banner's Continue Home Discovery button save the backend
user's Home without a folder picker. Keep ordinary folder choice unchanged.

## Red test

Add a Go service test for a pending desktop migration. It must fail until a
path-free Home confirmation saves the canonical Home root and starts one
scan. Add retry and stale-request cases. Add a component test that clicks
Continue Home Discovery and proves no picker command runs.

## Implementation

- Add a narrow confirmation route and service method. Check desktop mode
  and pending migration state before resolving and adding Home. Return the
  existing root on retry without another scan. Reject a stale request when
  another root cleared the pending state.
- Add a frontend action and hook callback for the route. Replace only the
  Home banner's `FolderPicker` with a translated direct `Button`. Disable
  repeated clicks while it saves and reload the discovery snapshot.
- Keep the Choose folders and Reconnect pickers. Keep desktop backend policy
  for browser clients, including a narrow viewport.
- Update the public desktop guide to describe one-click Home confirmation.

## Acceptance

1. A pending Home banner click sends one confirmation request with no path
   and opens no native or HTTP folder picker.
2. The backend persists its canonical Home root, clears pending migration,
   and starts one scan. A retry adds no duplicate and starts no new scan.
3. A stale click after another root was chosen does not add Home. Server
   mode cannot use the endpoint.
4. Choose folders and Reconnect retain their existing picker flow.

## ASCII UI preview

See [UI-01 and UI-02](plan.md#ascii-ui-preview). The Home button stays in
the migration banner. A separate Choose folders control remains visible.

## Verification

```bash
(cd apps/backend && go test ./internal/task/service ./internal/task/handlers -run 'TestDesktopDiscovery.*Home|TestConfirmHomeDiscovery' -count=1)
(cd apps && pnpm --filter @kandev/web test components/repository-discovery-root-controls.test.tsx components/repository-discovery-controls.test.tsx lib/desktop/folder-picker.test.ts)
(cd apps/web && pnpm e2e:run tests/task/repository-discovery-consent.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-repository-discovery.spec.ts)
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/task/service/repository_discovery_state.go`
- `apps/backend/internal/task/service/repository_discovery_test.go`
- `apps/backend/internal/task/handlers/repository_handlers.go`
- `apps/backend/internal/task/handlers/*_test.go`
- `apps/web/app/actions/repository-discovery.ts`
- `apps/web/hooks/domains/workspace/use-discovery-root-actions.ts`
- `apps/web/components/repository-discovery-controls.tsx`
- `apps/web/components/repository-discovery-root-controls.tsx`
- Related frontend unit and E2E tests
- `docs/public/desktop-app.md`

## Dependencies

None. Implement the service and route before wiring the frontend button.

## Risks

The backend service currently treats `"~"` as a literal path. The new
route must resolve Home itself. The pending-state check must prevent a
stale browser tab from adding Home after a different root was selected.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/local-repositories.md)
  and [design](../../specs/workspaces/system-design/local-repositories.md).

## Results

Passed the focused Go service and handler tests, discovery-control frontend
tests (8), desktop repository-discovery E2E (4), mobile repository-discovery
E2E (4), and public-doc validation (47 pages). The desktop E2E confirmed an
empty-body Home request without invoking a native or HTTP picker. Choose
folders still uses the native picker on desktop and the HTTP picker on mobile.
