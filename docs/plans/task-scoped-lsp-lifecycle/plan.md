---
spec: docs/specs/lsp-file-intelligence/spec.md
created: 2026-08-05
status: completed
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
- Persist a global `lsp_status_hidden_languages` user preference. Default every registered language
  to visible; filter only aggregate/status rows and counts, force-show the current editor language,
  and preserve the always-discoverable task/workspace entry when all languages are hidden.
- Present each language as an independently controlled compact collapsible. Keep state and detection
  in the summary, move policy/evidence/actions into the content, and replace the native policy
  dropdown with the shared Select primitive on desktop and touch layouts.

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
  errors, status visibility filtering, editor-language force visibility, independent collapsibles,
  shared Select styling, status-bar and topbar fallback placement, drawer inline composition, 44 px
  affordances, and translated copy.
- **User settings:** backend DTO/service/store and frontend hydration/dirty-state/card tests cover a
  validated, persisted hidden-language list with visible-by-default semantics.

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
- **Visibility customization:** production E2E hides one language in editor settings, verifies it is
  absent from desktop and mobile aggregate surfaces after reload without changing lifecycle state,
  verifies its current-editor shortcut still works, and restores the preference.
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
  supported executors, collapsible language rows, status visibility preferences, and honest
  progress.
- Update the Language servers row in `docs/public/feature-status.md` (reference) to remove the
  active-file/session/browser ownership boundary.
- Update `docs/public/configuration.md` (reference) for `KANDEV_LSP_MAX_SERVERS` and the deprecated
  fallback variable.
- Update `apps/backend/AGENTS.md` and `apps/backend/internal/agentctl/AGENTS.md` only after the new
  architecture lands, replacing the now-inaccurate session-owned LSP guidance.

---

## Verification Results

Completed 2026-08-05.

- TDD began with an executable production RED: the task controller was absent before any
  production implementation. Tasks 02–09 record their focused RED/GREEN evidence and actual files.
- Production Playwright passed 29/29 task-scoped LSP scenarios: 9 task-contract, 13 existing
  desktop, 3 phone/tablet, and 4 Docker/SSH tests. The fake Kotlin server verified real PID,
  initialize, import, generation, graceful-exit, and task-cleanup evidence.
- After rebasing onto `origin/main` at `828213a6a`, focused frontend verification passed 22 files /
  183 tests (upstream consolidated four test files), all-app typecheck, full lint, i18n check and
  ratchet, and production Vite build. Public docs passed 58 validator tests and 41-page validation.
- Focused backend packages, backend lint, and the race command across controller, task-host LSP,
  gateway, lifecycle, and agentctl client passed. `goleak` guards and the real process-group test
  cover worker and child-process cleanup.
- The final E2E matrix left no test-owned backend, agentctl, Kotlin server, or container processes.
  Exact commands, counts, one isolated unrelated broad-web-run artifact, and cleanup disposition
  are recorded in Tasks 10–11 and the delivery handoff.
- The repository-wide test target was also attempted after the rebase. It stopped only on unchanged
  `origin/main` filesystem-fixture failures in task handlers/service; isolated task cleanup and every
  changed backend package passed. Unchanged CLI release-workflow and review-workflow contract tests
  also expose two base-branch assertion mismatches and are recorded without expanding this feature.
- PR feedback remediation on 2026-08-06 closed lifecycle races around agent-start ordering,
  proven-no-process capacity release, task teardown cancellation, attachment replay/document
  baselines, replacement retry, WebSocket keepalive, atomic persistence returns, typed errors, and
  frontend capacity ordering. Capacity snapshots now carry a process epoch plus monotonic sequence,
  so delayed REST responses cannot overwrite newer WebSocket evidence across backend restarts.
  Focused changed-package tests, frontend typecheck/tests, backend lint, and the LSP/controller/
  gateway race suite passed. The broad backend target reproduced only the already-recorded unchanged
  local filesystem-fixture failures in task handlers/service; every changed package passed.
- A follow-up Codex review on 2026-08-06 found two controller races. Generation-scoped
  `process_exited` evidence now releases capacity before recovery and promotes the oldest queued
  server. Concurrent policy commands coalesce only when both action and policy value match, so a
  later distinct task policy still executes in FIFO order. Both regressions failed before the fix;
  their focused tests, the full controller package under `-race`, and backend lint pass. The
  production E2E assertion now matches the canonical full-text upstream `didChange` contract; its
  focused scenario passed, and the retry-only status-placement case passed four retry-free runs.
- The next Codex review found a task-host hard deadline that contradicted the accepted no-timeout
  initialization contract. The `initialize` request now uses the owned runtime lifetime context,
  so only explicit Stop, task-host shutdown, process exit, or environment teardown cancels it; the
  one-minute UI warning remains informational. Stop during an unanswered `initialize` bypasses the
  unavailable graceful-shutdown exchange, closes the protocol streams, and reaps the process tree.
- The following Codex review closed three remaining lifecycle gaps. Setting an inherited policy
  while the global default is disabled now stops the runtime without rewriting the requested
  policy. A successful user task Stop invokes the same task-LSP cleanup seam before moving the task
  to review, releasing process capacity deterministically. Task-host protocol writes now have a
  bounded deadline and close a wedged server pipe on timeout, so Stop and teardown cannot wait
  forever on JSON-RPC stdin. Focused regressions passed 20 repetitions, the changed task-host
  packages passed under `-race`, the orchestrator/backend wiring packages passed, and backend lint
  reported zero issues. CodeQL then identified a theoretical frame-allocation overflow; frame size
  arithmetic is now checked before allocation, with focused overflow and wire-format coverage.
- A later Codex review found two teardown races. Each synchronous task-host Start now registers a
  generation-aware cancelable operation before taking the language slot, allowing Stop/task cleanup
  to cancel and drain an in-flight installer instead of waiting for it to finish. Controller cleanup
  now retains capacity and publishes an actionable error when both per-language Stop and task-host
  cleanup fail; successful task-host fallback is treated as proof that the process is gone. Both
  regressions passed 20 repetitions under `-race`, as did the full supervisor/controller race suite.
- The terminal audit caught one late Codex finding after the preceding poll: coordinator/MCP task
  stops also transition a fully stopped child to review and therefore must invoke the same task LSP
  cleanup hook. The coordinator now checks that no working session remains, cleans task-owned
  runtimes before REVIEW, and reports cleanup failure. The focused ordering/idempotency/partial-stop
  suite passed 20 repetitions.
- The next exact-head review found a queued replacement Start could hold the cancellation mutex
  while waiting for the active install slot, and that an unhydrated session could temporarily attach
  to the previous active task in the frontend. Task-host starts now register independently in a
  cancellation barrier, so Stop cancels both the active install and already-waiting replacements
  without yielding the slot to them. A provided session now resolves only through its authoritative
  session/task mapping; the active task fallback remains limited to surfaces with no session. Both
  regressions failed first, then passed focused tests; frontend lint/typecheck also pass.
- The following exact-head review found that a queued reconcile could act on a keep-warm candidate
  after its effective task policy had changed to Disabled. Reconcile starts now re-read and validate
  the effective policy at the generation-allocation seam, so stale task/recovery candidates cannot
  launch or reserve capacity. The regression failed before the guard, then passed 20 repetitions
  under `-race`; the full controller race suite and backend lint pass.
- The next review found task cleanup could snapshot generation N, race with a successor allocation,
  then mark generation N+1 off without releasing its capacity slot. Cleanup now enters an exclusive
  task/language command batch, reloads the current generation and task host after earlier commands,
  holds every affected lane through the task-host teardown backstop, and only then releases the
  proven-dead current generation. The synchronized regression failed first, then passed 20 race
  repetitions with the focused cleanup failure/fallback suite.
- Before the next review request, the branch merged `origin/main` at `9a4c65f75`; the sole conflict
  retained both independently added accepted ADR index rows. On the merged tree, the controller,
  task-host, and gateway race suites pass, the four focused frontend LSP files pass 25 tests, web
  typecheck passes, and changed-code backend lint reports zero issues. The unfiltered backend lint
  target reports eight `goconst` findings introduced on `main`; they remain outside this PR diff.
- The 2026-08-06 status-surface follow-up added persisted per-language visibility and independent
  collapsibles backed by the shared Select. A production RED exposed missing boot-state hydration
  and an all-hidden fallback gap before both fixes. The final production matrix passed 10/10 task
  lifecycle, 13/13 existing desktop intelligence, and 3/3 phone/tablet scenarios without another
  Kotlin generation or import. Seven focused frontend files passed 73 tests; web lint, typecheck,
  formatting, i18n, production build, backend user/backendapp tests, changed-file Go lint, and the
  58-test/41-page public-doc validators passed. The container command rebuilt successfully but this
  host lacked a reachable Docker daemon, leaving two daemon-backed cases skipped and two
  fixture-safe cases passed; Task 10 retains the prior 4/4 real-container evidence.
- The 2026-08-12 independent GPT-5.6 Sol audit found seven backend recovery gaps. Runtime
  credentials now use an internal-only store, fresh Docker launches reserve their environment row
  before lifecycle startup, rotated credentials reach every live environment client before
  fallible persistence, Docker task hosts use ordered `/workspace` roots, settings resolve through
  the task workspace owner, runtime snapshots enforce an incarnation/revision high-water mark, and
  permanent environment deletion removes its internal credentials. The branch was rebased onto
  `origin/main` at `b170d205f` (already up to date). Exact-head `make test`, changed-code lint, and
  the LSP/task-host/lifecycle/executor race suites pass. Real-container first-launch, shared-session,
  and four Kotlin task-host scenarios pass 6/6.
- A second independent GPT-5.6 Sol review found five lifecycle defects: concurrent first resume
  could launch without a durable environment identity, inherited tasks could collide inside a
  shared physical task host, legacy runtime credentials remained user-visible, failed replacement
  cleanup could report the old generation as a successful restart, and delayed stop cleanup could
  leave capacity occupied. Resume now serializes before launch; authenticated task identity and
  task-specific workspace projections isolate `(task, language)` slots; runtime secrets migrate to
  stable internal identities; restart failures retain the requested generation as actionable
  evidence; and successful delayed reaping publishes Off. Each defect has a focused regression.
- The review fixes were rebased onto `origin/main` at `723c14001`. The conflict resolution retained
  both main's todo-panel visibility option and the branch's LSP status-language visibility setting.
  Exact-head backend-wide tests and lint pass; the affected task-host/controller/lifecycle/executor/
  task-service race suite passes; and 7 focused frontend files / 80 tests, web typecheck, lint,
  i18n checks, the new-code ratchet, production Vite build, and public-doc validators pass.
- The 2026-08-12 post-rebase GPT-5.6 Sol audit is tracked in Task 15. It closed eight concrete
  lifecycle/recovery defects: shared-environment reset races, cascaded runtime-secret cleanup,
  physical Docker repository projection, stale inherited environment references, editor-toolbar
  task identity, reconnect hydration, backend-clock rollback in capacity epochs, and reset-route
  authorization mapping. The branch was rebased onto `origin/main` at `88bf44dfc`; conflict
  resolution retained the task-scoped developer-tools contract, main's newer Chinese translations,
  and main's stronger GitHub background-refresh synchronization. Focused backend
  packages and the synchronized LSP race suite pass; five focused frontend files pass 27 tests,
  followed by web typecheck, lint, i18n checks, and the new-code ratchet. The repository-wide
  backend run reached only the environment's 10-minute package ceiling under severe SQLite I/O
  latency; it reported no assertion failure, and the affected packages pass independently.
- The next independent GPT-5.6 Sol exact-head audit closed seven further recovery gaps. Fresh
  single-repository Docker launches now persist a derived physical branch identity; cached runtime
  access is authorization-gated; failed initial frontend loads retry on reconnect; capacity retires
  superseded epochs and cannot promote while recovered processes still fill the limit; direct and
  cascade terminal mutations serialize on the physical environment and preserve terminal-session
  borrowers with rollback-safe ownership transfer; and a per-language stop failure is cleared only
  by explicit process-tree cleanup proof. Affected backend packages and their race suites pass, as
  do nine focused frontend files / 55 tests, web typecheck/lint, and both i18n checks.
- A further exact-head GPT-5.6 Sol audit found four lifecycle blockers and two major contract/state
  defects. Optional environment wiring is nil-safe; borrower Stop cannot mutate another task's
  physical environment; asynchronous direct and cascade teardown holds the physical writer lock;
  partial cascades roll back only unmutated owners; authoritative HTTP responses use per-task
  settlement order; and the ADR, spec, and public guide consistently preserve a shared host for a
  live borrower. Full task-service race verification, the exact CI regression, focused frontend
  tests/typecheck/lint, changed-code Go lint, and public-doc validation pass.
- Main then advanced to `9c3e7a2d3`; the 32-commit feature branch rebased without conflicts. The
  rebased head repeated the full task-service race suite, exact MCP CI regression, changed-code Go
  lint, focused frontend tests/typecheck/lint, and public-doc validators successfully before its
  replacement review and CI run.

---

## Implementation Waves And Parallel Candidates

Wave 1 — executable acceptance contract:

- [x] [Task 01: Lifecycle acceptance harness](task-01-acceptance-harness.md)

Wave 2 — persistence and task-host ownership:

- [x] [Task 02: Task language state contract](task-02-state-contracts.md)
- [x] [Task 03: Task-host runtime supervisor](task-03-task-host-supervisor.md)
- [x] [Task 04: Task-host attachment hub](task-04-attachment-hub.md)
- [x] [Task 05: Bounded language discovery](task-05-language-discovery.md)

Wave 3 — backend control and lifecycle integration:

- [x] [Task 06: Authorized task controller](task-06-task-controller.md)
- [x] [Task 07: Lifecycle reconciliation](task-07-lifecycle-reconciliation.md)

Wave 4 — shared frontend behavior and responsive UI:

- [x] [Task 08: Frontend task protocol bridge](task-08-frontend-protocol-bridge.md)
- [x] [Task 09: Responsive task control surface](task-09-responsive-control-surface.md)

Wave 5 — conformance and documentation:

- [x] [Task 10: LSP E2E conformance](task-10-e2e-conformance.md)
- [x] [Task 11: Public documentation](task-11-public-documentation.md)

Wave 6 — status-surface usability follow-up:

- [x] [Task 12: Persist LSP status visibility](task-12-persist-status-visibility.md)
- [x] [Task 13: Refine language disclosure](task-13-refine-language-disclosure.md)
- [x] [Task 14: Verify status customization](task-14-verify-status-customization.md)

Wave 7 — independent review and CI remediation:

- [x] [Task 15: Harden shared-environment recovery](task-15-harden-shared-environment-recovery.md)

After all task checks pass, follow the repository commit, push, PR, and PR-fixup workflows. The PR
must use the repository template, include docs impact and production E2E evidence, and remain in
the primary conversation through CI/review remediation.
