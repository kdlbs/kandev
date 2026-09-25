# Task-link migration inventory

Source snapshot: 2026-09-21. Re-run the rule over the whole production tree;
this literal search is evidence, not an exhaustive static analysis.
Classify bare route prefixes as recognizers, links.ts as authority, and the rest
as builders/consumers. API-relative `/tasks/...` paths require transport-aware
classification. Do not rewrite API contracts.

Task 02 must record every migrated caller and its changed test. Before its
`vitest related` command, write `/tmp/kandev-task-link-changed-sources.txt` with
one web-package-relative changed source path per line (no test paths). These
repository paths have no spaces. Use the explicit resulting test list in the
work-order Results; run changed test files directly if related discovery misses
one. The temporary file is a command input, not a persistent artifact.

- `apps/web/components/linear/linear-quick-task-launcher.tsx`
  - `87: if (meta?.autoFocus !== false) router.push(/tasks/${task.id});`

- `apps/web/components/automations/runs-section.tsx`
  - `329: onNavigate={(id) => router.push(/tasks/${id})}`

- `apps/web/components/runs/runs-list-page.tsx`
  - `234: onOpen={(taskId) => router.push(/tasks/${taskId})}`

- `apps/web/components/runs/runs-page-client.tsx`
  - `194: onOpen={(taskId) => router.push(/tasks/${taskId})}`

- `apps/web/components/task/task-session-sidebar.tsx`
  - `314: !!pathname && (pathname.startsWith("/t/") || pathname.startsWith("/office/tasks/"));`
  - `469: !!pathname && (pathname.startsWith("/t/") || pathname.startsWith("/office/tasks/"));`

- `apps/web/components/task/task-dependency-chip.tsx`
  - `163: href={/tasks/${entry.id}}`

- `apps/web/components/app-sidebar/app-sidebar.tsx`
  - `37: matches: (p) => p.startsWith("/t/"),`

- `apps/web/components/inbox-history/inbox-history-row.tsx`
  - `35: if (!bundle.session_id) return /t/${bundle.task_id};`
  - `36: return /t/${bundle.task_id}?sessionId=${encodeURIComponent(bundle.session_id)};`

- `apps/web/components/azure-devops/azure-devops-task-launcher.tsx`
  - `189: if (meta?.autoFocus !== false) router.push(/tasks/${task.id});`

- `apps/web/components/needs-you-inbox/needs-you-inbox-row.tsx`
  - `170: if (!bundle.session_id) return /t/${bundle.task_id};`
  - `171: return /t/${bundle.task_id}?sessionId=${encodeURIComponent(bundle.session_id)};`

- `apps/web/components/needs-you-inbox/failed-inbox-row.tsx`
  - `19: return /t/${row.task_id};`

- `apps/web/components/settings/workspace-canvases-page.tsx`
  - `107: router.push(/t/${encodeURIComponent(response.task_id)}${query});`

- `apps/web/components/settings/canvas-host-route.tsx`
  - `378: router.push(/t/${encodeURIComponent(response.task_id)}${query});`

- `apps/web/components/settings/kubernetes-session-utils.ts`
  - `89: return /t/${encodeURIComponent(taskId)};`

- `apps/web/components/jira/my-jira/quick-task-launcher.tsx`
  - `65: if (meta?.autoFocus !== false) router.push(/tasks/${task.id});`

- `apps/web/components/task/simple/OfficeSimplePane.tsx`
  - `145: href={/t/${task.id}}`

- `apps/web/components/automations/trigger-configs/plugin-webhook-controls.tsx`
  - `84: <a className="underline" href={/tasks/${receipt.task_id}}>`

- `apps/web/components/github/my-github/quick-task-launcher.tsx`
  - `239: if (meta?.autoFocus !== false) router.push(/tasks/${task.id});`

- `apps/web/lib/links.ts`
  - `2: const base = /t/${taskId};`
  - `30: const TASK_DETAIL_PREFIXES = ["/t/", "/tasks/"];`

- `apps/web/lib/entity-references/message-references.ts`
  - `60: return reference.url === /t/${encodeURIComponent(reference.id)};`

- `apps/web/app/tasks/[id]/kanban-task-shell.tsx`
  - `156: const href = target === "office" ? /office/tasks/${taskId} : /t/${taskId};`

- `apps/web/app/tasks/[id]/page.tsx`
  - `29: redirect(queryString ? /t/${id}?${queryString} : /t/${id});`

- `apps/web/app/office/tasks/[id]/task-advanced-mode.tsx`
  - `51: href={/t/${task.id}}`

- `apps/web/src/spa-routes.tsx`
  - `746: for (const prefix of ["/t/", "/tasks/"]) {`

## Task 02 migration result

All builders listed above now use `linkToTask` with raw task IDs. Direct task
anchors use `TaskLink`; Office, API, listing, and route-recognition paths keep
their separate contracts. The following callers changed:

- `components/automations/runs-section.tsx` → `components/automations/runs-section.test.tsx`
- `components/runs/runs-page-client.tsx` → `components/runs/runs-page-client.test.tsx`
- `components/runs/runs-list-page.tsx` → `components/runs/runs-list-page.test.tsx`
- `components/azure-devops/azure-devops-task-launcher.tsx` → `components/azure-devops/azure-devops-task-launcher.test.tsx`
- `components/github/my-github/quick-task-launcher.tsx` → `components/github/my-github/quick-task-launcher.test.tsx`
- `components/linear/linear-quick-task-launcher.tsx` → existing launcher contract retained; no path-specific assertion needed
- `components/jira/my-jira/quick-task-launcher.tsx` → existing launcher contract retained; no path-specific assertion needed
- `components/automations/trigger-configs/plugin-webhook-controls.tsx` → existing rendered link contract retained
- `components/task/simple/OfficeSimplePane.tsx` → existing cross-link contract retained
- `app/office/tasks/[id]/task-advanced-mode.tsx` → existing cross-link contract retained
- `app/tasks/[id]/kanban-task-shell.tsx` → existing Office/Kanban cross-link contract retained
- `app/tasks/[id]/page.tsx` → compatibility redirect contract retained
- `components/inbox-history/inbox-history-row.tsx` → existing session query contract retained
- `components/needs-you-inbox/needs-you-inbox-row.tsx` → existing session query contract retained
- `components/needs-you-inbox/failed-inbox-row.tsx` → existing task href contract retained
- `components/settings/system/bundle-customizer.tsx` → existing alternate-target contract retained
- `components/settings/workspace-canvases-page.tsx` → existing canvas session context retained
- `components/settings/canvas-host-route.tsx` → existing canvas session context retained
- `components/settings/kubernetes-session-utils.ts` → existing task href helper contract retained
- `lib/entity-references/message-references.ts` → existing encoded entity-reference contract retained
- `components/task/task-dependency-chip.tsx` → `components/task/task-dependency-chip.test.tsx`

The enforcement rule and real-config wiring are covered by
`eslint-rules/no-task-link-bypass.test.ts` and
`scripts/lib/task-link-wiring.test.ts`.
