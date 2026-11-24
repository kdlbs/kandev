---
created: 2026-11-17
status: done
requirements:
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
system_design:
  - ../../specs/platform/system-design/session-subscription-recovery.md
  - ../../specs/agents/system-design/session-recovery-guidance.md
legacy_specs: []
---

# Implementation Plan: Conversation loading and recovery status

## Overview

Align the conversation spinner with its loading and retrying labels.
Also make pending recovery visible above the chat input's recovery buttons.
Task 01 corrects history loading alignment. Task 02 improves the shared recovery status.
They run sequentially by default and have no implementation dependency.

Platform owns the history loading contract. Agents owns active recovery presentation and its operation-specific status.
Both changes extend existing requirement/design pairs, without creating a separate UI contract.
The user requests both improvements in the same task. The proposed recovery status reuses existing labels, controls, and operation state.

## Evidence and root cause

`SessionHistoryFeedback` already centers its children with `flex items-center`.
Its spinner sits inside an `aria-hidden` span that creates an inline formatting context.
The nested inline grid contributes baseline space, so the wrapper's center differs from the spinner's center.
The outer row centers the wrapper instead of the spinner.

A temporary standards-mode Chromium reproduction used the source spinner CSS and matching row markup.
At 1440-pixel and 393-pixel widths, the spinner center sat 6.640625 CSS pixels below the label center.
The reproduction used a 16-pixel font and 24-pixel line height.
Changing only the wrapper to `display: flex` reduced both offsets to zero.
Rendered screenshots also showed the corrected alignment.
These measurements describe the isolated reproduction, not a full application E2E run.

The second loading row in `MessageListStatus` places `GridSpinner` directly in its flex row.
It does not contain the wrapper that causes this defect.

The second screenshot shows the chat input's uncertain-delivery recovery surface during Resume.
`RecoveryBusyStatus` in `components/task/recovery-actions.tsx` uses a small muted circular spinner row after the buttons.
The loading feedback therefore follows disabled controls and has less emphasis than the surrounding content.
The existing `busyAction` identifies the operation but supplies no finer progress phases.
The same shared renderer serves the ordinary recovery card and stopped-session adapter.

## Scope

### In scope

- The loading and retrying row in `SessionHistoryFeedback`.
- Desktop and phone regression checks against the real chat surface.
- A readable grid-spinner status before the shared recovery buttons, including Resume and connection retry.
- Existing localized status and recovery semantics.

### Out of scope

- Global spinner styling, animation changes, or manual pixel offsets.
- Changes to message fetching, retry timing, conversation state, or navigation.
- New copy, public documentation, or unrelated loading surfaces.
- New recovery operations, automatic retries, progress percentages, or backend phase tracking.

## Technical approach

Add `className="flex"` to the existing `aria-hidden` spinner wrapper in `session-entry-feedback.tsx`.
Keep the wrapper so assistive technology receives the existing single status announcement.
The current outer row already supplies vertical centering and an 8-pixel gap.
This local correction needs no new component, shared CSS rule, dependency, or state.

Add one regression scenario to each existing session-entry desktop and phone suite.
Reuse `createSettledHistoryTask` and `routeSessionEntryRecovery`.
Drop the first session-scoped `message.list` response and observe the loading row.
After the proxy records that drop, hold the next history response and observe the automatic retry row.
Compare the actual spinner and label bounding-box centers in both states.
Release the held response and check that the persisted conversation appears.
Use proxy counters and visible state as causal waits, without fixed sleeps.

For Task 02, move `RecoveryBusyStatus` before `RecoveryChoiceButtons` in `recovery-actions.tsx`.
Replace its circular spinner with the existing `GridSpinner`, hidden from assistive technology inside a flex wrapper.
Use body-sized foreground text and the existing `task:resumingSession` label for Resume.
Other actions retain their existing operation-specific labels. Keep the live region mounted without visible height while idle.
Keep the warning explanation, disabled duplicate actions, button names, and available Stop behavior.
No locale values change because every label already exists in the shipped catalogs.

Extend `e2e/helpers/native-resume-projection.ts`, which already holds outgoing recovery requests and tests success plus timeout/retry.
Use its existing request barrier to inspect status order, spinner alignment, operation label, disabled Resume, and enabled Stop.
Release the same request and retain the existing saved-conversation and no-replay evidence.
Do not substitute a held response: backend events can complete recovery before that response reaches the browser.

## ASCII UI preview

UI-01: Open an existing task, loading or retrying. Desktop and phone share this row.

```text
Before:             Loading conversation...
            [grid]

After:      [grid]   Loading conversation...
Retrying:   [grid]   Taking longer than usual. Retrying...
```

The required structure puts the grid before the label with a shared vertical center.
The text remains localized. The exact text length and drawing spacing are illustrative.
The nearest phone exemplar is the dedicated chat composition in `components/task/task-layout.tsx`.
The short status stays inline inside that chat surface. It has no action or separate scroll region.
The transcript remains the scroll owner. Existing composer and safe-area behavior stay in place.
Phone text can wrap within the available width, while the spinner aligns with the label box's vertical center.
This preview maps to AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.12 and the rendered tests below.

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

Status placement before the buttons, vertical alignment, and the available Stop action are required.
Warning text, exact line breaks, and drawing spacing are illustrative. Use the existing translated explanations.
The nearest phone exemplars are the inline composer recovery region and `RecoveryActions`' stacked buttons.
This is short inline feedback inside the existing recovery surface, with no new overlay, navigation, or scroll owner.
Phone actions retain at least 44-pixel targets. Fine-pointer desktop actions retain their existing 28-pixel sizing.
UI-02 and UI-03 cover requirement 006 criteria .4, .14, .15, .16, and .17.

## Tests

Run the existing `session-entry-feedback.test.tsx` and `message-list-shared.test.tsx` suites.
They protect status announcements and loading-state selection during the markup correction.
Do not add a unit assertion that only repeats the new CSS class.
Browser geometry supplies the alignment regression.

Task 02 adds `components/task/recovery-actions.test.tsx` for operation labels, announcement ownership, status order, and disabled duplicate actions.
It also covers the available trailing Stop action and idle status behavior.
Update existing recovery card and stopped-banner expectations for the more specific Resume label.
Keep automatic checking distinct from an actual Resume attempt, and preserve the original warning semantics.

## E2E tests

Add `centers conversation progress during loading and retrying` to both files:

- `apps/web/e2e/tests/session/session-entry-recovery.spec.ts`, project `chromium`.
- `apps/web/e2e/tests/session/mobile-session-entry-recovery.spec.ts`, project `mobile-chrome`.

Both scenarios cover AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.12.
Measure the actual `.spinner-grid` and adjacent label, rather than the outer wrapper.
Require a vertical center difference of at most 1 CSS pixel for loading and retrying.
Check phone horizontal overflow and the final appearance of the persisted conversation.
Keep existing recovery scenarios and touch-target checks intact.

Task 02 extends the existing native Resume projection scenarios through their shared helper:

- `apps/web/e2e/tests/session/native-resume-projection.spec.ts`, project `chromium`.
- `apps/web/e2e/tests/session/mobile-native-resume-projection.spec.ts`, project `mobile-chrome`.

Both suites retain the `Resume stays busy during recovery` success and timeout/retry variants.
Check the status above the controls, actual spinner/label centers, one announced progress label, and enabled Stop while Resume stays disabled.
Check the phone surface and the 767/768-pixel action-layout boundary through the existing geometry helper.
Retain real recovery, failure feedback, draft, saved-conversation, and no-replay assertions after the barrier releases.
These checks cover requirement 006 criteria .4, .14, .15, .16, and .17.

## Work orders

- [x] [Task 01: Align conversation progress](task-01-align-conversation-progress.md)
- [x] [Task 02: Clarify recovery progress](task-02-clarify-recovery-progress.md)

Execution is sequential. This package does not authorize delegation.

## Verification results

Design evidence: the isolated browser reproduction showed a 6.640625-pixel offset before the temporary correction and zero after it.
The same result held at desktop and phone widths. The temporary reproduction did not modify application source or permanent tests.

Combined design-package checks passed:

- `python3 scripts/list-docs.py validate`: 455 decisions and 2030 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Documentation coverage preflight: `covered`, with no errors, for both work orders, both contract pairs, and both planned component paths.
- `git diff --check -- docs/specs docs/plans/conversation-loading-alignment`: passed.
- `git status --short -- docs/plans/conversation-loading-alignment`: the plan and both work orders are present and uncommitted.

The named browser session is closed, and temporary reproduction files are removed.
At the design handoff, implementation had not started. Implementation results follow.

Task 01 implementation results:

- The desktop and phone rendered regressions both measured a 4.484375-pixel spinner/label center offset before the markup fix.
- After the wrapper became flex, loading and retrying alignment passed in both viewports. The final full desktop and phone recovery specs passed (5 and 3 tests).
- Focused history unit tests, lint, typecheck, specification validators, documentation coverage, and `git diff --check` passed.

Task 02 implementation results:

- Before the renderer update, the new component checks and desktop/phone pending-state checks failed because the shared progress row had no dedicated test marker or status-before-actions presentation.
- `RecoveryBusyStatus` now appears before the buttons, uses the grid spinner and existing Resume label, and retains one polite status region. The decorative spinner remains hidden from assistive technology. Resume and retry stay disabled while Stop remains available.
- The 768-pixel mobile boundary allows the existing wrapped action layout while still checking touch target size, horizontal overflow, status alignment, and computed styles.
- The desktop and phone pending-state screenshots were inspected against UI-02 and UI-03.

Passed:

- `pnpm exec vitest run components/task/recovery-actions.test.tsx components/task/chat/session-recovery-card.test.tsx components/task/chat/session-recovery-color.test.tsx components/task/chat/session-stopped-banner.test.tsx components/task/chat/automatic-recovery-presentation.test.tsx` (69 tests).
- Targeted oxlint for recovery components/tests and the native Resume and delivery geometry helpers.
- `pnpm run typecheck`.
- `pnpm run i18n:ratchet` (0 added or modified hardcoded-copy violations; guard allowlist intact).
- Desktop native Resume projection E2E (2 tests) and phone native Resume projection E2E (2 tests). Each managed run rebuilt the Vite assets successfully.
- Documentation validators, the Task 01 coverage preflight, and `git diff --check`.

## Risks

- Font metrics can change the original offset. Real application E2E checks must measure the loaded application font.
- A test that measures only the wrapper can pass while the spinner remains misaligned.
- Removing `aria-hidden` can duplicate progress announcements. The correction must retain it.
- A shared status change affects several recovery operations. Preserve their distinct labels and admission rules.
- Response-only interception cannot reliably hold native Resume before authoritative recovery events arrive. Use the existing outgoing-request gate.

## Related packages

[Session entry recovery](../session-entry-recovery/plan.md) owns the existing feedback and browser fixtures.
[Silent transcript refresh](../silent-transcript-refresh/plan.md) owns suppression of progress during background refresh.
This repair does not reopen either package's completed scope or reuse their historical test results.

[Session error recovery UI](../session-error-recovery-ui/plan.md) and
[blocked-session recovery guidance](../blocked-session-recovery-guidance/plan.md) own existing composer placement and recovery eligibility.
This status change retains their contract and does not claim to resolve the guidance package's independent backend dependency.
