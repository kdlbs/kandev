---
id: "02-clarify-recovery-progress"
title: "Clarify recovery progress"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.14
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.15
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.16
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.17
system_design:
  - ../../specs/agents/system-design/session-recovery-guidance.md
---

# Task 02: Clarify recovery progress

## Summary

Give the chat input recovery surface a readable status above its disabled recovery buttons.
Use the shared grid spinner and existing operation labels on desktop and phone.

## In scope

- The shared `RecoveryBusyStatus` placement and presentation in `RecoveryActions`.
- Resume, checking, and the existing operation-specific pending labels.
- Component coverage and the existing native Resume projection scenarios.

## Out of scope

Backend operations, retry policy, phase tracking, warning copy, new translations, workspace projection, and recovery ownership.

## Acceptance

1. Pending status appears before the recovery buttons, with a decorative grid spinner aligned to the readable operation label.
2. The row announces progress once. Equivalent actions stay disabled, button names stay stable, and the existing Stop action stays available.
3. Desktop and phone retain their existing action composition, recovery outcomes, interrupted-prompt warning, saved conversation, and draft.

## ASCII UI preview

UI-02: Resume pending in the chat input recovery surface, desktop.

```text
Connection interrupted. The previous prompt's outcome is unknown.
Resume checks the saved conversation without sending it again.

[grid] Resuming session
[Retry connection: disabled] [Resume: disabled] [Stop]
```

UI-03: The same pending operation on a phone.

```text
Connection interrupted.
The previous prompt's outcome is unknown.
Resume checks the saved conversation
without sending it again.

[grid] Resuming session
[   Retry connection: disabled   ]
[       Resume: disabled         ]
[             Stop               ]
```

Status placement, centered alignment, and Stop availability are required. Wording and line breaks are illustrative.
Reuse the existing localized explanation and `task:resumingSession` status label.
The status remains inline in the composer recovery region, with existing scroll ownership and safe-area handling.
Phone buttons retain at least 44-pixel targets. Desktop keeps its wrapping row and 28-pixel fine-pointer controls.
These views cover requirement 006 criteria .4, .14, .15, .16, and .17.
See the [combined previews](plan.md#ascii-ui-preview).

## Implementation

1. Add component tests for readable pending labels, status-before-actions order, one progress announcement, disabled duplicate actions, available Stop, and idle height.
2. Cover each `RecoveryActionKind` and checking without an action. Keep button accessible names stable across idle and pending states.
3. Extend the existing `Resume stays busy during recovery` scenarios in `e2e/helpers/native-resume-projection.ts`.
4. Use its outgoing-request barrier to inspect the pending state before recovery can complete.
5. Assert the status above the controls, spinner/label center difference of at most 1 CSS pixel, and enabled Stop.
6. Use the existing geometry helper for desktop, phone, and the 767/768-pixel boundary. Check computed status styles across those widths.
7. Run the new assertions against unchanged production code and record the expected RED.
8. Move `RecoveryBusyStatus` before `RecoveryChoiceButtons`, after existing disclosures.
9. Replace `IconLoader2` with `GridSpinner` inside an `aria-hidden` flex wrapper.
10. Use `text-sm font-medium text-foreground` for progress and the primary color for the spinner.
11. Reuse `task:resumingSession` for the existing Resume label branch. Retain the other operation labels and checking fallback.
12. Retain the mounted status region with no visible idle height and no duplicate spinner announcement.
13. Update existing label expectations and run the required checks.

Use `data-testid="recovery-progress-status"` on the shared live region so tests can distinguish progress from independent failure notices.
Keep Stop in its current trailing action slot. Do not replace the warning header or create a second status owner.
Release the held request in all test exit paths. Keep success and timeout/retry scenarios and their real recovery assertions.

## Verification

The workspace already has dependencies. For a fresh checkout, run `pnpm install --frozen-lockfile` from `apps/` first.
Run these commands from the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/task/recovery-actions.test.tsx components/task/chat/session-recovery-card.test.tsx components/task/chat/session-recovery-color.test.tsx components/task/chat/session-stopped-banner.test.tsx components/task/chat/automatic-recovery-presentation.test.tsx)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/task/recovery-actions.tsx components/task/recovery-actions.test.tsx components/task/chat/session-recovery-card.test.tsx components/task/chat/session-recovery-color.test.tsx components/task/chat/session-stopped-banner.test.tsx e2e/helpers/native-resume-projection.ts e2e/helpers/delivery-recovery-controls.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/native-resume-projection.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-native-resume-projection.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/conversation-loading-alignment
```

Run desktop and phone E2E sequentially. Managed runs rebuild production assets and use isolated backends.
Inspect the captured pending-state screenshots against UI-02 and UI-03.
Run the documentation coverage preflight in Task 01 with both work orders, both requirement files, and both referenced system designs.
Record the expected RED and final GREEN in Results.

## Files likely touched

- `apps/web/components/task/recovery-actions.tsx`
- New `apps/web/components/task/recovery-actions.test.tsx`
- `apps/web/components/task/chat/session-recovery-card.test.tsx`
- `apps/web/components/task/chat/session-recovery-color.test.tsx`
- `apps/web/components/task/chat/session-stopped-banner.test.tsx`
- `apps/web/e2e/helpers/native-resume-projection.ts`
- `apps/web/e2e/helpers/delivery-recovery-controls.ts`
- This work order and `plan.md` for execution status and results.

The existing desktop and phone spec entrypoints call the shared helper and need no additional test registration.

## Dependencies

None. Execute after Task 01 by default; the scopes do not depend on one another.

## Risks

- The spinner has its own status role. Its wrapper must hide that decorative role from assistive technology.
- Shared status markup serves multiple recovery operations. Their labels and request admission must remain distinct.
- Backend events can finish recovery before a held response arrives. The existing outgoing-request barrier avoids that race.
- Failure and Stop notices can coexist with progress. Keep progress assertions scoped to the shared row.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/session-recovery-failures.md), criterion 006.4 and compatibility criteria .14-.17.
- [Pending feedback design](../../specs/agents/system-design/session-recovery-guidance.md#pending-feedback).
- [Active recovery ownership](../../decisions/2026-09-20-active-session-recovery-owner.md).
- `RecoveryActions`, `RecoveryBusyStatus`, and the existing pending-label mapping.
- `e2e/helpers/native-resume-projection.ts`, its request gate, and `native-resume-notice.ts`'s real recovery checks.

## Results

The new component checks and desktop/phone pending-state checks failed against the original renderer because the shared progress row had no dedicated test marker or status-before-actions presentation.

The shared recovery status now appears above its buttons and uses the grid spinner, a polite live region, and the existing `task:resumingSession` label. The spinner is decorative and hidden from assistive technology. The status stays mounted without idle height. Resume and Retry connection remain disabled while busy; Stop stays available. The 768-pixel phone boundary permits the existing wrapped action layout while retaining target-size, overflow, status-alignment, and computed-style checks.

Passed:

- `pnpm exec vitest run components/task/recovery-actions.test.tsx components/task/chat/session-recovery-card.test.tsx components/task/chat/session-recovery-color.test.tsx components/task/chat/session-stopped-banner.test.tsx components/task/chat/automatic-recovery-presentation.test.tsx` (5 files, 69 tests).
- Targeted oxlint for the recovery components/tests and native Resume and delivery geometry helpers.
- `pnpm run typecheck`.
- `pnpm run i18n:ratchet` (0 added or modified hardcoded-copy violations; guard allowlist intact).
- Desktop `pnpm e2e:run --project chromium tests/session/native-resume-projection.spec.ts` (2 passed).
- Phone `pnpm e2e:run --project mobile-chrome tests/session/mobile-native-resume-projection.spec.ts` (2 passed).
- PR CI fixup: desktop and phone contention checks now assert the shared "Resuming session" status. The pending Resume test keeps its request gate armed through a 300 ms negative assertion window before checking that disabled duplicate actions sent no second request. The focused desktop and phone E2E slices passed (4 each).
- Both managed E2E runs rebuilt the Vite assets successfully. Pending-state screenshots were inspected against UI-02 and UI-03.
- Documentation validators and coverage preflight, plus `git diff --check`.
