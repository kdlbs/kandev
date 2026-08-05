---
id: "10-e2e-conformance"
title: "LSP E2E conformance"
status: pending
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
cd apps/backend && go test -race ./internal/lsp/... ./internal/agentctl/server/lsp ./internal/gateway/websocket
cd apps && pnpm --filter @kandev/web test -- --run lib/lsp lib/state/slices/lsp components/lsp components/app-status-bar
cd apps/web && pnpm run typecheck
cd apps && pnpm run i18n:check && pnpm run i18n:ratchet
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

Pending.
