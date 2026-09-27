---
id: "01-safe-diagnostics"
title: "Safe readable diagnostics"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.13
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.14
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.15
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 01: Safe readable diagnostics

## Summary

Make all scoped session/recovery diagnostic text use one sanitized, copyable
presentation. Reproduce and fix the desktop text-column collapse while preserving
chronological placement and operation-specific causes.

## In scope

TDD for domain sanitizer and disclosure, replace raw summary fallbacks with
localized operation/cause copy, integrate bootstrap/request/workspace/action/run
error consumers, correct content-column geometry, and translate new copy.
Use the backend redaction fixture semantics as evidence; do not assume stripping
control characters makes text safe. Add browser details/wrapping scenarios first.

## Out of scope

Recovery action reordering and cross-surface owner changes (Task 02), secret repair,
backend persistence or sanitizer policy changes, and unrelated alert redesign.

## Acceptance

1. All scoped render/copy paths use bounded sanitized operation-labelled details; unsafe/empty data is omitted and clipboard failure never falls back to raw data.
2. Desktop icon/title/description placement and phone diagnostics use available width without nested transcript scrolling; long errors do not produce vertical character columns or page overflow.
3. Five locale catalogs, semantic disclosure, copy status and keyboard controls pass targeted tests.

## ASCII UI preview

UI-03 from the [combined preview](plan.md#ui-03-details-pending-and-resolved-states-shared-composition):

```text
! Session could not resume
  The runtime connection is unavailable.
  v Technical details       [Copy details]
  Resume: <sanitized wrapped detail>
  Restore: <separate sanitized detail>
```

All lines share the content column after the icon. Phone Copy moves to a separate
full-width row with >=44px hit area. Details use the transcript's scroll owner.
This work order implements the detail/width subset of UI-01/UI-02; Task 02 owns
action grouping and duplicate ownership. Criteria .13-.15 apply.

## Verification

Run the new tests red before implementation; new test files named here are planned.
Commands run from repository root, sequentially. Install once in a fresh worktree.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/session-error-details.test.ts components/task/session-error-details.test.tsx components/task/chat/session-bootstrap-recovery-card.test.tsx components/task/chat/messages/action-message.test.tsx lib/state/slices/session-runtime/workspace-restoration.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant --namespace task)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-error-recovery-ui.spec.ts -- --grep 'details wrap and copy safely')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-error-recovery-ui.spec.ts -- --grep 'details wrap and copy safely')
git diff --check
```

Inspect generated locale changes before marking complete. If the shared Alert
primitive changes, add and run its focused regression via the web test harness
as `components/task/session-error-layout.test.tsx` plus the rendered geometry case.

## Files likely touched

- New `apps/web/lib/session-error-details.ts` and `.test.ts`.
- New `apps/web/components/task/session-error-details.tsx` and `.test.tsx`.
- `apps/web/components/task/ensure-session-error.tsx`, `workspace-unavailable.tsx`, `chat/session-bootstrap-recovery-card.tsx`, `chat/messages/action-message-details.tsx`, `simple/components/run-error-entry.tsx` and their tests.
- `apps/web/lib/state/slices/session-runtime/workspace-restoration.ts`, task/chat locale catalogs, and new desktop/mobile session-error-recovery-ui E2E files.
- `apps/packages/ui/src/alert.tsx` only if the isolated regression proves the shared column contract defective.

## Dependencies

None. Existing session recovery/history and backend sanitization are compatibility
inputs, not code to replace.

## Risks

Legacy text may contain malformed credential syntax. Prefer omission over leakage;
keep a localized summary and safe category when no useful detail remains.

## Parallelism

`sequential`

## Inputs

Paired design: Diagnostics and localization; Layout, phone contract and accessibility.
Existing `routingerr/sanitize_test.go`, mobile picker shell, Alert primitive,
workspace restoration tests, and launch-failure recovery E2E fixtures.

## Results

Completed on 2026-09-19. See the [plan verification ledger](plan.md#verification-results)
for the 295 focused tests, desktop/mobile browser runs, localization, lint,
typecheck, document validation, screenshots, and the resolved flaky mobile run.
No runtime secret provisioning or recovery operation contract was changed.
