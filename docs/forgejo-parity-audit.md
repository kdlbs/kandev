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

## Remaining delivery gates

- Replace the compact Forgejo settings implementation with the shared settings
  save coordinator and a dedicated Zustand/WebSocket state slice.
- Add frontend hook/component tests and desktop/mobile Playwright coverage for
  connection, queue, watches, PR feedback, and disconnect.
- Run the suite under a Node runtime compatible with Vitest/Rolldown; the
  current local Node runtime lacks `node:util.styleText`.
