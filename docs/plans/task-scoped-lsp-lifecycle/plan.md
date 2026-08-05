---
spec: docs/specs/lsp-file-intelligence/spec.md
created: 2026-08-05
status: draft
---

# Implementation Plan: Task-Scoped LSP Lifecycle

## Overview

Replace the browser/session-owned LSP stream with the ownership boundary accepted in
[ADR-2026-08-05-task-scoped-lsp-ownership](../../decisions/2026-08-05-task-scoped-lsp-ownership.md).
Build the durable task/language contract first, move process and JSON-RPC ownership into the task
host, add the authorized backend control plane and recovery hooks, then migrate Monaco and compose
one responsive task-level control surface. Acceptance E2E is written red before production code;
the final waves update every existing LSP scenario, public docs, and scoped architecture guidance.

The implementation remains sequential in the primary conversation. Waves document dependency
order only and do not authorize subagents.

---

## Backend

### Durable task/language contract

- Add task-scoped models, enums, snapshot DTOs, `ControlAction`, and trusted `Origin` metadata under
  `apps/backend/internal/lsp/`. Validate registered language IDs, policies, phases, initiators, and
  stable reason/error codes at this boundary.
- Add `task_lsp_languages` to both fresh schema and replayable migrations in
  `apps/backend/internal/task/repository/sqlite/base_schema.go` and `base_migrations.go`. Its
  composite `(task_id, language)` primary key, task cascade, policy/detection fields, phase,
  generation, revision, timestamps, reasons, initiator, restart-required overlay, and error fields
  match the spec exactly.
- Implement the `internal/lsp.Store` interface in
  `apps/backend/internal/task/repository/sqlite/task_lsp.go`. Upserts and transitions increment the
  row revision atomically; generation allocation is monotonic; delete/archive recovery queries do
  not infer ownership from sessions.
- Cover fresh SQLite, legacy SQLite replay, env-gated Postgres replay, composite uniqueness,
  cascade deletion, policy round-trip, revision ordering, and generation allocation.

### Task-host runtime supervisor

- Extract the connection-owned logic in `apps/backend/internal/agentctl/server/api/lsp.go` into an
  instance-owned manager under `apps/backend/internal/agentctl/server/lsp/`. `api.Server` creates
  one manager and closes it during instance teardown.
- Serialize `Start(language, generation, config)`, `Stop`, and `Restart` per language. A duplicate
  generation is idempotent; Restart reaps the old process before admitting the supplied next
  generation. Continue to use `process.Manager.StartPipedProcess` and owned installer operations,
  including the existing command registry, path/cache rules, frame limits, write deadlines, and
  full process-tree cleanup.
- Move `initialize`/`initialized`, client capabilities, `workspace/configuration`, configuration
  changes, progress-token handling, server capabilities, diagnostics, and graceful
  `shutdown`/`exit` into the task-host manager. `ready` is emitted only after initialize succeeds;
  `process_started` is separate evidence.
- Expose authenticated internal task-host control, snapshot, and watch routes. A disconnected
  control/watch request never owns the process. Teach `instance.Instance.IsIdle` (through a narrow
  background-work capability) that a live language server prevents idle reaping.

### Task-host attachment hub

- Add a downstream attachment hub beside the task-host manager. Namespace browser request IDs by
  attachment and generation; route responses only to the owner; handle cancellation and detach;
  fan out safe notifications and cached diagnostics; centrally satisfy server requests that the
  task host advertised.
- Intercept lifecycle methods so an attachment cannot send `initialize`, `shutdown`, or `exit`.
  Maintain one upstream open document per canonical URI: duplicate opens add references, changes
  get monotonic versions in arrival order, stale duplicate opens do not rewind text, and only the
  final close sends upstream `didClose`.
- Send an `attached` envelope with generation, workspace metadata, and capabilities before raw
  feature traffic. Detaching the final browser releases documents and outstanding requests but
  leaves the task-host process, initialization, progress, and caches alive.

### Bounded language discovery

- Add a read-only scanner to `apps/backend/internal/agentctl/server/lsp/discovery.go` using the task
  root and `process.Manager.RepoSubpaths()`. Enforce the spec's two-second, 10,000-entry, depth-six,
  ignored-directory, and no-directory-symlink budgets. Inspect names/extensions only.
- Expose authenticated discovery through agentctl and its backend client. Return deterministic
  sorted language IDs plus complete/partial metadata. Discovery must not call the installer,
  process runner, `GetOrEnsureExecution`, or a language-server endpoint.
- Trigger discovery only from an already-ready Local PC/Worktree or Local Docker task environment
  and after task workspace-source changes. Persist its task/language projection; synthesize all
  supported languages in read responses when no row exists.

### Authorized task controller

- Implement `apps/backend/internal/lsp/controller.go` as the one service seam for snapshots,
  policy changes, Start/Stop/Restart, attachment resolution, task teardown, and reconciliation.
  Every exported user operation calls `task.Service.AuthorizeTaskAccess` before any LSP row,
  environment, execution, capacity, installer, or agentctl access.
- Resolve effective policy in the backend: Keep warm always desires a server while the task is
  active; Disabled never does; Inherit requires both bounded detection and the singleton global
  auto-start default. Settings changes reconcile affected task/language rows and push live server
  configuration through agentctl.
- Add a backend executor-capability resolver keyed by the canonical task environment. Local
  PC/Worktree and Local Docker are true; Remote Docker, SSH, Sprites, missing, and unknown are
  false. Apply it before capacity and `GetOrEnsureExecutionForEnvironment`.
- Replace the browser-stream limiter with a task/language server limiter. Parse
  `KANDEV_LSP_MAX_SERVERS`, fall back to deprecated `KANDEV_LSP_MAX_CONNECTIONS` only when unset,
  hold one slot from process admission through complete reaping, and keep desired overflow in a
  deterministic queue without starting/resuming task resources.
- Add the task-scoped REST routes and `/lsp/tasks/:taskId/:language/attach` proxy. Remove
  `/lsp/:sessionId` and all public session language ownership. Handlers stamp human origin; request
  bodies cannot supply task/session/runtime IDs, generation, initiator, or reason.
- Serialize each task/language command. Persist policy/action before detached bounded convergence,
  coalesce duplicate in-flight actions, preserve acceptance ordering for unlike actions, and use
  agentctl generation idempotency to close retry/time-out races.

### Reconciliation and task cleanup

- Start an explicitly owned controller worker with `Start`, `Close`, context cancellation, and a
  joined `WaitGroup`. On backend start, inspect durable desired rows and live task-environment
  executions, adopt task-host generations before launching, stop orphan/disabled runtimes, clear
  stale progress, and rebuild capacity/queue state deterministically.
- Maintain non-owning task-host watch streams. Reconnect and adopt snapshots after backend/agentctl
  transport loss. On a proven crash while desired, schedule 1s/5s/30s recovery; reset the budget
  only after five ready minutes. Never time out a live initialize request.
- Publish semantic `task.lsp_state_changed` events and browser `task.lsp.changed` snapshots with
  per-language revisions. Snapshot and live-event ordering must reject stale state on the frontend.
- Wire task/environment lifecycle ownership before runtime cleanup: task stop, archive, delete,
  environment replacement, and teardown cancel recovery, stop all task languages, clear runtime
  evidence, then rely on existing executor cleanup as the kill backstop. Do not wire session stop,
  browser close, or editor lifecycle as task LSP cleanup.
- Preserve policy/evidence across temporary task stop and archive; cascade on delete. On resume or
  replacement environment readiness, discover/reconcile once. Refuse a new environment launch
  until the old task-host generation is confirmed dead.
- For multi-repository source changes, push `workspace/didChangeWorkspaceFolders` when advertised;
  otherwise persist and publish `restart_required` without automatic reimport.

---

## Frontend

### Task LSP domain state

- Add task-scoped HTTP types and calls in `apps/web/lib/types/http-lsp.ts` and
  `apps/web/lib/api/domains/lsp-api.ts`.
- Add an `lsp` Zustand slice keyed by `taskId` then language. Hydrate through
  `useTaskLsp(taskId)`, merge full snapshots, apply `task.lsp.changed`, and reject lower revisions.
  Keep only command-pending presentation locally; backend/task-host state remains authoritative.
- Expose shared Start/Stop/Restart/SetPolicy actions and derived task aggregate through domain
  hooks. Remove browser-local manual enablement and ignore/delete legacy
  `kandev-lsp:<session>:<language>` keys.

### Monaco task attachment

- Re-key `LSPClientManager` connections from session/language to task/language and open only the
  task attachment route. The manager consumes task-host capabilities from the attach envelope and
  no longer sends initialize, configuration, shutdown, or exit.
- Keep session identity only for browser Monaco model isolation and navigation. Route a provider
  request from its source model to the task connection, map returned task-host URIs back into the
  initiating session model, and keep TypeScript built-in suppression model/capability scoped.
- Forward document opens/changes/closes/saves through the attachment broker. Editor lease release
  may close a downstream attachment, but it has no lifecycle timer and never changes task policy or
  task-host generation.
- The phone file viewer remains attachment-free. A warm task server can exist because of task
  policy, but mounting the phone viewer cannot cause it.

### Aggregate controls and responsive composition

- Add a pure `task-lsp-view-model.ts` that orders languages, determines relevance, chooses the
  compact summary, formats honest phase/work/elapsed evidence, and exposes action availability.
- Replace the active-file-only `LspStatusItem` with the active task aggregate under stable ordering
  ID `builtin:lsp`. When the Application status bar is enabled, show it independently of active
  panel/file. When disabled or the compact aggregate is absent, show a task/workspace topbar action.
- Keep a supported Monaco editor toolbar shortcut at all times; it focuses the current language in
  the shared task controller. Remove the placement selector and its runtime placement resolver.
  Retain the backend settings field as ignored compatibility data for this release.
- Build one shared language list/disclosure with policy selection, phase, progress, metadata,
  errors, and Start/Stop/Restart. Restart uses a translated confirmation explaining process and
  analysis loss. Render all task-host strings as text and preserve long-token wrapping.
- On phone and coarse-pointer tablet, enable the existing App Status drawer on task routes even
  when the Application status-bar feature is disabled. Render LSP rows inline (no nested drawer),
  use one internal scroll owner, 44 px actions, safe-area/dvh containment, and no document-level
  horizontal overflow. Desktop uses the compact bar disclosure; presentation differs, state and
  actions do not.
- Route every new label, policy, reason, error, confirmation, empty state, and accessibility name
  through `lsp.json`/`settings.json` in English, pseudo, and Simplified Chinese.

---

## Tests

- **Persistent ownership:** `apps/backend/internal/task/repository/sqlite/task_lsp_test.go` and
  `task_lsp_postgres_test.go` cover `(task_id, language)` uniqueness, policy/evidence round-trip,
  monotonic revision/generation, migration replay, archive retention, and task-delete cascade.
- **Authorization:** `apps/backend/internal/lsp/controller_test.go` and handler tests prove task
  authorization happens before row/environment/execution/capacity access and an unauthorized
  caller receives no runtime evidence.
- **Control concurrency/capacity:** controller and task-host manager tests use synchronized fakes
  for duplicate Start, overlapping Start/Stop/Restart, idempotent generation retries, one-process
  maximum, queue ordering, slot release after reaping, and unsupported-before-ensure behavior.
- **Task-host lifecycle:** `apps/backend/internal/agentctl/server/lsp/manager_test.go` covers install,
  process start, initialize, early progress, ready/idle, configuration updates, graceful/forced
  stop, crash recovery signals, and instance teardown. `idle_reaper_test.go` proves a live LSP is
  not considered idle.
- **Attachment hub:** task-host tests cover request-ID remapping, stale generations, two attachments,
  disconnect cancellation, capability/diagnostic replay, duplicate document opens, ordered changes,
  final close, and no process stop on final detach.
- **Discovery:** table-driven temp-tree tests cover every supported language signal, multi-root
  order, ignored directories, no symlink traversal, each bound, partial results, cancellation, and
  zero installer/process calls.
- **Recovery/cleanup:** backend integration tests with the real repository and fake agentctl cover
  adopting a live generation after backend restart, one replacement after task-host loss, stale
  progress clearing, bounded crash retries, task resume, environment replacement, multi-session
  access, source-root handling, and stop/archive/delete cleanup.
- **Race/leak:** run focused `-race` suites for task-host manager/hub and backend controller/recovery;
  every long-running package gets a `goleak` TestMain or explicit goroutine-count assertion and
  verifies workers, watchers, timers, pipes, and child processes are joined.
- **Frontend domain:** `lsp-slice.test.ts`, `lsp-api.test.ts`, and hook tests cover task/language keys,
  policy/effective policy, stale-revision rejection, action payloads without session/origin,
  progress/restart evidence, and reload hydration.
- **Monaco bridge:** manager/provider/document tests cover task-keyed attachment reuse across
  sessions, no browser initialize/shutdown, attachment reconnect without policy change, source
  session URI mapping, diagnostic replay, model-scoped built-in suppression, save races, and final
  attachment release without an idle lifecycle stop.
- **View model/components:** pure and component tests cover aggregate priority/counts, active-file
  independence, policy controls, action availability, restart confirmation, metadata, actionable
  errors, status-bar and topbar fallback placement, drawer inline composition, 44 px affordances,
  and translated copy.

---

## E2E Tests

All browser scenarios use the production Vite build served by the Go backend. The fake Kotlin
server records PID, initialize/import count, task generation, progress, shutdown, exit, and signal
evidence so tests assert process truth rather than only UI labels.

- **Start without an editor:** `e2e/tests/lsp/task-lsp-lifecycle.spec.ts` starts Kotlin from the task
  fallback before opening a Kotlin file, sees initialize/import progress, then opens a file and
  receives diagnostics from generation one.
- **Active-file and idle independence:** switch to unsupported files and chat/review/terminal panels,
  fast-forward beyond the former two-minute browser timer, and assert the status remains actionable
  with one PID/initialize/import.
- **Multi-session/browser dedup:** switch between two sessions of the same task and attach a second
  browser context; both get intelligence while process/import generation stays one.
- **Reload/reconnect:** reload and reconnect the browser; assert the task snapshot and provider
  recover with no new process or initialize/import.
- **Explicit lifecycle:** one confirmed Restart produces exactly generation two after generation
  one exits; Stop disables policy and later file/session/browser mounts do not reacquire. Task stop
  reaps the server.
- **Progress away from file:** hold Kotlin initialize with reported Gradle work, activate another
  panel, and assert the aggregate remains visible, honest, and stoppable.
- **Placement fallback:** disable the Application status bar and prove the task/workspace entry is
  discoverable with no supported active file.
- **Phone/tablet parity:** `mobile-lsp-file-intelligence.spec.ts` proves phone Status can control a
  task server without the file viewer auto-starting it; tablet exposes the same policy/generation,
  one non-nested contained drawer, 44 px actions, safe-area containment, and no horizontal overflow.
- **Executor safety:** the Local Docker spec starts once inside the container and shares that
  process; SSH/unsupported specs operate the task surface and prove no execution/server resource is
  launched. Missing binary, initialize rejection, crash, capacity, and restart-required states
  remain actionable.
- **Existing coverage:** update every assertion in `lsp-file-intelligence.spec.ts`,
  `mobile-lsp-file-intelligence.spec.ts`, Docker LSP, and SSH LSP specs to the new contract. Remove
  obsolete localStorage/session-connection assertions, but retain or strengthen all intelligence,
  installer, URI, configuration, save, progress, cleanup, containment, and security assertions.

---

## Public documentation

- Update the Language servers section in `docs/public/developer-tools.md` (how-to) with task policy,
  aggregate/fallback/mobile controls, Start/Stop/Restart effects, discovery, persistence/recovery,
  supported executors, and honest progress.
- Update the Language servers row in `docs/public/feature-status.md` (reference) to remove the
  active-file/session/browser ownership boundary.
- Update `docs/public/configuration.md` (reference) for `KANDEV_LSP_MAX_SERVERS` and the deprecated
  fallback variable.
- Update `apps/backend/AGENTS.md` and `apps/backend/internal/agentctl/AGENTS.md` only after the new
  architecture lands, replacing the now-inaccurate session-owned LSP guidance.

---

## Verification Results

Pending. On completion, synchronize this section with every task's `## Results`: include RED/GREEN
evidence, exact test counts and commands, race/goleak outcomes, production E2E artifacts, child
process cleanup evidence, public-doc validation, and final worktree status.

---

## Implementation Waves And Parallel Candidates

Wave 1 — executable acceptance contract:

- [ ] [Task 01: Lifecycle acceptance harness](task-01-acceptance-harness.md)

Wave 2 — persistence and task-host ownership:

- [ ] [Task 02: Task language state contract](task-02-state-contracts.md)
- [ ] [Task 03: Task-host runtime supervisor](task-03-task-host-supervisor.md)
- [ ] [Task 04: Task-host attachment hub](task-04-attachment-hub.md)
- [ ] [Task 05: Bounded language discovery](task-05-language-discovery.md)

Wave 3 — backend control and lifecycle integration:

- [ ] [Task 06: Authorized task controller](task-06-task-controller.md)
- [ ] [Task 07: Lifecycle reconciliation](task-07-lifecycle-reconciliation.md)

Wave 4 — shared frontend behavior and responsive UI:

- [ ] [Task 08: Frontend task protocol bridge](task-08-frontend-protocol-bridge.md)
- [ ] [Task 09: Responsive task control surface](task-09-responsive-control-surface.md)

Wave 5 — conformance and documentation:

- [ ] [Task 10: LSP E2E conformance](task-10-e2e-conformance.md)
- [ ] [Task 11: Public documentation](task-11-public-documentation.md)

After all task checks pass, follow the repository commit, push, PR, and PR-fixup workflows. The PR
must use the repository template, include docs impact and production E2E evidence, and remain in
the primary conversation through CI/review remediation.
