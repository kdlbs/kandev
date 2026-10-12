---
id: "02-recovery-hierarchy"
title: "Unified recovery ownership and actions"
status: done
wave: 2
depends_on: ["01-safe-diagnostics"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.11
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.12
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.14
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.15
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.16
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 02: Unified recovery ownership and actions

## Follow-up

The initial pass is complete. Its active-control placement is superseded by
[work order 03](task-03-uniform-recovery.md) after the expanded fixture review.
Historical verification below does not prove the new composer-owner contract.

## Summary

Connect all scoped surfaces to the existing recovery owner and one deterministic
action hierarchy. Integrate desktop/phone secondary options and prove the two
reported examples while preserving independent errors and recovery semantics.

## In scope

Pure presentation/action derivation, explicit request-owner correlation, dependent
workspace navigation, initial ensure fallback, typed cause handling, shared pending
state, send-feedback suppression, chronological history, mobile drawer/focus flow,
locale updates, public recovery how-to and complete desktop/mobile scenarios.

## Out of scope

New operation permissions, backend identity guesses or protocol redesign, runtime
secret repair, automatic fresh starts, task reparenting, unrelated workspace errors.

## Acceptance

1. Correlated failures have one active control/announcement owner; distinct or ambiguous failures survive, including shared-task and other-session errors.
2. Every design action-table row invokes the existing operation/confirmation, with one pending request and stale-attempt protection. Resume/restore causes remain distinct.
3. Desktop and native phone paths match UI-01/UI-02, pass rendered geometry/focus/copy/history checks, and document actual verification outcomes.

## ASCII UI preview

UI-01 and UI-02 excerpt from the [combined preview](plan.md#ascii-ui-preview):

```text
Desktop Chat                     Dependent Terminal
! Session could not resume       Workspace unavailable
  <short cause>                  [View recovery]
  [Retry resume] [More options v]
  > Technical details

Phone Chat                       Phone options drawer
! Session could not resume       Recovery options   Close
<short cause>                    Restore read-only workspace
[       Retry resume       ]     Start fresh session...
[       More options       ]     <safe-area padding>
> Technical details
```

Only correlate when identity proves equivalence. Shared-task errors retain their
shell placement. Phone options are a Drawer; desktop options are a menu. Preserve
the single transcript scroll owner and confirmation flow. Criteria .11-.16 apply;
diagnostic primitive is delivered by Task 01.

## Verification

Implement with TDD. Run commands from repository root, sequentially; reuse Task
01's installed dependencies. Tests named session-error-recovery-ui and
session-recovery-actions are new planned files.

```bash
(cd apps/web && pnpm exec vitest run lib/session-error-details.test.ts lib/session-recovery-presentation.test.ts lib/session-recovery-actions.test.ts lib/state/slices/session-runtime/workspace-restoration.test.ts hooks/processed-message-filtering.test.ts hooks/domains/session/use-session-recovery-actions.test.ts hooks/domains/session/use-session-recovery-actions-guard.test.ts hooks/domains/session/use-session-resumption.test.ts components/task/session-error-details.test.tsx components/task/ensure-session-error.test.tsx components/task/workspace-unavailable.test.tsx components/task/chat/session-bootstrap-recovery-card.test.tsx components/task/chat/session-stopped-banner.test.tsx components/task/chat/messages/action-message-recovery.test.tsx components/task/chat/messages/action-message.test.tsx components/task/task-launch-error-context.test.tsx components/task/simple/components/run-error-entry.test.tsx components/task/passthrough-chat-composer.test.ts components/task/chat/chat-input-area.test.ts components/task/chat/chat-input-area.test.tsx components/task/simple/components/task-launch-error-entry.test.tsx components/task/chat/messages/agent-status.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:zh-hant --namespace task)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-error-recovery-ui.spec.ts tests/task/launch-failure-recovery.spec.ts tests/session/session-resume-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-error-recovery-ui.spec.ts tests/task/mobile-launch-failure-recovery.spec.ts tests/session/mobile-session-resume-recovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add any directly changed existing suite to this command block before marking
complete. Use the configured Pixel 5 project; no device override or broad suite.
Capture actual requests and geometry, not only button presence or CSS classes.
Manually inspect collapsed/expanded/pending desktop/phone screenshots, synthetic
secret absence, safe-area clearance and focus return. Record artifact paths.

## Files likely touched

- `apps/web/lib/session-recovery-presentation.ts`, new `lib/session-recovery-actions.ts` and associated tests.
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`, `use-session-recovery-feedback.ts`, `hooks/processed-message-filtering.ts` and their tests.
- `apps/web/components/task/task-launch-error-context.tsx`, `chat/session-bootstrap-recovery-card.tsx`, `chat/session-stopped-banner.tsx`, `chat/messages/action-message-recovery.tsx`, `ensure-session-error.tsx`, `workspace-unavailable.tsx`, `simple/components/run-error-entry.tsx` and caller adapters.
- `apps/web/components/task/passthrough-chat-composer.tsx` and other discovered send-feedback owners; domain-owned options component using existing menu/Drawer primitives.
- New desktop/mobile E2E files, existing affected recovery suites, task/chat locale catalogs, `docs/public/sessions-and-review.md`, and this package's result/status sections.

## Dependencies

Task 01 safe diagnostics. Re-read source for concurrent runtime-recovery changes
before implementation; keep its existing contracts and update this plan if a
material dependency changes.

## Risks

No correlation is preferable to incorrect suppression. Pending state must outlive
presentation changes without an extra mutation hook. A details or options trigger
can disappear after success; focus must have a deterministic surviving target.

## Parallelism

`sequential`

## Inputs

Paired design: Shared derivation and ownership; Action selection; Layout, phone
contract and accessibility. Existing task error scope ADR and predecessor plans;
`mobile/mobile-picker-sheet.tsx`, `task-layout.tsx`, launch/recovery test helpers.

## Results

Completed on 2026-09-19. See the [plan verification ledger](plan.md#verification-results)
for the 295 focused tests, desktop/mobile browser runs, localization, lint,
typecheck, document validation, screenshots, and the resolved flaky mobile run.
No runtime secret provisioning or recovery operation contract was changed.
