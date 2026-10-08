---
id: "02-browser-retry"
title: "Present retryable inspection contention"
status: done
wave: 2
depends_on:
  - "01-resume-admission"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
acceptance_criteria:
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.6
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.7
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.8
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.9
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
---

# Task 02: Present retryable inspection contention

## Summary

Treat inspection contention as a request-local retry outcome on desktop and phone.
Keep the pending indication during backend waiting. An exhausted wait offers
same-session resume without workspace fallback or fabricated failure history.

## In scope

- Recognize the existing structured conflict kind in the recovery service.
- Handle it before workspace fallback and rollback optimistic local STARTING.
- Map manual and automatic recovery feedback to one localized message.
- Retain the existing recovery owner, same-session action, draft, and attachments.
- Add deterministic desktop/phone browser coverage and a scoped gateway proxy.
- Add the message to every shipped locale and generated pseudo catalog.

## Out of scope

New routes, stores, recovery actions, layout redesign, automatic retry loops,
task-state repair, and weaker recovery authorization.

## Acceptance

1. Typed contention produces one localized retry notice, no restore request, and
   no task-failed toast/history. Generic failures retain existing workspace fallback.
2. Matching retry keeps session/environment/provider identity and draft content.
   Stale results, navigation, cancellation, and archive cannot publish stale feedback.
3. Desktop and phone show pending feedback and reachable same-session retry.
   Phone preserves the existing touch geometry and zero horizontal overflow.

## ASCII UI preview

Use UI-01, UI-02, and UI-03 from the [full preview](plan.md#ascii-ui-preview).
These views implement criterion 003.8.

```text
Pending: [spinner] Resuming session

Exhausted, desktop:
This workspace is still being checked.
Try resuming the session again.
[Resume session]

Exhausted, phone:
This workspace is still being
checked. Try resuming the
session again.
[       Resume session       ]

Success: existing composer, retained draft and attachments
```

Reuse `SessionRecoveryCard` and `RecoveryActions`. Keep one inline region and its
current scroll owner. Phone controls retain at least 44-pixel touch targets.
No new overlay, drawer, or hidden desktop recovery owner is needed.

## Regression first

Add `use-session-resumption-inspection-contention.test.ts` with a structured
`recovery_inspection_busy` exception. Assert zero restore_workspace requests,
restored optimistic state, no workspace_read_only outcome, and one retry notice.
Record its current fallback failure before implementation.

Extend service tests for typed recognition, unrelated conflicts, wrapped transport
errors, localization, and legacy fallbacks. Add action-hook coverage for explicit
resume, no destructive bypass, stale attempts, and successful notice retirement.
Do not classify contention through message-text matching.

Add browser specs for UI-01 through UI-03. Adapt the existing session-entry and
archived-recovery gateway proxy pattern. Reject the selected first resume request
before forwarding it, then allow the user retry to reach the real backend.
Keep all unrelated frames transparent and scope interception to task/session.

Use a worker-owned backend and a recoverable retained conversation. The existing
`session-resume-keeps-review-state.spec.ts` supplies the restart/REVIEW fixture
pattern. Arm request/state observers before navigation. Prove no restore request
during the causal observation window and persisted REVIEW after recovery.
Send a new marker after retry to prove that the same conversation accepts input.
Capture a phone screenshot of the exhausted state and check action geometry.
Backend lock contention remains Task 01's proof, not a claim from the proxy.

## Verification

Run from the repository root. If workspace dependencies are absent, first run
`pnpm install --frozen-lockfile` from `apps/`.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-resumption-inspection-contention.test.ts hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption-guard-refusal.test.ts hooks/domains/session/use-session-recovery-actions-inspection-contention.test.ts lib/services/session-recovery-service.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/services/session-recovery-service.ts hooks/domains/session/use-session-resumption-operations.ts hooks/domains/session/use-session-recovery-actions.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-open-inspection-contention.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-open-inspection-contention.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run desktop and mobile managed runners sequentially with fresh production builds.
Record discovered test counts, results, and the phone screenshot location.
Include any additional changed production TypeScript files in the focused eslint command.

## Files likely touched

- `apps/web/lib/services/session-recovery-service.ts` and its existing test.
- `apps/web/hooks/domains/session/use-session-resumption-operations.ts`
- `apps/web/hooks/domains/session/use-session-resumption-inspection-contention.test.ts` (new)
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`
- `apps/web/hooks/domains/session/use-session-recovery-actions-inspection-contention.test.ts` (new)
- `apps/web/e2e/helpers/session-open-inspection-contention.ts` (new)
- `apps/web/e2e/tests/session/session-open-inspection-contention.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-session-open-inspection-contention.spec.ts` (new)
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/task.json`

Reuse existing markup. If this outcome needs a narrower action list, change the
recovery view-model. Add focused tests.

## Dependencies

Task 01 supplies safe server bookkeeping and the unchanged typed conflict.

## Risks

A proxy that rejects only the response permits an unseen upstream agent launch.
An unscoped proxy can hide unrelated failures. Existing recovery renderers must
not interpret a request-local conflict as an unresolved durable startup failure.

## Parallelism

`sequential`

## Inputs

- Requirement 003 and its inspection-contention amendment.
- System design: Browser response and recovery, Verification boundary.
- `use-session-resumption-guard-refusal.test.ts` and service/action-hook tests.
- `apps/web/e2e/helpers/session-entry-recovery.ts`.
- `/mobile-parity`, `/e2e`, and scoped web guidance.

## Results

Implemented typed `recovery_inspection_busy` recognition and request-local
feedback. Automatic and manual recovery now preserve the session after safe
contention, skip workspace-only fallback, and clear only the matching notice on
successful same-session resume. Desktop and phone reuse the existing inline
recovery owner; the phone retry retains its measured touch target and draft.

Review remediation routes the retry through the notice owner. The automatic
retry override is supplied only when the task/session-matched automatic owner
holds the inspection notice and no manual notice is active. Otherwise the button
calls the manual recovery action, retaining policies such as `provider_restored`.
Rendered-card regressions cover no automatic owner, a mismatched owner, the
matching automatic owner, and provider-restored policy on the manual retry.

Validation passed:

- Current rendered-card and recovery-action Vitest checks: 34 tests across 4
  files. Web typecheck, focused ESLint with zero warnings, and production Vite
  build passed.
- Focused Vitest regression: 142 tests across 9 files.
- Web typecheck and focused ESLint over all changed production TypeScript files;
  ESLint passed with zero warnings.
- `i18n:check` and `i18n:ratchet`.
- Desktop Chromium E2E: 1 passed. Phone `mobile-chrome` E2E: 1 passed. Both
  managed runs rebuilt the backend and production Vite assets. A host replay
  captured and visually checked the phone exhausted-state view.
- Screenshot: `apps/web/e2e/test-results/session-mobile-session-ope-31ab7-ion-and-preserves-the-draft-mobile-chrome/session-open-inspection-contention-mobile.png`.
- The Task 01 orchestrator regression and the prescribed backend race suite,
  document catalog validation, specification lint, and `git diff --check`.
- Desktop and phone E2E were not rerun for the review correction because it only
  changes retry callback ownership; the new rendered-card tests exercise both
  branches without changing layout or touch behavior.
