---
id: "16-harden-reattachment-and-preemption"
title: "Harden task-host reattachment and terminal preemption"
status: completed
wave: 8
depends_on: ["15-harden-shared-environment-recovery"]
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 16: Harden Task-Host Reattachment and Terminal Preemption

## Acceptance

- Backend startup adopts equal live generations into capacity before controls and treats ambiguous
  error snapshots as possibly live.
- Missing task-host cache state triggers existing-only physical reattachment before creation;
  unsupported or uncertain probes cannot launch, resume, or provision resources.
- Explicit Stop, Disabled policy, and terminal task mutation cancel blocked Start/install work
  before waiting for language or task admission.
- A selected document root's canonical identity comes from the same OS root handle that remains
  pinned; replacing its pathname between resolution and pinning cannot authorize an outside root.
- Browser task/language/session leases survive WebSocket generations and one session's reconnect
  cannot silently detach another session.
- Task subscription acknowledgement precedes the authoritative post-subscribe refresh, closing the
  stable-state lost-event window.
- Workspace config commit and all live runtime, hub, folder-notification, and snapshot updates are
  serialized per task, with no stale refresh applying after a newer commit.
- Capacity release reserves and promotes the next task asynchronously under controller lifecycle;
  Stop does not wait for another task's launch, canceled promotion cannot leak capacity, and Close
  cancels and joins active promotion work.

## TDD Evidence

- Root-handle replacement and identity-mismatch regressions failed before same-handle pinning.
- Multi-session reconnect, final-lease release, unavailable close, and activation retry regressions
  failed before manager-owned stable leases and reconnect.
- Concurrent workspace refresh ordering failed before the per-task ref-counted update gate.
- Equal-snapshot capacity, cleanup-error recovery, and startup-admission regressions failed before
  synchronous startup reconciliation.
- Detached/unhealthy task-host lookup and no-create executor regressions failed before physical
  existing-only reattachment.
- Blocked Start versus Stop and terminal task-mutation regressions failed before command and task
  admission cancellation.
- Task subscription readiness and post-ACK hydration regressions failed before acknowledged task
  subscriptions.
- Cross-task Stop/promotion, canceled-reservation handoff, and controller-close regressions failed
  before lifecycle-owned asynchronous capacity promotion; the focused set passed 20 race-enabled
  repetitions.

## Verification

Completed on 2026-08-13 after rebasing onto `origin/main`. Verification results:

- `go test -race -count=1 ./internal/lsp ./internal/agent/runtime/lifecycle
  ./internal/agentctl/server/lsp ./internal/gateway/websocket` passed.
- `go test -race -count=20 ./internal/task/service -run
  '^TestServiceTerminalMutationCancelsActiveTaskLSPAdmission$'` passed. The full task-service race
  package also exercised the changed path; one broad run exceeded the package timeout while waiting
  in unrelated SQLite schema setup, so the deterministic changed-path race test was repeated twenty
  times instead of weakening the package timeout.
- Both GitHub backend package-shard selections, 291 packages total, passed locally under `-race`.
- The four focused frontend reconnect, task-subscription, and browser-lease files passed all 36
  tests; `pnpm run typecheck` and changed-file ESLint with zero warnings passed.
- `pnpm run build:vite`, the public-docs validator (60 tests), i18n checks, and i18n ratchet passed.
- The production Playwright scenarios for concurrent same-task access and explicit restart/stop
  generation behavior passed against rebuilt production artifacts.
- The agentctl LSP package cross-compiled for Windows amd64. GitHub's native Windows containment
  check passed the selected-root junction and hostile-UNC tests after the test was updated to use a
  real pinned root handle.
- Commit hooks passed architecture, formatting, changed-code Go/Web lint, i18n, public-copy, and
  Conventional Commit checks.
- Independent review findings for root identity, browser lease recovery, workspace refresh ordering,
  detached task-host recovery, startup capacity adoption, terminal preemption, task subscription
  readiness, asynchronous capacity promotion, and E2E strength were reproduced and closed with
  deterministic regressions. PR-wide exact-head CI and read-only review remain delivery gates rather
  than implementation-task acceptance criteria.

## Files

- `apps/backend/internal/lsp/**`
- `apps/backend/internal/task/service/**`
- `apps/backend/internal/agent/runtime/lifecycle/**`
- `apps/backend/internal/agentctl/server/lsp/**`
- `apps/backend/internal/backendapp/task_lsp.go`
- `apps/web/lib/lsp/**`
- `apps/web/lib/ws/**`
- `apps/web/hooks/use-lsp*`
- `apps/web/hooks/domains/lsp/**`
- `docs/decisions/2026-08-05-task-scoped-lsp-ownership.md`
- `docs/specs/lsp-file-intelligence/spec.md`
