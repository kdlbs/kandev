---
status: current
system: ui
requirements:
  - REQ-UI-QUICK-CHAT-ELEVATION-001
---

# Quick Chat and terminal elevation System Design

## Purpose and boundaries

The UI system owns the visual elevation contract for the shared Quick Chat
surface. `QuickChatModal` presents ordinary conversations, configuration
conversations, and Quick Terminal tabs through one responsive Radix dialog.

Conversation lifecycle, terminal lifecycle, workspace scoping, and persisted
tab order remain owned by their existing systems. The separate new-Quick-Chat
picker is not part of this design.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-QUICK-CHAT-ELEVATION-001` | [Surface composition](#surface-composition), [Responsive behavior](#responsive-behavior), [Verification](#verification) |

## Components and responsibilities

- [`QuickChatModal`](../../../../apps/web/components/quick-chat/quick-chat-modal.tsx)
  owns the shared dialog composition and passes the local backdrop treatment
  to `DialogContent`.
- [`DialogContent`](../../../../apps/packages/ui/src/dialog.tsx) owns the Radix
  portal, overlay placement, focus management, dismissal, and panel stacking.
  Its global overlay default is unchanged.
- `QuickChatContent` and the existing tab/content components keep ownership of
  conversation and terminal rendering. They do not receive backdrop state.
- The existing mobile tab-strip close control remains the explicit close path
  for the phone composition.

## Surface composition

`QuickChatModal` continues to render the shared `DialogContent` with its
existing responsive geometry and `shadow-2xl` panel elevation. It supplies a
surface-local responsive overlay class that keeps the existing lighter phone
layer and combines a stronger dark layer with a light background blur from the
`sm` breakpoint upward, such as
`bg-black/20 sm:bg-black/40 sm:backdrop-blur-sm`.

The class is applied only through this `DialogContent` instance. The shared
`DialogOverlay` default and unrelated dialogs remain unchanged. The overlay
continues to cover the page and block interaction below the dialog through the
existing Radix modal behavior.

## Control flow

1. A Quick Chat or Quick Terminal launcher updates the existing Quick Chat
   state to open the shared dialog.
2. Radix mounts the overlay and dialog in the existing portal. The overlay
   darkens and lightly blurs the page, while the panel remains above it.
3. Tab selection, terminal input, conversation input, and all existing close
   handlers continue to operate inside the unchanged panel content.
4. Escape or an explicit close action changes the existing open state to
   closed. Radix removes the overlay and content and restores focus through
   the existing dialog lifecycle.

## Responsive behavior

At tablet and desktop widths, the overlay is visible around the centered,
resizable panel. The backdrop treatment does not change panel dimensions,
position, content scroll ownership, or pointer and keyboard behavior.

At phone widths, the panel remains `h-dvh`, `w-screen`, and full-screen with
its existing safe-area padding. The panel covers the backdrop, so the contract
checks phone usability, the explicit close control, viewport containment, and
zero document-level horizontal overflow rather than expecting visible page
dimming.

The nearest shipped mobile exemplar is the existing Quick Chat mobile entry
flow in
[`mobile-quick-chat-entry.spec.ts`](../../../../apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts),
which proves the full-screen dialog, touch close control, and overflow
behavior. This styling change does not introduce a new mobile composition or
touch interaction.

## Failure and recovery

If a browser does not support backdrop filters, the dark semitransparent layer
still provides the required separation. No fallback changes dialog state,
focus return, Escape handling, or page interaction blocking.

## State and persistence

No new state, API, persistence, or migration is introduced. The overlay exists
only while the existing Quick Chat dialog is open.

## Security and accessibility

The existing Radix modal overlay remains the interaction barrier. The change
does not add an independently interactive element, alter focus trapping, or
change accessible names and close controls.

## Verification

- The desktop Quick Chat E2E scenario opens the existing setup flow and checks
  that the portaled overlay has a non-transparent background and non-default
  backdrop filter, that the panel shadow remains, and that closing removes
  both surfaces.
- The existing mobile Quick Chat E2E scenarios remain the evidence for the
  phone entry point, full-screen close control, and document overflow.
- The focused component test suite remains unchanged because the behavior is
  owned by the real Radix rendering path and is verified through Playwright.

## Related decisions

None. This is a local styling change within the existing dialog contract.
