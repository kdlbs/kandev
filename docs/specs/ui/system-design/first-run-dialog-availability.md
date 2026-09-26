---
status: current
system: ui
requirements:
  - REQ-UI-FIRST-RUN-DIALOG-001
---

# First-run dialog availability system design

## Purpose and boundaries

`PageClient` decides whether to present the first-run dialog. The UI responsive breakpoint defines phone availability; the dialog continues to own step navigation and agent-profile edits. The executor design owns its step's content.

## Requirement mapping

| Requirement                   | Design sections                                                                  |
| ----------------------------- | -------------------------------------------------------------------------------- |
| `REQ-UI-FIRST-RUN-DIALOG-001` | [Availability and state](#availability-and-state), [Verification](#verification) |

## Availability and state

Use the existing `useResponsiveBreakpoint()` hook and its `isMobile` result, which is true below 768 CSS pixels. `PageClient` presents `OnboardingDialog` only when the existing `ONBOARDING_COMPLETED` browser-local marker is absent and `isMobile` is false. The breakpoint hook uses a synchronous external-store snapshot so initial phone rendering does not briefly show the dialog.

Phone suppression is a visibility rule, not a completion transition. Do not call `handleOnboardingComplete`, write the marker, or save agent-profile edits on a phone visit or a desktop-to-phone resize. If the viewport becomes large enough again while the marker is absent, the dialog can reopen. Existing Skip and Get Started completion paths remain unchanged on larger screens.

This preserves the current browser-local marker for the existing tour. The proposed contextual feature guides have a separate once-per-user, cross-install requirement and cannot reuse this marker as their durable state.

## Verification

- A `PageClient` component test covers phone and larger breakpoints with absent and present completion markers, plus resize transitions.
- Replace the two mobile Playwright expectations that currently open the dialog. Assert the normal page is usable, the dialog remains absent, and the completion marker stays absent through a phone visit. Resize to a larger viewport and assert that the dialog appears.
- Desktop Playwright coverage confirms the tour still opens when unfinished and remains hidden after Skip or completion.
