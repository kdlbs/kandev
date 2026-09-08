---
spec: docs/specs/agents/requirements/runtime-updates.md
decision: docs/decisions/2026-07-26-user-managed-agent-runtime-updates.md
created: 2026-07-26
status: completed
---

# Implementation Plan: Managed Agent Runtime Updates

## Overview

Replace PR 1950's repository-maintained npm pins and scheduled updater with
unversioned, cache-preferring managed ACP launch commands. Then add a
host-scoped, user-triggered update pipeline that refreshes npm's execution
cache, re-probes ACP capabilities, and streams the result into the responsive
Settings > Agents UI. The implementation preserves active sessions and treats
ACP negotiation plus advertised capabilities as the compatibility boundary.

---

## Backend

### Managed npm runtime contract

- Add `ManagedNPMRuntimeSpec` and the optional `ManagedNPMRuntimeAgent`
  capability to `apps/backend/internal/agent/agents/agent.go`.
  `ManagedNPMRuntimeSpec` owns the hard-coded npm package and ACP argv suffix;
  request data never selects either value.
- Add command helpers in
  `apps/backend/internal/agent/agents/managed_npm_runtime.go`:
  `CachedACPCommand()` emits
  `npx --yes --prefer-offline <unversioned-package> <acp-args>`, and
  `CacheUpdateCommand()` emits a direct argv command equivalent to
  `npm exec --yes --prefer-online --package=<unversioned-package> -- node -e ""`.
  The update command primes the same unversioned npm execution-cache key
  without relying on each package exposing a version flag.
- Implement the capability for Claude, Codex, OpenCode, Copilot, and Gemini in
  their existing files. Use `CachedACPCommand()` from `BuildCommand`,
  `Runtime().Cmd`, and `InferenceConfig().Command` so sessions and capability
  probes resolve the same runtime. Keep passthrough/authentication helpers
  separate where they intentionally use another command.
- Remove all exact version constants/specs from the five agents. Keep remote
  install scripts unversioned. Copilot no longer opts its managed ACP launch
  into native-binary preference; its native CLI remains available for
  discovery and login.

### Runtime update and capability refresh

- Extend `apps/backend/internal/agent/hostutility/public.go` and
  `manager.go` with `RefreshWithCommand(ctx, agentType, command)`. It reuses the
  existing bounded ACP probe and cache normalization but overrides the
  inference command for that probe. The normal `Refresh` path remains
  unchanged.
- Add `apps/backend/internal/agent/settings/controller/agent_update.go` with an
  `agentRuntimeUpdater` seam for tests and a production implementation that:
  reads current version from `hostutility.Manager.Get`, resolves the target
  with direct argv `npm view <package> dist-tags.latest --json`, runs only the
  built-in `CacheUpdateCommand`, and calls `RefreshWithCommand` with the
  cache-preferring ACP command.
- Add `apps/backend/internal/agent/settings/controller/agent_update_job.go`.
  `AgentUpdateJobStore` uses the existing 64 KiB ring-buffer, five-minute
  timeout/retention, four-worker bound, batched output, and broadcaster
  patterns. It transitions through `queued`, `resolving`, `updating`,
  `refreshing`, and a terminal state while retaining current/target/refreshed
  versions and refresh errors.
- Add a shared same-agent maintenance coordinator in
  `apps/backend/internal/agent/settings/controller/maintenance_jobs.go`.
  Repeated update requests reuse the active update. An install/update conflict
  returns `409 Conflict` with the active maintenance job identity and kind;
  neither command is started.
- Classify an authentication-required post-update probe as package success with
  `refresh_error`. Registry, npm-exec, timeout, unsupported-protocol, and
  non-recoverable initialization failures are terminal failures and retain the
  prior capability cache.
- After a successful refresh, call `BroadcastAvailableAgents` so the new
  version, models, modes, commands, and configuration options reach open
  Settings pages without reload.

### HTTP, DTO, and WebSocket contracts

- Extend `apps/backend/internal/agent/settings/dto/dto.go` with
  `RuntimeUpdateDTO`, `AgentUpdateJobStatus`, `AgentUpdateJobDTO`,
  `ListAgentUpdateJobsResponse`, and the optional
  `AvailableAgentDTO.RuntimeUpdate`.
- Populate runtime metadata in
  `apps/backend/internal/agent/settings/controller/agent_discovery.go` only
  when an available built-in agent implements `ManagedNPMRuntimeAgent`.
  `current_version` comes from the latest successful host capability record.
- Register the three update endpoints in
  `apps/backend/internal/agent/settings/handlers/handlers.go`, protected by the
  existing Settings interlock for POST. Preserve `202 Accepted` for a newly
  queued or deduplicated update and return typed 404/409/503 errors.
- Add `ActionAgentUpdateStarted`, `ActionAgentUpdateOutput`, and
  `ActionAgentUpdateFinished` to
  `apps/backend/pkg/websocket/actions.go`.

## Frontend

### API, state, and WebSocket hydration

- Extend `apps/web/lib/types/http-agents.ts` with `RuntimeUpdate` metadata.
- Add `AgentUpdateJob`, `updateAgent`, `listAgentUpdateJobs`, and
  `getAgentUpdateJob` to `apps/web/lib/api/domains/settings-api.ts` and export
  them through the existing API barrels.
- Add `updateJobs` plus set/upsert/append/clear actions to
  `apps/web/lib/state/slices/settings/types.ts`,
  `settings-slice.ts`, `apps/web/lib/state/store.ts`, and their re-export
  surfaces.
- Add typed update payloads/actions to
  `apps/web/lib/types/backend.ts` and consume the three notifications in
  `apps/web/lib/ws/handlers/agents.ts`. Appended output chunks must not replace
  the retained job log.
- Add `useAgentRuntimeUpdates` beside the existing install hook in
  `apps/web/app/settings/agents/page.tsx`. It rehydrates retained jobs on
  mount, submits updates, handles maintenance conflicts, and relies on
  `agent.available.updated` for the refreshed catalogue.

### Installed-agent update control

- Add
  `apps/web/components/settings/agent-runtime-update-control.tsx` and compose it
  into `installed-agent-card.tsx` only when `runtime_update.supported` is true.
- Show current version (or `Unknown`), target when resolved, the current job
  phase, bounded streamed output, refresh-only warnings, retryable failures,
  and terminal success. Disable the action while install or update maintenance
  is active for the same agent.
- Preserve the existing single-column phone card layout. The labeled update
  button has at least a 44 px touch target, version/progress content wraps
  vertically, the document remains the primary scroller, and only the bounded
  output log gets internal vertical scrolling. No horizontal page scrolling or
  hover-only affordance is introduced.

## Documentation and PR replacement

- Delete the scheduled updater workflow, updater scripts/tests, obsolete
  pin-oriented plan, and the rejected scheduled-pin ADR added by the current
  branch.
- Restore `.github/workflows/lint-action-pinning.yml` and the unrelated GitLab
  poller test to their `origin/main` behavior using explicit patches.
- Rewrite `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`,
  `README.md`, and
  `docs/decisions/0034-agentclientprotocol-codex-acp.md` so they describe
  unversioned managed ACP resolution rather than numeric pins.
- Document the Settings update action, host-only scope, active-session
  behavior, model refresh, and best-effort npm cache in
  `docs/public/agents-and-profiles.md`.

## Tests

- **What:** all five managed agents use the unversioned cache-preferring command
  on ACP build/runtime/inference surfaces and expose the correct hard-coded
  package metadata.
  **File:** tests beside
  `apps/backend/internal/agent/agents/{claude_acp,codex_acp,opencode_acp,copilot_acp,gemini}.go`
  and `apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go`.
  **How:** table-driven argv assertions; reject `@<version>` and explicit
  `latest`.
- **What:** cache update commands use direct argv, online freshness, a
  hard-coded package, and no user-controlled shell.
  **File:**
  `apps/backend/internal/agent/agents/managed_npm_runtime_test.go`.
  **How:** unit tests over every managed runtime spec.
- **What:** force refresh probes with the override command and replaces the
  host capability cache.
  **File:** `apps/backend/internal/agent/hostutility/manager_test.go`.
  **How:** fake agentctl probe request/response assertions.
- **What:** target resolution, job states, output bounds, deduplication,
  install/update mutual exclusion, refresh success, auth-required refresh, and
  hard failures follow the spec.
  **File:**
  `apps/backend/internal/agent/settings/controller/agent_update_test.go` and
  `agent_install_test.go`.
  **How:** fake updater/host utility plus real in-memory job scheduling.
- **What:** handler → controller → job-store endpoints return accepted,
  retained, not-found, conflict, and interlock responses and broadcast update
  events.
  **File:**
  `apps/backend/internal/agent/settings/handlers/agent_update_handlers_test.go`.
  **How:** Gin HTTP integration tests with injected controller seams.
- **What:** the catalogue exposes managed runtime metadata and omits it for
  unmanaged/native-only agents.
  **File:**
  `apps/backend/internal/agent/settings/controller/agent_discovery_test.go`.
  **How:** registry and capability-cache fixtures.
- **What:** frontend state merges snapshots and output chunks without
  duplication, and the installed card renders idle/running/success/failure
  states accessibly.
  **File:**
  `apps/web/lib/state/slices/settings/settings-slice.test.ts`,
  `apps/web/lib/ws/handlers/agents.test.ts`, and
  `apps/web/components/settings/agent-runtime-update-control.test.tsx`.
  **How:** Vitest store/handler tests and Testing Library component tests.

## E2E Tests

- **Scenario:** given an installed managed agent, starting Update shows
  current-to-target versions, reachable progress/output, and terminal success.
  **File:**
  `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`.
  **What to verify:** stable test IDs, POST submission, retained-job
  rehydration, progress text, terminal state, and retry after failure using
  deterministic network fixtures while backend integration is covered in Go.
- **Scenario:** given a successful update whose refreshed catalogue adds a
  model, the new model appears without a document reload.
  **File:** same spec.
  **What to verify:** an `agent.available.updated` catalogue replacement is
  reflected in the installed agent/profile model surface.
- **Scenario:** given a phone viewport and a running update, all version,
  progress, output, and retry controls are touch-reachable without horizontal
  overflow.
  **File:**
  `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`.
  **What to verify:** labeled 44 px action, wrapped content, bounded log scroll,
  page scroll, and `document.scrollWidth <= document.clientWidth`.

## Implementation Waves

Wave 1 (parallel):

- [x] [Task 01: Define unpinned managed runtimes](task-01-unpinned-managed-runtimes.md)
- [x] [Task 02: Remove pin automation and update documentation](task-02-remove-pin-automation.md)

Wave 2:

- [x] [Task 03: Build the backend update pipeline](task-03-backend-update-pipeline.md)

Wave 3:

- [x] [Task 04: Add the Settings update UI](task-04-settings-update-ui.md)

Wave 4:

- [x] [Task 05: Verify the runtime update user flows](task-05-runtime-update-e2e.md)

## Verification

- `cd apps/backend && go test ./internal/agent/agents ./internal/agent/runtime/lifecycle`
- `cd apps/backend && go test ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/handlers`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/settings/settings-slice.test.ts lib/ws/handlers/agents.test.ts components/settings/agent-runtime-update-control.test.tsx`
- `cd apps/web && pnpm e2e:run tests/settings/agent-runtime-update.spec.ts`
- `cd apps/web && pnpm e2e:run tests/settings/mobile-agent-runtime-update.spec.ts -- --project=mobile-chrome`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `git diff --check`
- Commit through active hooks, then delegate `/verify` in full mode before
  pushing the replacement PR branch.

## Risks

- npm's execution cache is explicitly best-effort. Tests assert command intent,
  not durable npm cache internals.
- A same-protocol upstream runtime can still regress. The UI must preserve
  prior capability data and expose the failure; rollback remains out of scope.
- Capability probes can require authentication after package installation.
  Package success and capability refresh status must remain distinct.
- Existing sessions retain their already-started process. No lifecycle restart
  or process replacement belongs in these tasks.
