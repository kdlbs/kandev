---
status: active
system: cli
created: 2026-09-02
owners:
  - web
---

# Mobile Passthrough Composer Requirements

## Overview

CLI passthrough sessions expose an agent terminal and a Kandev composer in the
same task surface. Phone users and coarse-pointer tablet users must be able to
compose and send a structured follow-up without losing terminal output, drafts,
attachments, or contextual references. The CLI system owns this contract
because submission must preserve the raw PTY delivery semantics of a
passthrough session.

## Terminology

- **Passthrough composer:** The Kandev-controlled prompt editor displayed with
  an agent's passthrough terminal.
- **Explicit send:** A composer action that records one user message and
  delivers its resolved content through the passthrough session's configured
  submit behavior.
- **Raw terminal input:** Keystrokes sent directly to the agent PTY without
  creating a Kandev user message.

## Requirements

### REQ-CLI-MOBILE-PASSTHROUGH-COMPOSER-001: Touch-safe passthrough composition

**Intent:** Let a phone user prepare and send a structured passthrough prompt
without losing access to the running terminal or current draft.

**User story:** As a phone user in a CLI passthrough session, I want to compose
and send a follow-up by touch, so that I can continue the session without a
desktop keyboard.

#### Acceptance criteria

- **AC-CLI-MOBILE-PASSTHROUGH-COMPOSER-001.1:** When a user opens a CLI
  passthrough session on a phone or coarse-pointer tablet, the system shall
  keep the terminal, owned composer and status controls, and open composer
  inside the visible application surface without horizontal document overflow.
- **AC-CLI-MOBILE-PASSTHROUGH-COMPOSER-001.2:** When a shared composer action or
  passthrough status-row action owned by this change is shown on a phone or
  coarse-pointer tablet, its touch target shall be at least 44 CSS pixels high
  and 44 CSS pixels wide. The owned controls are the composer plan, attachment,
  context, cancel, send, and split Implement actions, plus the passthrough
  Chat, Comments, Proceed, and Send to Agent actions. Integration-owned status
  chips, such as dependency and PR/provider chips, retain their own component
  contracts.
- **AC-CLI-MOBILE-PASSTHROUGH-COMPOSER-001.3:** When the user types content and
  activates explicit send, the system shall create one user message and deliver
  the resolved prompt once through the active passthrough session's configured
  submit behavior.
- **AC-CLI-MOBILE-PASSTHROUGH-COMPOSER-001.4:** When the user types `@` or uses
  the context control, the system shall provide the existing prompt, plan,
  task, and workspace-file references. A selected suggestion shall remain in
  the draft until explicit send, and the suggestion overlay shall follow the
  shared [composer overlay requirements](../../ui/requirements/composer-suggestion-overlays.md).
- **AC-CLI-MOBILE-PASSTHROUGH-COMPOSER-001.5:** When the user selects the
  attachment control on a phone, the system shall open the operating system's
  file picker and preserve the existing attachment upload, retry, removal, and
  error outcomes before explicit send.
- **AC-CLI-MOBILE-PASSTHROUGH-COMPOSER-001.6:** When the user types `/` in a
  passthrough composer, the system shall keep it as literal prompt text and
  shall not show ACP slash-command suggestions that the session did not
  advertise.
- **AC-CLI-MOBILE-PASSTHROUGH-COMPOSER-001.7:** When the user leaves a task with
  an unsent passthrough draft and later returns to the same session, the system
  shall restore its text, selected context, and ready attachments without
  exposing that draft in another session.
- **AC-CLI-MOBILE-PASSTHROUGH-COMPOSER-001.8:** When the software keyboard or
  mobile browser chrome changes the visual viewport, the system shall keep the
  composer and active suggestion overlay reachable while the terminal remains
  the only flexible, internally scrollable content region.

## Out of scope

- Advertising or synthesizing ACP slash commands for passthrough sessions.
- Treating explicit composer send as a raw Enter keystroke.
- Adding one-tap Ctrl+C, Escape, Enter, or other raw PTY keys to the agent
  passthrough surface. That terminal-input contract requires a separate issue.
- Replacing the contextual composer popup with a drawer or full-screen picker.
- The companion mobile remote-access documentation tracked by GitHub issue
  `#2807`.
