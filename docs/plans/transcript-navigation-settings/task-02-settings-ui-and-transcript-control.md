---
id: "02-settings-ui-and-transcript-control"
title: "Settings UI and transcript control"
status: done
wave: 2
depends_on: ["01-portable-settings-contract"]
plan: "plan.md"
spec: "../../specs/ui/requirements/transcript-navigation-settings.md"
---

# Task 02: Settings UI and Transcript Control

## Acceptance

- The Transcript Navigation card shows four independently draftable switches with the documented
  defaults and persists only changed fields through the shared Save action.
- Hiding **Show transcript auto-scroll control** removes the per-session transcript button without
  changing the default enabled auto-scroll behavior or stored per-session choice.
- Task Actions explanatory copy accurately describes the optional controls.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/settings/anchored-prompt-bar-settings.test.tsx components/task/chat/auto-scroll-toggle-button.test.tsx
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/components/settings/anchored-prompt-bar-settings.tsx`
- `apps/web/components/settings/anchored-prompt-bar-settings.test.tsx`
- `apps/web/components/settings/general-settings.tsx`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/components/task/chat/auto-scroll-toggle-button.test.tsx`

## Dependencies

Task 01.

## Parallelism

Sequential. The UI consumes the contract and defaults introduced by Task 01.

## Inputs

- Spec: What and auto-scroll visibility scenarios.
- ADR 0046 and `docs/specs/ui/requirements/settings-manual-save.md`.
- Existing `AutoScrollToggleButton` and `useTranscriptAutoScrollEnabled` behavior.

## Risks

- Hiding the control must not reset session storage or change message-list auto-scroll selectors.
- A quick-chat status bar with no remaining right-side controls must not retain empty layout chrome.

## Output Contract

Report settings/manual-save behavior, transcript visibility behavior, files changed, exact test
results, blockers and residual risks, then mark this task `done` and update `plan.md`.

## Result

- Added the fourth manual-save switch with plain-language copy confirming that it only hides the
  per-session control; new transcripts still auto-scroll.
- Updated Task Actions copy to describe optional transcript controls.
- Made `AutoScrollToggleButton` return no control when the saved visibility preference is off,
  without changing its per-session enabled state or session-storage behavior.
- `cd apps && pnpm --filter @kandev/web test -- --run components/settings/anchored-prompt-bar-settings.test.tsx components/task/chat/auto-scroll-toggle-button.test.tsx` — passed (12 tests, 2 files).
- `cd apps/web && pnpm run typecheck` — passed.
