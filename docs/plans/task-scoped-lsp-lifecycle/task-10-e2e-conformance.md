---
id: "10-e2e-conformance"
title: "LSP E2E conformance"
status: completed
wave: 5
depends_on: ["09-responsive-control-surface"]
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 10: LSP E2E Conformance

## Acceptance

- The Task 01 production-build contract is green with exact PID/initialize/import/generation counts
  for pre-file Start, panel/idle independence, session/browser sharing, reload, Restart, Stop, task
  stop, progress visibility, and fallback discovery.
- Existing desktop, phone/tablet, Local Docker, and unsupported-executor LSP suites use the new task
  controller without weakening diagnostics, provider, configuration, URI, save, installer, progress,
  containment, cleanup, capacity, or binary-security assertions.
- Focused backend race/leak checks and frontend unit/type/i18n checks remain green after E2E-driven
  hardening; all managed runners tear down backend, agentctl, browser, and child processes.

## TDD sequence

1. Run the fixed Task 01 suite after Tasks 02–09 and classify every failure using fresh
   `error-context.md`, screenshot, logs, and fake-server event evidence; do not increase timeouts.
2. Update obsolete existing test setup/selectors to use task policy/control. Preserve each original
   behavioral assertion and add explicit process/generation evidence where browser streams used to
   stand in for lifecycle truth.
3. Make desktop/local-host suites green, then mobile, then container/SSH projects using the managed
   production runner and correct project selection. Verify intended test counts before accepting a
   run.
4. Run mutating specs with affected neighbors under one worker/retries zero where they share global
   settings or capacity. Restore every saved setting/policy and delete test-owned sentinel files.
5. Re-run focused race/leak/unit/type/i18n checks after final E2E fixes and record process teardown.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/lsp/task-lsp-lifecycle.spec.ts
cd apps/web && pnpm e2e:run --no-build tests/lsp/lsp-file-intelligence.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/lsp/mobile-lsp-file-intelligence.spec.ts
cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/docker/lsp-file-intelligence.spec.ts tests/ssh/lsp-unsupported-executor.spec.ts
cd apps/backend && go test -race ./internal/lsp ./internal/agentctl/server/lsp ./internal/gateway/websocket ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl
cd apps && pnpm --filter @kandev/web test -- --run lib/lsp lib/state/slices/lsp components/lsp components/app-status-bar
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
```

Use `--no-build` only after the first managed run built the final unchanged production artifacts and
the packaged E2E plugin exists. Record exact discovered/passed counts for every project.

## Files likely touched

- `apps/web/e2e/fixtures/fake-lsp-server.mjs`
- `apps/web/e2e/tests/lsp/lsp-e2e-helpers.ts`
- `apps/web/e2e/tests/lsp/task-lsp-lifecycle.spec.ts`
- `apps/web/e2e/tests/lsp/lsp-file-intelligence.spec.ts`
- `apps/web/e2e/tests/lsp/mobile-lsp-file-intelligence.spec.ts`
- `apps/web/e2e/tests/docker/lsp-file-intelligence.spec.ts`
- `apps/web/e2e/tests/ssh/lsp-unsupported-executor.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`
- production/test files from Tasks 02–09 only when a concrete failing conformance case proves a bug

## Dependencies

Tasks 01–09: fixed acceptance harness plus complete backend/frontend behavior.

## Parallelism

Sequential. Suites share production artifacts, fake-server helpers, global settings, and capacity.

## Inputs

- Every spec scenario and Required tests item from the user request.
- E2E managed-runner/project/fixture rules and existing LSP tests.
- Task-level process/generation evidence from agentctl and fake server, never browser-socket counts
  alone.

## Output contract

Report each command/project/test count, any failures and root-cause fixes, fake PID/generation/import
evidence, artifacts inspected/captured, settings/sentinel cleanup, and race/leak/process teardown.
Update task/plan status and actual files.

## Results

Completed 2026-08-05.

- Production-build task contract: `pnpm e2e:run tests/lsp/task-lsp-lifecycle.spec.ts` — 9/9
  passed in 1.4 minutes. Fake-server evidence held one PID/initialize/import across file, panel,
  session, browser, reload, and former-idle-boundary changes; explicit Restart produced exactly one
  replacement generation, Stop prevented reacquisition, and archive reaped the process.
- Existing desktop coverage: `pnpm e2e:run --no-build tests/lsp/lsp-file-intelligence.spec.ts` —
  13/13 passed in 2.1 minutes after the final rebase, including missing-binary guidance, honest
  progress, shared configuration, multi-root URIs, crash recovery, archive cleanup, and real-server
  capacity.
- Responsive coverage: `pnpm e2e:run --no-build --project mobile-chrome
  tests/lsp/mobile-lsp-file-intelligence.spec.ts` — 3/3 passed in 17.5 seconds across phone and
  tablet composition.
- Runtime coverage: `KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers
  tests/docker/lsp-file-intelligence.spec.ts tests/ssh/lsp-unsupported-executor.spec.ts` — 4/4
  passed in 1.2 minutes. Local Docker shared one task host; SSH failed closed without LSP launch.
- After rebasing onto `origin/main` at `828213a6a`, focused frontend suites passed 22 files / 183
  tests after upstream test consolidation. All-app typecheck, full frontend lint, i18n check and
  ratchet, and production Vite builds passed.
- A non-required broad web run completed all 1,130 files and all 8,703 assertions, then exited on
  one unhandled localhost/Monaco error while a concurrent managed E2E backend was being torn down.
  The named automation test passed 2/2 in isolation after E2E ended; this run is recorded as an
  interference artifact, not claimed as a clean full-suite pass.
- `go test -race ./internal/lsp ./internal/agentctl/server/lsp
  ./internal/gateway/websocket ./internal/agent/runtime/lifecycle
  ./internal/agent/runtime/agentctl` passed. Both long-running LSP packages use `goleak` TestMain;
  focused backend packages and backend lint passed with zero findings.
- Final managed runners left no E2E containers, backend, agentctl, fake-server, or Kotlin child
  processes. One exact temp-home tree left by an earlier interrupted runner was identified from its
  `KANDEV_E2E_MOCK` environment and reaped before the clean final matrix.
- Post-rebase `make test` reached only unchanged `origin/main` filesystem-fixture failures in task
  handlers/service (`parent directory cannot be accessed` / invalid folder location) before Make
  stopped; all changed backend packages and isolated task-LSP cleanup tests passed. Separate CLI and
  script targets exposed unchanged base-branch release/review workflow assertion mismatches. Root
  lint, architecture lint, formatting, docs validation, and all-app typecheck passed.
- PR CI on 2026-08-06 exposed one stale incremental-change assertion after attachment replay moved
  the upstream document contract to canonical full text. The corrected production-build scenario
  passed 1/1 with retries disabled. The same shard's status-placement retry passed once in isolation
  and 3/3 repeated runs with one worker and no retries, so no product change or timeout increase was
  made for that non-reproducing timing artifact.
- After the 2026-08-12 Docker credential/environment fixes, the exact production container command
  selected first launch, multi-session environment reuse, and the complete Docker task-host LSP
  spec. All 6/6 passed in 1.7 minutes: Kotlin discovery/start before opening an editor, in-container
  server execution, two-session Monaco isolation, and edit persistence across LSP Stop/reload.
- After the follow-up independent review and rebase onto `origin/main` at `723c14001`, `make test`
  and `make lint` pass. `go test -race ./internal/agentctl/server/lsp ./internal/lsp
  ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor ./internal/task/service
  -count=1` passes. The rebased settings/status surface passes 7 focused frontend files / 80 tests,
  typecheck, lint, `i18n:check`, `i18n:ratchet`, and the production Vite build. Public docs pass
  58/58 validator tests and 41-page validation.
