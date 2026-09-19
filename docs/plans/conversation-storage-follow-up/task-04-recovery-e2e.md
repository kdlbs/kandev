---
id: "04-recovery-e2e"
title: "Close desktop and mobile recovery coverage"
status: done
wave: 2
depends_on:
  - "01-postgres-coverage"
  - "02-office-migration"
  - "03-task-service"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-006
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.7
  - AC-PLUGINS-PROMPT-HISTORY-HOST-005.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-005.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.4
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.5
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.6
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.7
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.8
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.9
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.10
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.12
system_design:
  - ../../specs/plugins/system-design/conversation-source-reconciliation.md
---

# Task 04: Close desktop and mobile recovery coverage

## Summary

Prove the existing core and plugin recovery behavior after the backend work orders finish.
Extend missing deterministic scenarios and fix only defects exposed by this recovery matrix.

## In scope

- Own recovery E2E tests, fixture controls, and any narrowly required client/gateway fixes.
- Run core and plugin gap recovery, pagination after repair, prompt history, and stream isolation.
- Cover a dropped final change while idle, equal-revision checks, irrelevant agent streaming,
  complete hydration beyond 100 turns, binding renewal/reconnect, and access revocation after readiness.
- Assert stable IDs, current turn durations, preserved loaded ranges, no deleted-row resurrection,
  and no history rereads for equal revisions or fully covered irrelevant updates.
- Inspect existing unit tests first. Keep direct-SQL and timer edge cases at the deterministic
  repository/client boundary when browser controls cannot express them; record this limitation.
- Add a focused mobile recovery scenario to an existing mobile spec if its current flow lacks it.
  Mobile uses the Panels picker, full-height panel, touch controls, and one scroller.
- Use approved E2E helpers, no arbitrary sleeps or overlapping full suites. Read the E2E and
  mobile-parity skills. Fresh worktrees need the frozen workspace install before pnpm commands.
- Record results at the final code commit and synchronize original Task 01/04/05 and companion
  plan evidence without replacing historical counts. Do not mark any unrun matrix complete.

## Out of scope

UI redesign, plugin extraction, new auth contracts, and a generic full verification campaign.

## Acceptance

1. All named desktop and mobile suites pass after the final fixes, with actual counts and tested SHA.
2. The scenario matrix maps each required recovery condition to passing browser or explicitly named
   lower-level evidence; mobile has real recovery evidence rather than only panel placement.
3. The original package's remaining gates are reconciled accurately; unresolved failures remain open.

## Verification

Run from the repository root. Run the browser commands sequentially with the guarded runner.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/conversation-recovery.spec.ts tests/task/prompt-history-panel.spec.ts tests/session/session-stream-overload-isolation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-prompt-history-plugin.spec.ts tests/task/mobile-prompt-history-panel.spec.ts tests/session/mobile-session-stream-overload-isolation.spec.ts)
(cd apps/web && pnpm exec vitest run lib/plugins/conversation-source-scope.test.tsx lib/plugins/conversation-reconciliation.test.ts lib/ws/client.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/backend && go test -race ./internal/gateway/websocket -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If fixes touch additional tests, add their exact commands to Results and run them too.
A later production-code change invalidates relevant earlier final-head evidence; rerun affected checks.

## Files likely touched

- `apps/web/e2e/tests/plugins/conversation-recovery.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-prompt-history-plugin.spec.ts`
- `apps/web/e2e/tests/task/prompt-history-panel.spec.ts`
- `apps/web/e2e/tests/task/mobile-prompt-history-panel.spec.ts`
- `apps/web/e2e/tests/session/session-stream-overload-isolation.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-stream-overload-isolation.spec.ts`
- `apps/web/e2e/helpers/ws-drop.ts` and existing fixture controls.
- `apps/web/lib/plugins/conversation-source-scope.ts`, `conversation-scope.tsx`, and associated tests.
- `apps/backend/internal/gateway/websocket/conversation_delivery.go` if a reproduced defect requires it.

## Risks

Long-lived authorization and binding tests can become slow or flaky. Use causal controls and bounded
fake-clock tests for timing mechanics, plus browser evidence for the resulting user-visible recovery.

## Dependencies

01-postgres-coverage, 02-office-migration, 03-task-service

## Parallelism

`sequential`

## Inputs

- [Host requirements](../../specs/plugins/requirements/prompt-history-extraction-host.md) and [source reconciliation design](../../specs/plugins/system-design/conversation-source-reconciliation.md).
- [Architecture decision](../../decisions/2026-09-16-conversation-source-reconciliation.md).
- Implementation commit `c0a048bc128f7ef9a1051caf95ed442627faf9df`; comparison base `88c6c0fe0a6ae5d25332070d603b99f7d9241645`.
- [Original package](../conversation-storage-replacement/plan.md) and its recorded evidence.

## Results

- The final guarded Docker desktop command passed 5/5 tests: core recovery (2), plugin recovery with pagination (1), session stream isolation (1), and Prompt History panel behavior (2). An earlier full-matrix run had one transient plugin-panel timeout; its isolated rerun and the final full matrix both passed.
- The final guarded Docker mobile command passed 5/5 tests: fixture-plugin Host navigation (1), native session-sheet stream isolation (1), Prompt History panel touch navigation (1), long-history touch loading (1), and cancellation after leaving Chat (1).
- The focused frontend Vitest command passed 41/41 tests. `pnpm run typecheck` passed. `go test -race ./internal/gateway/websocket -count=1` passed, including the synthetic-client authorization regression and task-event receipt projection regression. Specification catalog validation, full specification lint, and `git diff --check` passed.
- The browser helper now drops the v2 source-change notification for the targeted prompt, so recovery tests exercise the source reconciliation path while preserving compatibility frames.
- No layout, navigation, scrolling, or touch behavior changed. The mobile tests use the existing Panels picker, full-height panel, and single scroller.
- Tested code revision: `213492517315abb38697b765e91e6fe0ea5388c7`.
