---
id: "03-reviewed-bulk-updates"
title: "Add reviewed bulk runtime updates"
status: done
wave: 3
depends_on:
  - "02-runtime-update-manager"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-MANAGER-001
  - REQ-AGENTS-RUNTIME-MANAGER-002
  - REQ-AGENTS-RUNTIME-MANAGER-003
acceptance_criteria:
  - AC-AGENTS-RUNTIME-MANAGER-001.2
  - AC-AGENTS-RUNTIME-MANAGER-001.6
  - AC-AGENTS-RUNTIME-MANAGER-001.7
  - AC-AGENTS-RUNTIME-MANAGER-002.3
  - AC-AGENTS-RUNTIME-MANAGER-002.4
  - AC-AGENTS-RUNTIME-MANAGER-003.1
  - AC-AGENTS-RUNTIME-MANAGER-003.2
  - AC-AGENTS-RUNTIME-MANAGER-003.3
  - AC-AGENTS-RUNTIME-MANAGER-003.4
  - AC-AGENTS-RUNTIME-MANAGER-003.5
  - AC-AGENTS-RUNTIME-MANAGER-003.6
system_design:
  - ../../specs/agents/system-design/runtime-update-manager.md
---

# Task 03: Add reviewed bulk runtime updates

## Summary

Add Update all and Update selected with exact reviewed targets, guarded individual admission, and per-runtime outcomes.
Reuse the existing server jobs and maintenance boundary. Keep unconfirmed requests explicit and successful rows out of retries.
Complete public documentation and specification supersession after the implementation passes its targeted checks.

## In scope

- Eligibility, exact previews, checkbox selection, preserved exclusions, and distinct availability versus bulk counts.
- Optional `reviewed_runtime` evidence in existing preview/request DTOs, frontend types, and the existing endpoint.
- Backend comparison before policy changes, strict update-only admission, and guarded managed preparation/activation.
- Store-owned submission state, one active run, sequential individual admissions, WS result projection, and bounded shared job recovery.
- Per-row progress, compatibility notices, partial failure, failed-only review/retry, and unconfirmed-result handling.
- Close/reopen, SPA navigation, authentication/permission changes, and reload/reconnect without mutation replay.
- Dirty-policy exclusion, explicit permissions, locales, user guidance, and final replacement of obsolete layout contracts.
- TDD across the backend admission boundary, pure coordinator logic, API encoding, and desktop/phone flows.

## Out of scope

- A server batch endpoint, durable browser queue, transaction across agents, or automatic retry.
- Bulk native self-update, external updates, managed fallback, migration, rollback, repair, or return-to-default.
- Changed automatic consent defaults, new installers, metrics, remote caches, or replacement of live sessions.

## Acceptance

1. Only eligible exact managed updates enter bulk approval. The server rejects stale review evidence before consent changes or guarded activation and preserves legacy individual requests.
2. One approval attempts each reviewed row once, reports actual matched job outcomes, continues independent failures, and retries only explicitly reviewed failed rows.
3. Approved work survives overlay/SPA navigation. Reload and lost responses never replay mutations or claim unsupported success. Desktop/phone controls, locales, docs, and final contracts agree.

## ASCII UI preview

Excerpts from [UI-02, UI-04, and UI-06](plan.md#ascii-ui-preview), covering 003.1 through 003.6.

```text
UI-02: Reviewed overview, desktop
+------------------------------------------------------------------+
| Runtime updates                                              [X] |
|------------------------------------------------------------------|
| [x] Claude  0.100.0 -> 0.103.0       [Update] [Manage versions]    |
| [ ] Codex   2.5.0 -> 2.6.0          [Update] [Manage versions]    |
| OMP         Individual updater      [Review update]               |
| Cursor      Managed externally      [Manual guidance]             |
|------------------------------------------------------------------|
| [Close]                                    [Update selected (1)] |
+------------------------------------------------------------------+

UI-04: Phone selection and fixed footer
Claude
[x] 0.100.0 -> 0.103.0
[Update] [Manage versions]
... one scrolling body ...
--------------------------------------
[        Update selected (1)         ]
              safe area

UI-06: Actual outcomes
Claude       Updated
Codex        Validating candidate...
Codex server Failed: candidate could not start   [Details]
OpenCode     Submitting...
--------------------------------------
1 updated, 1 failed, 2 in progress.              [Close]

After completion: 3 updated, 1 failed.    [Retry failed (1)]
Unconfirmed result:                     [Refresh status]
```

The overview supplies review before the named action. No additional generic confirmation dialog is required.
An empty selection disables approval. Retry returns failed rows to exact-target review before another submission.
The footer remains visible, phone actions have actual targets of at least 44px, and logs stay within the same detail surface.

## Verification

Write and run failing stale-review, duplicate-submission, partial-failure, and lifecycle tests before production changes.
Run this complete block from the repository root after implementation:

```bash
(cd apps/backend && go test ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/agent/settings/dto -run 'Test.*(Reviewed|AgentUpdate|RuntimeUpdate|Automatic|OpenCode)' -count=1)
(cd apps/backend && go test -race ./internal/agent/settings/controller -run 'TestReviewedAgentUpdate' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/settings/controller/agent_update_review.go apps/backend/internal/agent/settings/controller/agent_update_review_test.go apps/backend/internal/agent/settings/controller/agent_update.go apps/backend/internal/agent/settings/controller/agent_update_job.go apps/backend/internal/agent/settings/handlers/handlers.go apps/backend/internal/agent/settings/handlers/agent_update_handlers_test.go apps/backend/internal/agent/settings/dto/dto.go
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm exec vitest run lib/agents/runtime-update-manager.test.ts lib/agents/runtime-update-batch.test.ts lib/api/domains/agent-update-api.test.ts hooks/domains/settings/use-agent-runtime-updates.test.tsx hooks/domains/settings/use-agent-runtime-update-statuses.test.tsx hooks/domains/settings/use-agent-runtime-update-batch.test.tsx components/settings/agent-runtime-update-manager.test.tsx components/settings/agent-runtime-update-manager-review.test.tsx components/settings/use-runtime-auto-update-policy.test.ts lib/agents/runtime-update-statuses.test.ts)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings lib/agents/runtime-update-manager.ts lib/agents/runtime-update-batch.ts lib/api/domains/agent-update-api.ts lib/state/slices/settings hooks/domains/settings/use-agent-runtime-updates.ts hooks/domains/settings/use-agent-runtime-update-batch.ts components/settings/agent-runtime-update-manager.tsx e2e/tests/settings/agent-runtime-bulk-update.spec.ts e2e/tests/settings/mobile-agent-runtime-bulk-update.spec.ts e2e/tests/settings/agent-runtime-bulk-update-helpers.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-runtime-bulk-update.spec.ts tests/settings/agent-runtime-notifications.spec.ts tests/settings/agent-runtime-update.spec.ts tests/settings/agent-self-update.spec.ts tests/settings/agent-runtime-update-copilot.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-runtime-bulk-update.spec.ts tests/settings/mobile-agent-runtime-notifications.spec.ts tests/settings/mobile-agent-runtime-update.spec.ts tests/settings/mobile-agent-self-update.spec.ts tests/settings/mobile-agent-runtime-update-copilot.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The review helper, batch coordinator/hook, their tests, and bulk E2E files are new files from this work order.
Include every additional changed Go file in scoped lint.
Include each extracted frontend module in changed-file lint and its focused tests.
Keep tests for other callers of the individual endpoint. Missing review evidence must disable bulk without breaking single-runtime updates.
Use fake updaters and temporary stores for Go tests. Use intercepted fixture requests and matched WS jobs for browser behavior.
Cover source/family changes before preparation and during candidate validation, and assert no native update command executes.
Cover a different active job, a POST response lost after admission, observer unmount during shared recovery, and out-of-order list/live results.
Use request counters for failed-only retry and reload without replay. Retain server job IDs across close/reopen assertions.
Record actual counts and results only after every command completes. Run desktop and phone E2E sequentially with fresh builds.

## Files likely touched

- `apps/backend/internal/agent/settings/dto/dto.go`.
- `apps/backend/internal/agent/settings/handlers/handlers.go` and `agent_update_handlers_test.go`.
- `apps/backend/internal/agent/settings/controller/agent_update_review.go` and its tests (new guarded admission helper).
- `apps/backend/internal/agent/settings/controller/agent_update.go`, `agent_update_job.go`, and focused guard/activation tests.
- `apps/web/lib/api/domains/agent-update-api.ts` and its tests.
- `apps/web/lib/agents/runtime-update-manager.ts` and its tests.
- `apps/web/lib/agents/runtime-update-batch.ts` and its tests (new store-owned coordinator).
- `apps/web/hooks/domains/settings/use-agent-runtime-update-batch.ts` and its tests (new adapter).
- `apps/web/hooks/domains/settings/use-agent-runtime-updates.ts` and its tests for shared dependency-neutral request logic.
- `apps/web/lib/state/slices/settings/{types.ts,settings-slice.ts}` and related initial-state/type wiring for transient batch state.
- `apps/web/components/settings/agent-runtime-update-manager.tsx` and its tests.
- New desktop/phone bulk E2E specs and shared helpers under `apps/web/e2e/tests/settings/`.
- Locale `agents.json` catalogs and `docs/public/agents-and-profiles.md`.
- The manager requirement/design, linked notification/summary presentation sections, historical plan supersession notes, and this package's statuses/results.

## Dependencies

Task 02 supplies the manager overview, shared detail presentation, policy-dirty state, and target integration.

## Risks

- Current exact-version requests lack review preconditions. A client-only freshness test cannot prevent a cross-tab source or selection race.
- An activation guard must follow existing lock order and avoid native preparation for a managed-only bulk request.
- Legacy hooks can hydrate a different active job after conflict. A bulk row must never adopt that job as its approved result.
- An admitted manual update disables consent before candidate work. A rejected stale review must leave consent unchanged.
- Unmount cleanup must not cancel approved work. Authority changes must stop remaining unsubmitted requests without publishing late results.
- The server has no durable aggregate batch. Reload can show retained individual evidence only.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/runtime-update-manager.md), REQ-AGENTS-RUNTIME-MANAGER-003 and referenced manager behavior.
- [Design](../../specs/agents/system-design/runtime-update-manager.md), Bulk review through Lifetime and recovery.
- Existing `runtimeActivationGuard`, `maintenanceCoordinator`, update job merge actions, API request tests, and runtime E2E fixtures.
- Scoped backend/frontend guidance, TDD, E2E, mobile parity, and public documentation guidance.

## Results

Completed 2026-11-10. Added optional server review evidence on preview and pinned admission, guarded managed activation, exact preview review with two active requests, store-owned sequential submission, per-row outcomes, failed-only retry, retained-job recovery, and desktop/phone bulk specs. Backend settings tests and the reviewed race test passed; scoped Go lint reported 0 issues. Frontend focused Vitest run passed 70 tests across 11 files, including authority fencing, malformed success responses, fresh retry evidence, completion reset, and WebSocket reconnect recovery. Typecheck, i18n check, i18n ratchet, changed-file Oxlint, public-doc validation, and specification lint passed. Bulk E2E passed: desktop 2/2 and phone 2/2, including partial failure, fresh-evidence retry, completion-to-review, and dismissal during an active run. The nine static-review corrections and their regressions are recorded in plan.md.
