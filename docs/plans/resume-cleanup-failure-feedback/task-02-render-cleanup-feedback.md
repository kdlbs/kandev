---
id: "02-render-cleanup-feedback"
title: "Explain cleanup failure in recovery UI"
status: done
wave: 2
depends_on:
  - "01-preserve-cleanup-failures"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.8
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.9
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.12
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.13
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 02: Explain cleanup failure in recovery UI

## Summary

Use the backend's safe typed cause for one localized cleanup-failure explanation
in the existing session error surface. Verify manual and automatic failures,
reload, pending state, and later success on desktop and phone.

## In scope

Browser cause types/parsing and request evidence; recovery model copy; seven
shipped locale catalogs and generated variants; focused model/card regressions;
shared mock-backed desktop/mobile E2E; public recovery guidance and validators.

## Out of scope

New recovery operations, resource-scope changes, global error-bar redesign,
new overlays, raw Kubernetes text matching, and live-cluster manipulation.

## Acceptance

1. New typed cleanup causes choose localized cause-first guidance. Unknown or
   legacy records retain fallback. Both resume and distinct restore causes stay
   readable with safe, initially collapsed, copyable details and current identity.
2. Desktop and phone have one active recovery surface, eligible pending-disabled
   actions, preserved draft/history/session after reload, and no horizontal
   overflow. Later owned success clears only matching controls. Phone targets
   measure at least 44px; a rendered phone check matches UI-03.
3. Locale and public how-to guidance describe checking executor connectivity and
   retrying the existing session without asserting confirmed runtime death,
   data restoration, or a fresh-start bypass.

## ASCII UI preview

UI-02/UI-03, entry: selected failed session, composer replaced by its recovery
card. Full context and expanded/pending notes are in the
[plan preview](plan.md#ascii-ui-preview).

```text
Desktop:
[!] Resume could not finish
The previous runtime could not be stopped.
Check executor connectivity, then retry Resume.
[Resume] [existing eligible actions]   > Details

Phone:
[!] Resume could not finish
The previous runtime could not
be stopped. Check executor
connectivity, then retry Resume.
[          Resume           ]
[ existing eligible action  ]
> Details
```

Wording is illustrative; title/cause/action/disclosure order, phone stacking,
touch geometry and one recovery scroll owner are structural. Maps to
002.5/.8/.9/.12/.13. Reuse `SessionRecoveryCard` and the native task layout;
do not create another modal or drawer for this session-specific copy.

## Verification

Install workspace dependencies if absent before package commands. Add failing
typed-cause/model regressions first. From repository root:

```bash
(cd apps/web && pnpm exec vitest run lib/session-last-agent-error.test.ts components/task/chat/session-bootstrap-recovery-model.test.ts components/task/chat/session-recovery-model.test.ts components/task/chat/session-recovery-card-details.test.tsx components/task/chat/session-recovery-card-inspection-retry.test.tsx)
(cd apps/web && pnpm exec vitest run lib/active-session-recovery.test.ts lib/services/session-recovery-service.test.ts hooks/domains/session/use-session-recovery-actions-cleanup.test.ts hooks/domains/session/use-session-resumption.test.ts components/task/chat/messages/action-message-cleanup-history.test.ts components/task/workspace-unavailable.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run tests/session/session-error-recovery-ui.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-error-recovery-ui.spec.ts -- --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run changed-file ESLint/Prettier on actual edited TS/TSX and catalogs, plus any
additional changed hook/model tests. E2E runs are sequential under the guarded
runner. Inspect a rendered phone screenshot and record geometry/scroll results.
Update this command list if implementation adds a distinct test boundary.

## Files likely touched

- `apps/web/components/task/workspace-unavailable.tsx` and its owner-correlation tests
- `apps/web/lib/active-session-recovery.ts` and its tests
- `apps/web/lib/services/session-recovery-service.ts` and its tests
- `apps/web/lib/session-last-agent-error.ts` and its tests
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts` and existing related tests
- `apps/web/hooks/domains/session/session-recovery-operation-fence.ts`
- `apps/web/hooks/domains/session/use-session-resumption-operations.ts` and existing resumption tests
- `apps/web/hooks/domains/session/use-session-recovery-actions-cleanup.test.ts` (new)
- `apps/web/components/task/chat/session-bootstrap-recovery-model.ts` and its tests
- `apps/web/components/task/chat/session-recovery-model.ts` and its tests
- `apps/web/components/task/chat/session-recovery-card.tsx` and existing safe-details tests
- `apps/web/components/task/chat/session-recovery-card-inspection-retry.test.tsx`
- `apps/web/components/task/chat/session-bootstrap-recovery-content.tsx`
- `apps/web/components/task/chat/messages/action-message-recovery-history.tsx` and cleanup history regression
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/task.json` and generated pseudo catalog
- `apps/web/e2e/helpers/session-error-recovery-ui.ts`
- `apps/web/e2e/tests/session/session-error-recovery-ui.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-error-recovery-ui.spec.ts`
- `docs/public/sessions-and-review.md`, `docs/public/k8s.md`
- This plan, work orders, and paired specs for final results/lifecycle synchronization.

## Dependencies

Task 01 must establish the code, safe cause and response/error-stamp contract.

## Risks

Request errors may arrive before durable events. Keep cause and attempt/stamp
correlation instead of flattening into strings. Fresh start shares unavailable
runtime boundaries and is not a connectivity repair. Expanded/copy details must
never fall back to raw transport payloads. Supply all languages, not only English.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/agent-resume-runtime-recovery.md), requirement 002.
- [Design](../../specs/agents/system-design/agent-resume-runtime-recovery.md#runtime-cleanup-failure-feedback).
- `/mobile-parity`, `/e2e`, `/tdd`, and `/docs-maintainer` guidance during implementation.
- Existing desktop/mobile recovery E2E helpers and safe-details primitive.

## Results

Implemented and verified on October 10.

- Manual and automatic failures preserve the host-owned cleanup cause. The
  existing recovery card uses localized title/body copy in seven shipped
  languages and generated pseudo, safe details and matching-stamp history.
- Current manual and automatic `inspection_busy` notices take precedence over
  retained cleanup guidance while the cleanup code remains in Details. Ordinary
  informational notices preserve the cleanup fallback. Hook regressions and the
  actual desktop retry path reproduced the hidden notice before this correction.
- Cancelled and completed sessions can present a current cleanup failure
  without changing their lifecycle state. Matching resolution removes the
  active controls while retaining localized history and the composer draft.
- A baseline failure reproduced at `19237eb23` exposed a lost Files-to-Chat
  link when workspace readiness superseded a restore attempt. The matching
  automatic session owner now remains reachable; unrelated sessions and
  restoration attempts keep their independent controls. That automatic-recovery
  E2E fixture is unchanged.
- Model, service, hook, history and owner-correlation unit regressions passed,
  including automatic resume with both successful and failed workspace restore.
  Typecheck, changed-file ESLint, localization check and new-copy ratchet passed.
- Full frontend CI exposed an older recovery-card test mock that omitted the
  new cleanup parser export. The three failures reproduced locally; changing
  that mock to preserve real service exports passed all four inspection-retry
  cases and eight presentation-model cases together.
- Integration with main's silent restart recovery preserves its no-prompt
  contract and canonical operation-fence types. Cleanup causes follow the moved
  manual-failure type; the combined service, hook, model and card selection
  passed all 170 cases after conflict resolution.
- The combined hook exceeded the 600-line lint limit. Response association now
  lives beside the existing operation-fence helpers without changing behavior;
  zero-warning lint and all 41 affected hook/card/model cases passed afterward.
- Integration with record reconstruction on main `dac20ecf4` preserves its
  delivery-owner request identity and extracted eligibility helper. Two added
  hook cases keep current-owner cleanup feedback and reject a delayed cleanup
  response after delivery ownership changes. All 151 selected frontend cases
  passed on this combined tree without weakening the operation fence.
- Final guarded desktop and phone suites each passed all ten scenarios with
  zero retries. Cleanup coverage includes pending disablement, failed cleanup
  retry, a subsequent inspection-contention notice with retained cleanup details,
  collapsed/expanded details, reload and retained history. The successful
  resolution fixture uses the authoritative matching error stamp, not a live
  Kubernetes repair. Existing cases cover clipboard safety and draft retention.
- Phone controls meet the 44px target and both viewports have no horizontal
  overflow. Desktop and phone screenshots were inspected with synthetic data.
- Public docs tests/validation, catalog and full spec lint passed. Deterministic
  documentation coverage includes the actual production paths and work orders.
