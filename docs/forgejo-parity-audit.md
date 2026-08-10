# Forgejo parity audit

This is the delivery audit for the Forgejo integration. GitLab remains the
behavioral reference, but Forgejo is deliberately implemented through its REST
v1 API rather than through GitLab adapters.

## Implemented

- Workspace-scoped PAT configuration, health checks, repository discovery, and
  a queue of open issues and pull requests.
- Task issue/PR links, refresh and unlink operations, PR creation, mergeability,
  Forgejo Actions branch rollups, commits, files, comments, reviews, approvals,
  and change requests.
- Issue and review watches with background polling, task creation,
  deduplication, workflow context, and inflight limiting for issue watches.
- Signed webhook verification, delivery replay protection, and polling fallback.
- Saved review/action presets, settings management, and a dedicated queue page.

## Intentional provider differences

- No GitLab-style merge-request subscription state is persisted: Forgejo
  notifications/subscriptions are instance/user policy and are not a reliable
  per-PR REST parity surface across supported Forgejo versions.
- Forgejo Actions runner administration is out of scope. Kandev displays run
  outcomes only.
- Kandev does not automatically close Forgejo issues or map Kanban columns back
  to provider state.

## Delivery evidence

- The settings form participates in the shared settings-save coordinator; the
  Forgejo Zustand slice and WebSocket handlers refresh workspace watches,
  presets, queue data, and per-task links without a full-page reload.
- Unit coverage covers the Forgejo state slice, WebSocket handlers, API hooks,
  queue task creation/linking, task-link controls, and PR detail actions.
- Playwright coverage covers workspace-scoped connection state, mobile settings
  layout, issue-watch save/poll workflow context, queue linking, and task PR
  branch prefill. The test uses mocked Forgejo responses while exercising the
  real Kandev backend and browser UI.
- Backend coverage includes REST client contracts, workspace authorization,
  watch task dedupe/inflight behavior, poller lifecycle, signed webhook replay
  protection, task-link refresh, and the durable `forgejo_issue_imports`
  one-task-per-issue claim.

## Validation command set

Run the provider-focused checks from the repository root:

```bash
cd apps/backend
go test ./internal/forgejo ./internal/gateway/websocket ./internal/backendapp

cd ../web
pnpm exec tsc --noEmit
pnpm exec vitest run app/forgejo/forgejo-page-client.test.tsx \
  components/forgejo/forgejo-task-links-button.test.tsx \
  hooks/domains/forgejo lib/state/slices/forgejo/forgejo-slice.test.ts \
  lib/ws/handlers/forgejo.test.ts
pnpm exec playwright test --config e2e/playwright.config.ts \
  e2e/tests/integrations/forgejo-settings.spec.ts
```
