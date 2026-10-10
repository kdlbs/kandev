---
id: "05-silent-restart-recovery"
title: "Restore interrupted sessions silently"
status: in_progress
wave: 5
depends_on:
  - "02-session-recovery"
  - "03-recovery-ui"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.11
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.13
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.14
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.15
system_design:
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 05: Restore interrupted sessions silently

## Summary

Replace the PR's manual interruption flow with automatic restart recovery.
Restore the original conversation without dispatching an instruction.
This work supersedes the manual continuation portions of Tasks 02 and 03.

## Evidence and regression

`InterruptedSessionsRecovery` includes completed `continued` metadata as a candidate.
That metadata retains its previous generation and execution identity.
`recoverySessionMatches` rejects the old generation after a successful continuation.
The UI also requires a nonempty instruction, which contradicts the requested restart behavior.
The exact rejection path for the user's two live sessions remains unconfirmed.

Start with `TestRestartRecoveryRestoresNativeConversationWithoutPrompt` through the real startup service path.
The current implementation must fail because it does not restore those interrupted conversations automatically.
Add `TestContinuedRecoveryDoesNotRetryPreviousOwner` against a real SQL generation transition.
Use the existing interrupted-resume fixture without mocking the recovery method under test.

## Scope and owned files

- Add a bounded startup recovery worker in `internal/orchestrator`, integrated after lifecycle adoption and startup reconciliation.
- Cancel and join that worker through `Service.Stop`; do not create untracked recovery goroutines.
- Reuse identity-aware retained replay for surviving processes and terminal evidence.
- Separate restore-only progress from instruction snapshots in `internal/task/models` and `internal/task/repository/sqlite`.
- Use existing restore-attempt persistence where sufficient. Add a registered migration only for missing durable fields or constraints.
- Update `interrupted_session_resume.go`, `interrupted_session_dispatch.go`, and `session_delivery_recovery.go` as needed.
- Preserve guarded journal retirement in `internal/agent/runtime/lifecycle` and `internal/agentctl/journal`.
- Retire the PR-only batch continuation handler and types if no supported caller remains.
- Remove `InterruptedSessionsRecovery` mounts from `agent-runtime-unavailable-alert.tsx`, `app-status-surface-provider.tsx`, and `task-chat-panel.tsx`.
- Remove the interruption instruction form from `session-stopped-banner.tsx`, with unused components, hooks, clients, tests, and locale keys.
- Update the existing desktop and phone durable-stream recovery browser selections.
- Update `docs/public/sessions-and-review.md` through docs-maintainer and synchronize affected package records.

All backend paths above are relative to `apps/backend`; web paths are under `apps/web`.
Keep generic recovery for unrelated failures and all journal safety/ACK fixes.
Do not repair the user's live database or send a continuation message.

## Acceptance

1. A startup pass reattaches surviving owners or restores confirmed terminated conversations with zero new prompts, messages, turns, or queue dispatches.
2. Crash/retry tests cover each persistence boundary, retained native identity, unchanged uncertain submissions, independent blocks, stale owners, and mixed eligible/blocked candidates.
3. Desktop and phone show normal chat after successful recovery, with no interruption form. A subsequent explicit user message succeeds through normal admission.

## Required regression matrix

- Surviving active owner: same process and turn, retained output replayed once, no native reload.
- Terminated owner: same session, workspace, native conversation, and uncertain old submission; zero prompt dispatch.
- Recovery interruption before and after journal retirement, generation commit, and completion marker.
- Already restored `continued` metadata: no old-owner retry or repeated restore.
- New unrelated owner, unknown process liveness, missing native state, locked/corrupt journal: blocked without replacement.
- Archived/completed, Office/automation, dynamic route, permission, and capacity restrictions remain effective.
- One blocked candidate alongside a recoverable candidate: the recoverable candidate completes independently.
- Shutdown during recovery: worker exits before repository/runtime teardown.
- Browser reload and desktop/phone startup: no global form and no instruction submission.

## ASCII UI preview

UI-05, from [the package plan](plan.md#ascii-ui-preview):

```text
Before: [Resume interrupted sessions]
        [session selection] [instruction] [acknowledgment]
After:  [existing conversation]
        [normal composer]
```

The existing task chat is the phone exemplar. Successful recovery adds no surface.
Session-local failures retain safe Retry/Stop controls and the existing scroll owner.
Phone controls remain stacked, with safe-area clearance and targets of at least 44 pixels.
Assert both form absence and zero prompt dispatch; visibility alone does not prove silent recovery.

## Verification

Run commands sequentially from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/orchestrator/handlers ./internal/orchestrator/executor ./internal/task/repository/sqlite -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/session-stopped-banner.test.tsx lib/session-recovery-actions.test.ts hooks/domains/session/use-session-recovery-actions.test.ts lib/services/session-recovery-service.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/agent-survival-restart.spec.ts tests/layout/agent-runtime-replacement.spec.ts tests/session/durable-stream-recovery.spec.ts tests/session/durable-reattachment.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-agent-survival-restart.spec.ts tests/layout/mobile-agent-runtime-replacement.spec.ts tests/session/mobile-durable-stream-recovery.spec.ts tests/session/mobile-durable-reattachment.spec.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the named regression first and record its expected failure before production edits.
If persistence changes, repeat repository and store-conformance checks with `KANDEV_TEST_POSTGRES_DSN` configured.
The managed browser runner rebuilds artifacts. Do not overlap browser selections or test a stale binary.
Record unavailable Darwin/Windows, real-provider, and remote-executor gates separately.

## Dependencies and risks

Execute after Tasks 01 through 04; no parallel delegation is required.
Restore does not imply continuation of a terminated model invocation.
A successful restore waits for the next ordinary user message, as required by the no-prompt contract.
Authentication identity, queue holds, and interrupted multi-store commits require direct regression coverage.

## Results

Implementation authorized on 2026-10-09 and resumed on 2026-10-10. GPT-6 Luna max workers use disjoint backend, storage, and web ownership under the primary coordinator.

- Web changes: eight focused suites (106 tests), full lint, typecheck, and i18n checks passed. Managed desktop/phone selections await backend integration.
- Review tightened stale-error clearing to match the restored execution and submission. Its 11 focused state-merge tests, scoped lint, and typecheck passed. The production Vite build passed on the current web changes (14.19 seconds).
- Storage changes: focused `TestSilentRestore*` and additive migration replay passed under race on SQLite and PostgreSQL 16. The latest selection also passed for the durable `candidate_dead` transition and rejected unsafe direct rotation. Scoped storage lint passed. The final store-conformance race gate passed with SQLite and an isolated PostgreSQL 16 instance (406.242 seconds), including the final checkpoint implementation.
- Startup recovery: the expected RED showed that restart did not restore the native conversation. The final focused orchestrator selection passed without race instrumentation, including startup restoration, queue behavior, previous-owner and changed-generation guards, worker cancellation, candidate-dead crash boundaries, missing launch evidence, eligibility filtering, and capacity denial. The startup auth resolver, retired-handler rejection, runtime flags, configuration, and profile checks passed after fixture repairs. SQL guard passed. The integrated race gate and managed browsers remain pending.
- Public docs validation (63 validator tests, 47 published pages), specification catalog validation, and specification lint passed for the updated docs.
- The first desktop browser run exposed a blocked restore behind a terminal predecessor with no persisted PID. Exact saved process-identity proof now passes focused race tests, including live descendants, incomplete identity, remote exclusion, and inspection errors. Cleanup regressions also prove that a concurrently created workspace successor remains untouched. The workspace-only allocation guard passes race tests across all three lazy creation APIs, including the interval before recovery-block persistence, malformed evidence, historical completion, cached access, and ordinary allocation.
- Resume admission refactoring passed the focused `ResumeTaskSession` and `RecoverSession_ContextContinuation` race selection. The combined backend run passed lifecycle after integration fixes, runtime agentctl, handlers, executor, and SQLite. Final orchestrator and browser gates will run after rebase.

These results do not establish final runtime or PR readiness. Integration, managed browsers, final review, and delivery remain pending.
