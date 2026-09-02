---
id: "17-rebase-current-main"
title: "Rebase and reconcile current-main lifecycle contracts"
status: completed
wave: 9
depends_on: ["16-harden-reattachment-and-preemption"]
plan: "plan.md"
spec: "../../specs/platform/requirements/lsp-file-intelligence.md"
---

# Task 17: Rebase and Reconcile Current-Main Lifecycle Contracts

## Acceptance

- The complete feature history rebases onto the current `origin/main`; conflict resolution keeps
  main's task-environment, startup, cleanup, UI composition, and authorization invariants while
  retaining task-scoped LSP ownership.
- The real task/language server cap comes from main's typed startup configuration through
  `limits.lspMaxServers`. `KANDEV_LSP_MAX_SERVERS` is the preferred environment alias and the old
  YAML/environment connection names remain fallback-only compatibility inputs.
- Task-environment cleanup deletes both task-host and container-level encrypted runtime secret
  handles. Durable asynchronous cleanup snapshots preserve all four references after task or
  environment rows cascade.
- Docker container-control credentials use the internal runtime-secret store and the same stable
  task-environment owner as the rotated agentctl auth token. Legacy control handles are removed
  during deterministic migration and teardown.
- Task-LSP startup recovery begins only after main's bootstrap listener is bound, preserving the
  launcher liveness contract while capacity inventory and task-host adoption run.
- The canonical requirements and system design no longer expose session/editor ownership or the
  former active-file-only status behavior. Every plan task points at the migrated requirements
  document.
- Focused backend lifecycle/configuration tests, frontend task-LSP tests and typecheck, public-doc
  validation, changed-code lint, and conflict checks pass on the rebased tree.

## TDD Evidence

- A canonical startup-config regression failed because `LimitsConfig` had only the obsolete
  browser-connection field and the task-LSP controller parsed its environment independently.
- A reset regression failed because cleanup issued only the task-host credential pair after main
  added separate container control/bootstrap handles.
- A durable cleanup regression failed because the JSON cleanup snapshot discarded main's
  `json:"-"` container credential references before the worker ran.
- Existing Docker secret tests failed because the deterministic owner switch did not recognize
  main's container-control metadata and its reveal path still used the user-visible secret store.
- The startup-order test failed until task-LSP recovery was inserted after listener binding and
  before orchestrator/automation consumers.
- Main's Git comparison API no longer exposed the old LSP-owned comparison constant. A focused
  process/API regression drove the shared `ComparisonResolution.Ready()` contract and removed the
  cross-package coupling.
- The existing topbar component regression failed for coarse pointers until the task route kept a
  48 px bar and its LSP shortcut retained a 44 px touch target.
- A reconnect regression observed three task-subscription lease acquisitions instead of one. A
  second regression observed zero acquisitions when the shared WebSocket client became available
  after the first render. The hook now keeps one lease per task/client and separately waits for the
  current connection's subscription acknowledgement before authoritative hydration.
- Main's concurrent WebSocket dispatch made subscribe/unsubscribe acknowledgement insufficient to
  prove final task membership. The gateway now applies those two mutations in wire order while
  leaving ordinary request handling concurrent.
- Main's stricter Go complexity and locale-completeness gates initially failed. The environment
  persistence path was decomposed without changing behavior, obsolete Docker helpers were removed,
  and the complete Portuguese and Traditional Chinese LSP/settings catalogs were reconciled.
- Main's specification hook rejected the migrated system design's retired `shipped` status and its
  54 KiB consolidated first part. The current design is now split at the state-machine boundary
  into two authoritative `current` files, each below the 32 KiB limit, without dropping content.

## Verification

- Rebased all 76 feature commits onto `origin/main` at
  `c51ec0a21b1997fb267289cecf493d941112d7a7`; the merge base is exact and range-diff retained the
  complete feature series.
- `go test ./... -run '^$'` passed for every backend package.
- `go test -race ./internal/lsp ./internal/agent/runtime/lifecycle` passed, and
  `go test -race ./internal/gateway/websocket` passed after the ordered task-subscription change.
- `golangci-lint run ./... --new-from-rev=c51ec0a21b1997fb267289cecf493d941112d7a7
  --timeout=5m` reported `0 issues`.
- `pnpm vitest run hooks/domains/lsp/use-task-lsp.test.tsx lib/ws/client.test.ts
  components/task/task-top-bar.test.tsx` passed 34 tests; the final complete 28-file LSP and
  WebSocket-focused suite passed 245 tests.
- `pnpm run typecheck`, changed-file ESLint, `pnpm run i18n:check`,
  `pnpm run i18n:ratchet`, and the Vite production build passed.
- Production-build Playwright passed all 10 desktop task-LSP lifecycle scenarios and all three
  phone/tablet LSP scenarios. This includes inherited-default startup, reconnect hydration,
  status-bar fallback, tablet touch geometry, and phone Status-drawer controls.
- `node --test scripts/validate-public-docs.test.mjs` passed 61 tests and
  `node scripts/validate-public-docs.mjs` validated all 41 published pages.
- `pre-commit run specification-lint --files` passed for both LSP system-design parts.
- `git diff --check` passed on the final rebased worktree.

## Files

- `apps/backend/internal/common/config/**`
- `apps/backend/internal/backendapp/{main.go,task_lsp.go}`
- `apps/backend/internal/task/service/**`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agentctl/server/{api/git.go,process/comparison_target.go}`
- `apps/backend/internal/gateway/websocket/{client.go,client_pump_integration_test.go}`
- `apps/web/components/task/task-top-bar.tsx`
- `apps/web/hooks/domains/lsp/{use-task-lsp.ts,use-task-lsp.test.tsx}`
- `apps/web/lib/ws/{client.ts,client.test.ts}`
- `apps/web/src/locales/{pt-pt,zh-hk,zh-tw}/{lsp.json,settings.json}`
- `docs/public/configuration.md`
- `docs/specs/platform/requirements/{lsp-file-intelligence.md,startup-configuration-parity.md}`
- `docs/specs/platform/system-design/lsp-file-intelligence-*.md`
- `docs/plans/task-scoped-lsp-lifecycle/**`
