---
status: current
system: cli
requirements:
  - REQ-CLI-MOBILE-PASSTHROUGH-COMPOSER-001
---

# Mobile Passthrough Composer System Design

## Purpose and boundaries

The CLI system owns the user-visible prompt path for sessions backed by a raw
agent PTY. The web application implements that path with an inline composer,
but it must keep passthrough submission distinct from raw terminal input. This
design uses the UI system's shared composer overlay contract and the Tasks
system's attachment contract without redefining either one.

The composer remains inline above the passthrough status row. A drawer would
introduce another focus, draft, and viewport owner without solving the measured
touch-target defect. The existing mobile task layout already constrains the
surface with `100dvh`, safe-area padding, and one flexible terminal region.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-CLI-MOBILE-PASSTHROUGH-COMPOSER-001` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow), [Responsive behavior](#responsive-behavior), [Persistence](#persistence) |

## Components and responsibilities

- `PassthroughToolbar` owns the vertical terminal, optional composer, comments,
  and passthrough status-row composition. It applies touch geometry to the
  owned mobile and coarse-pointer tablet status controls, Chat, Comments,
  Proceed, and Send to Agent, while preserving compact desktop controls.
- `PassthroughComposerPanel` configures the shared `ChatInputContainer` for a
  passthrough session. It disables unadvertised agent commands and external
  entity references while retaining context mentions, attachments, plan
  context, and session-scoped draft behavior.
- `MobileChatInputToolbar` owns the mobile and coarse-pointer tablet
  presentation of shared composer controls. Its context, attachment, plan,
  cancel, send, and split Implement controls use a touch presentation with a
  minimum 44-by-44 CSS-pixel target. Desktop toolbar geometry does not change.
- `PassthroughTerminal` remains the raw PTY surface and the only flexible child
  in the toolbar. On mobile, it retains touch scrolling.
- The shared suggestion popup remains responsible for visual-viewport
  containment, internal result scrolling, and touch selection.

The current defect comes from fixed `h-6` and `h-7` classes in the passthrough
status row and shared composer primitives. The mobile and tablet layouts reused
those desktop-sized controls without a touch presentation, which produced
measured 24-to-28 CSS-pixel targets.

## Data and contracts

No backend, WebSocket, or persisted data contract changes.

- Composer submission continues to use `message.add` with `task_id`,
  `session_id`, a unique `client_message_id`, resolved content, optional
  attachments, and optional context-file metadata.
- The backend remains responsible for converting that message into the active
  passthrough agent's configured submit sequence. The browser must not send a
  raw Enter key as a substitute for this request.
- `hasAgentCommands={false}` remains the passthrough capability declaration.
  Literal `/` input is therefore intentional, not a missing mobile menu.
- The existing task attachment contract owns file materialization, retry,
  removal, and error behavior.

## Control flow

1. The phone user opens Chat from the passthrough status row.
2. `PassthroughToolbar` renders the inline `PassthroughComposerPanel` and moves
   focus into its TipTap editor.
3. The user enters text, selects contextual `@` results, or chooses attachments.
   These actions update the session-scoped draft and do not submit it.
4. Explicit send resolves selected comments, context files, plans, task
   mentions, and attachments into one `message.add` request.
5. After the request succeeds, the composer clears its sent state and closes.
   On failure, it stays available with the draft and error feedback intact.

Escape closes the Kandev composer while it has focus. Raw Ctrl+C, Escape, Enter,
and other terminal keys remain xterm interactions and are not added to the
composer toolbar by this capability.

## Responsive behavior

Touch presentation is selected by the existing responsive breakpoint. The
mobile and coarse-pointer tablet branches use touch-sized controls; fine-pointer
desktop branches retain compact controls. The terminal stays `min-h-0` and
flexible; the composer and status row keep their intrinsic height. This
prevents the document from becoming a second scroll container when the composer
opens.

The shared composer actions and passthrough status-row actions owned by this
change use at least 44-by-44 CSS-pixel hit areas. This includes the composer
plan, attachment, context, cancel, send, and split Implement actions, plus the
passthrough Chat, Comments, Proceed, and Send to Agent actions. Visual glyphs
can remain smaller. Integration-owned status chips, including dependency and
PR/provider chips, keep their own component contracts. A horizontal action
strip can scroll internally when its controls exceed the available width, but
it must not cause document-level horizontal overflow.

The existing visual-viewport-aware suggestion popup remains attached above the
composer. The implementation does not replace it with a drawer.

## Failure and recovery

- If `message.add` fails, the composer keeps the unsent draft and exposes the
  existing translated error. It must not clear or close as though the send
  succeeded.
- If attachment upload fails, the existing attachment item remains retryable or
  removable before send.
- If the Visual Viewport API is unavailable, the shared overlay falls back to
  layout-viewport bounds.
- A passthrough session that has no advertised ACP commands continues to accept
  literal slash text without showing an empty or misleading command menu.

## Persistence

TipTap JSON, selected context, and attachment state continue to use the
session-keyed chat draft store. Switching tasks selects another session key;
returning restores the original session's draft. This change adds no storage,
migration, or retention policy.

## Verification boundary

Focused component tests prove the mobile and desktop class contracts. The
mobile passthrough Playwright specification proves touch geometry, explicit
send, literal slash behavior, contextual suggestions, operating-system file
selection, session-scoped draft restoration, and lack of document overflow.

Automated mobile Chromium coverage is necessary but not sufficient to close the
issue. The final verification records a smoke test on iPhone Safari and Android
Chrome for focus, software-keyboard reachability, suggestion selection, file
selection, and send.

## Related specifications

- [CLI-mode task parity](../requirements/cli-mode-parity.md)
- [Composer suggestion overlays](../../ui/requirements/composer-suggestion-overlays.md)
- [Slash command composer selection](../../ui/requirements/slash-command-composer.md)
- [Task prompt attachments](../../tasks/requirements/prompt-attachments.md)
