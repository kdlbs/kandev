---
spec: docs/specs/agents/requirements/runtime-updates.md
created: 2026-08-16
status: implemented
---

# Implementation Plan: Recover Stale npm Runtime Metadata

## Overview

Managed npm runtimes normally start with `npx --prefer-offline`. A stale npm
packument can then hide a dependency version that the configured registry
contains. The managed process exits, and ACP reports only that its peer
disconnected. This plan detects the strict npm `ETARGET` signature in captured
stderr, invalidates only the trusted runtime's deterministic `_npx` tree, and
retries the same package and version once with `--prefer-online`. If recovery
fails, Kanban and Office show an npm-specific inline card with one runtime retry
action and collapsed technical details.

The implementation does not run `npm cache clean --force`. It does not change
the registry, package, active version, model, permissions, or session identity.

## Backend

### Classification and trusted command construction

- `apps/backend/internal/agent/runtime/routingerr/runtime_rules.go`: add a
  stable code for a managed npm version-resolution failure. Require npm
  `ETARGET` and `No matching version found for package@version` in the same
  bounded stderr sample. Match the supported old and new npm error prefixes.
  Do not classify a generic ACP disconnect or unrelated npm failure.
- `apps/backend/internal/agent/agents/managed_npm_runtime.go`: add a trusted
  command option for online metadata refresh. The option changes only
  `--prefer-offline` to `--prefer-online`. The managed runtime spec still owns
  the package and ACP arguments.
- Move deterministic `_npx` cache-key discovery and safe deletion behind one
  helper in `apps/backend/internal/agent/managedruntime`. Reuse it from runtime
  update jobs and launch recovery. Reject the npm cache root, the `_npx` root,
  symlink escapes, and every path outside the exact derived key.

### Bounded launch recovery

- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`: after a
  host-local managed runtime fails ACP initialization, fetch its bounded stderr
  and classify the combined failure evidence. Retry only the new exact
  classifier.
- Stop the first child, invalidate the exact execution tree, rebuild the same
  command in online mode, reconnect streams, and initialize once more. Preserve
  the active exact version, command prefix, model, permissions, and session ID.
- Track a startup attempt generation on `AgentExecution`. Completion and stream
  events from the first child must not publish a terminal state for the retry.
  Cancellation and shutdown must win over recovery.
- Emit no user-facing failure when the online retry succeeds. If it fails,
  publish one final event with a stable failure code and bounded sanitized
  details. Never include raw host paths or use stderr values as command input.

### Failure persistence and recovery actions

- Extend the lifecycle and watcher event data with optional failure code and
  details fields. Keep existing generic error fields compatible.
- Extend `models.LastAgentError` with optional code and details fields. Store
  the fields in existing session JSON metadata, so no schema migration is
  required.
- `apps/backend/internal/orchestrator/event_handlers_agent.go`: recognize the
  structured npm failure. Persist one recovery message with
  `failure_kind = managed_runtime_npm_resolution`, sanitized `error_output`,
  and one **Retry runtime** action. Use resume mode when a resume token exists.
  Otherwise, use the existing fresh-run recovery mode behind the same label.

## Frontend

### Shared recovery presentation

- Extend `RecoveryMetadata` and `LastAgentError` parsers with the optional
  structured failure fields. Accept snake case from the API and camel case
  after store serialization.
- `apps/web/components/task/chat/messages/action-message.tsx`: render the
  managed npm failure through the existing inline special-recovery path. State
  that npm could not prepare the runtime and that Kandev refreshed package data
  and retried once. Keep technical details collapsed and show only
  **Retry runtime**.
- `apps/web/components/task/simple/chat-entries.ts` and
  `apps/web/components/task/simple/components/run-error-entry.tsx`: carry the
  same failure code and details to Office chat and render the same meaning and
  action set.
- Add all copy to the existing i18n catalog. Do not mention ACP in user-facing
  text.

### Mobile design contract

Kanban and Office keep their current single chat scroll owner. The recovery
card remains inline on desktop and phone. It does not open a dialog, sheet, or
drawer. On phone widths, actions stack when needed and each control is at least
44 px high. The nearest shipped mobile exemplar is the provider recovery card
in `action-message.tsx`. Business state, recovery requests, error parsing, and
copy keys remain shared across viewports.

## Documentation

- Update the troubleshooting and managed runtime sections in
  `docs/public/agents-and-profiles.md`. Explain the automatic same-version
  retry, the configured-registry check, and why global npm cache cleanup is not
  the normal recovery path.
- Treat the page as a combined how-to and reference page. Keep commands and UI
  names synchronized with the shipped behavior.

## Tests

- Classifier tests cover the reported npm error, old and new npm prefixes,
  bounded sanitization, and false positives.
- Managed command tests prove that normal launches stay offline-preferred and
  recovery changes only the npm freshness flag.
- Cache helper tests prove exact-key deletion, absent-key idempotency, and path
  and symlink refusal.
- Lifecycle tests prove one successful retry, one terminal repeated failure,
  preserved command identity, no intermediate failed event, stale event
  rejection, and cancel or shutdown precedence. They also prove no retry for
  remote, native, passthrough, and unrelated failures.
- Orchestrator tests prove structured error persistence and the single runtime
  retry action for sessions with and without resume tokens.
- Frontend unit tests prove structured metadata parsing and the specialized
  Kanban and Office presentations.

## E2E Tests

- **Scenario:** GIVEN a terminal failure during managed npm resolution in Kanban chat,
  WHEN the recovery card renders on desktop, THEN it names npm, keeps details
  collapsed, offers only **Retry runtime**, and sends the expected recovery
  request.
  **File:** `apps/web/e2e/tests/session/managed-runtime-npm-recovery.spec.ts`.
- **Scenario:** GIVEN the same failure on a phone viewport, WHEN the user opens
  the task and taps **Retry runtime**, THEN the action remains touch-safe, the
  chat has no horizontal overflow, and no nested overlay appears.
  **File:**
  `apps/web/e2e/tests/session/mobile-managed-runtime-npm-recovery.spec.ts`.
- **Scenario:** GIVEN the same persisted `last_agent_error` in Office chat,
  WHEN the task reloads, THEN the npm-specific copy and one retry action remain
  visible.
  **File:** `apps/web/e2e/tests/office/managed-runtime-npm-recovery.spec.ts`.

## Verification Commands

Bootstrap a fresh worktree before frontend checks:

```bash
cd apps
pnpm install --frozen-lockfile
```

Run focused backend tests:

```bash
cd apps/backend
go test ./internal/agent/runtime/routingerr ./internal/agent/agents ./internal/agent/managedruntime
go test ./internal/agent/runtime/lifecycle -run 'Test.*ManagedRuntime.*Npm|Test.*StartupRecovery'
go test ./internal/orchestrator -run 'Test.*ManagedRuntime.*Recovery|Test.*RecoveryActions|Test.*LastAgentError'
```

Run focused frontend and contract checks:

```bash
cd apps/web
pnpm exec vitest run components/task/chat/messages/action-message.test.tsx components/task/simple/chat-entries.test.ts components/task/simple/components/run-error-entry.test.tsx lib/session-last-agent-error.test.ts
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

Build once and run the desktop and mobile browser scenarios:

```bash
make build-web
make build-backend
cd apps/web
pnpm e2e:run --no-build tests/session/managed-runtime-npm-recovery.spec.ts tests/office/managed-runtime-npm-recovery.spec.ts
pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-managed-runtime-npm-recovery.spec.ts
```

Validate public documentation:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-classify-and-build-recovery-command](task-01-classify-and-build-recovery-command.md)

Wave 2:

- [x] [task-02-retry-managed-runtime-launch](task-02-retry-managed-runtime-launch.md)

Wave 3:

- [x] [task-03-present-npm-recovery](task-03-present-npm-recovery.md)

Wave 4:

- [x] [task-04-prove-responsive-recovery](task-04-prove-responsive-recovery.md)
- [x] [task-05-document-runtime-recovery](task-05-document-runtime-recovery.md)

Tasks 04 and 05 can run in parallel after the backend and frontend contracts
are stable. The default execution order remains sequential. No subagent
delegation is authorized by this plan.
