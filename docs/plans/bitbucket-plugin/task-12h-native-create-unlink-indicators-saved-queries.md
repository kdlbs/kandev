---
id: "12h-native-create-unlink-indicators-saved-queries"
title: "Close native create, unlink, task-indicator, and saved-query gaps"
status: completed
wave: 3h
depends_on: ["12g-bitbucket-status-detail-adapters"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
decision: "../../decisions/2026-08-10-plugin-change-request-mutations.md"
---

# Task 12h: Close Native Create, Unlink, Task-Indicator, and Saved-Query Gaps

## Intent

Fix four live-evaluation gaps without adding Bitbucket-specific host branches: route
native create through a registered provider after Git push, expose shared unlink,
hydrate native task indicators from one workspace association snapshot, and persist
dashboard saved queries with GitHub/GitLab interaction parity.

## Owned paths

- `apps/web/lib/plugins/{types,registry,registry-normalization}.ts` and focused tests
- `apps/backend/internal/plugins/handlers.go` and authenticated-action tests
- `apps/web/hooks/use-git-operations.ts`, `apps/web/components/vcs/**`, and focused tests
- `apps/web/components/integrations/**`, task/sidebar/Kanban list placements, and tests
- `docs/{decisions,specs,plans,public}/**` plugin contract documentation
- Attached `kdlbs/kandev-plugin-bitbucket/{ui,internal,manifest.yaml}` implementation/tests

## Implementation

1. Extend repository-provider registration with lifecycle-cancelled native change-
   request creation and declared option support. Resolve only the persisted repository
   attached to the active task/repo scope; push through `worktree.push`, then call the
   provider. Keep built-in `worktree.create_pr` behavior unchanged.
   Extend task-scoped authenticated actions with an optional repository selector that
   the host accepts only when it is attached to the verified task.
2. Extend review-provider registration with lifecycle-cancelled unlink and a normalized
   workspace association external store. Add host-owned unlink controls and semantic
   task indicators in desktop/mobile shared surfaces.
3. Register Bitbucket create/unlink/association callbacks. Server create selects the
   host-verified repository ID, records the returned association, and remains safe after
   a successful push. Link/create/unlink refresh both review and association snapshots.
4. Add `capabilities.user_state`, persist bounded workspace saved-query records through
   `host.storage`, and reuse host scope/dialog primitives for save/select/delete on
   desktop and mobile.
5. Update public plugin API docs, rebuild the plugin bundle/package, and cover complete
   behavior in host fixture and packaged plugin E2E.

## TDD and acceptance

1. RED host tests pin provider resolution, push-before-create ordering, post-push retry,
   lifecycle abort, option visibility, association dedupe, unlink, and no-plugin parity.
2. RED plugin tests pin authenticated create payloads, repository selection, link/unlink
   refresh, association snapshots, storage validation, named save/select/delete, and
   workspace isolation.
3. Desktop/mobile E2E proves conditional native Create PR, resulting association,
   sidebar/Kanban glyph, shared unlink, multi-link preservation, saved-query reload and
   delete, and unload cleanup.
4. Run focused frontend unit/type/lint checks, plugin UI/Go/race/build/package checks,
   and `git diff --check` in both repositories.

## Risks

- Push succeeds before remote creation; retry must not duplicate a pull request or hide
  the partial-success state.
- Multi-repository tasks require exact repo-scope-to-persisted-repository resolution.
- Association hydration must stay workspace-bounded and shared across many task rows;
  per-task provider polling is not acceptable.
- `host.storage` is permission-gated; upgrade failure must remain visible and must not
  corrupt in-memory saved-query state.

## Completion evidence

- Native **Create PR** pushed the verified session checkout, dispatched
  `pullrequests.create` through the registered Bitbucket provider, received HTTP 201,
  and linked the returned pull request without adding Bitbucket logic to agentctl.
- Shared desktop status hover exposed unlink; unlink removed topbar/composer status and
  sidebar indicator immediately and remained detached after a full reload.
- Workspace association hydration rendered linked pull-request indicators across the
  sidebar from one provider snapshot.
- A changed dashboard query enabled **Save current query**; the shared host dialog saved
  it, reload retained it, and selecting it restored the exact query.
- Host focused frontend tests (100), typecheck, ESLint, backend plugin/SDK tests, Go
  lint, plugin UI tests (44), plugin Go race tests, vet, package verification, and public
  docs validation all passed.
