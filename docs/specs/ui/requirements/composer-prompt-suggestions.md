---
status: draft
system: ui
created: 2026-10-06
owners:
  - web
---

# Composer Prompt Suggestion Requirements

## Overview

After an agent finishes a turn, the user often answers with a short, obvious
follow-up such as "yes, run the migration and the tests" or "commit this".
Claude Code predicts that reply and shows it as dimmed ghost text in the empty
prompt box. Tab fills it in for editing, and Enter sends it.

Kandev offers the same interaction in the shared chat composer for task chat and
Quick Chat. The suggestion must appear while the user is still reading the
reply, because a late suggestion is worse than none.

Suggestions are native only. The session's own agent runtime produces them, and
Kandev shows one only when its adapter establishes native support from handshake
evidence and then receives a suggestion. Claude Code forks the finished
conversation, with the prompt cache warm, and asks the same model for the
user's likely next message. In local measurements it arrived about 1.7 seconds
after the turn result, at almost no token cost. Kandev never generates a
replacement suggestion: an agent without native support, or a turn for which
the agent sent none, leaves the composer exactly as it is today.

UI owns this contract because the observable outcome is a composer presentation
and interaction. Agents owns [native capability normalization](../../agents/requirements/native-prompt-suggestion-capability.md).
Tasks keeps ownership of turn and session lifecycle. A suggestion
never changes the session, turn, or prompt lifecycle until the user submits it.

## Terminology

- **Prompt suggestion:** One predicted next user prompt for a session, tied to
  the agent turn that produced it.
- **Suggestion turn:** The latest completed agent turn of a session. A prompt
  suggestion is valid only for its suggestion turn.
- **Native support:** Provider support established by the adapter. It is separate
  from user preference and effective session enablement. Agents owns the evidence
  rules. Today the only implemented native source is Claude Code.
- **Native session:** A session with provider support that Kandev requested for
  this launch or resume. Only an effectively enabled session can show suggestions.
- **Stale suggestion:** A prompt suggestion whose suggestion turn is no longer
  the latest turn of its session.
- **Ghost text:** The dimmed, non-editable rendering of a prompt suggestion in
  an empty composer. It is not part of the draft.
- **Accept:** Copy the suggestion into the composer as editable draft text.
- **Send:** Submit the suggestion as the next prompt without editing it.

## Requirements

### REQ-UI-PROMPT-SUGGEST-001: Opt-in prompt suggestion preference

**Intent:** Let the user decide whether agents with native support produce
next-prompt suggestions.

**User story:** As a user, I want to turn next-prompt suggestions on or off, so
that I control their cost and visual noise.

#### Acceptance criteria

- **AC-UI-PROMPT-SUGGEST-001.1:** Settings > Task behavior shall show a "Suggest next
  prompt" switch. The switch shall be off for new users and for existing users
  who never set it.
- **AC-UI-PROMPT-SUGGEST-001.2:** When the preference is off, the composer shall
  render no ghost text. Sessions launched or resumed while it is off shall not
  ask their agent for native suggestions.
- **AC-UI-PROMPT-SUGGEST-001.3:** When the user saves the preference, it shall
  persist as a per-user setting and apply after a page reload and on another
  browser.
- **AC-UI-PROMPT-SUGGEST-001.4:** After the preference changes, composer
  rendering shall follow the new value immediately. Native generation shall
  follow it from the session's next launch or resume. The setting's help text
  shall say so, and shall say that only agents with native support, such as
  Claude Code, produce suggestions.

### REQ-UI-PROMPT-SUGGEST-002: Native suggestions from agents with established support

**Intent:** Deliver the agent's own suggestion with no extra model call and
within about two seconds of the turn ending.

**User story:** As a user running Claude, I want the same next-prompt
suggestions that Claude Code shows, so that I can continue with one key.

#### Acceptance criteria

- **AC-UI-PROMPT-SUGGEST-002.1:** When the preference is on and Kandev launches
  or resumes an interactive task or Quick Chat session with established native
  support, the session shall request native prompt suggestions. Agents owns the
  support evidence contract. Custom ACP bridges with sufficient evidence shall
  qualify regardless of their name. A vendor namespace or selected model alone
  shall not establish support.
- **AC-UI-PROMPT-SUGGEST-002.2:** The session shall record whether it
  negotiated native suggestions. A session that did not negotiate them shall
  never show a suggestion, even if a suggestion payload reaches it.
- **AC-UI-PROMPT-SUGGEST-002.3:** When the agent emits a native prompt
  suggestion after a turn result, the system shall deliver it to every browser
  that shows the session. It shall be bound to that turn, and it shall survive a
  page reload until a newer turn starts.
- **AC-UI-PROMPT-SUGGEST-002.4:** When a native suggestion arrives after the
  user already sent a newer prompt to that session, the system shall discard it.
- **AC-UI-PROMPT-SUGGEST-002.5:** Office runs, automation sessions, utility
  calls, and passthrough terminal sessions shall not request native
  suggestions.
- **AC-UI-PROMPT-SUGGEST-002.6:** The system shall bind each native suggestion
  to the agent turn that produced it, and shall store it only for that turn.
  When the originating turn cannot be established, for example because a newer
  prompt started before the suggestion arrived, the system shall discard the
  suggestion instead of attributing it to a later turn.

### REQ-UI-PROMPT-SUGGEST-003: No replacement suggestion

**Intent:** Keep the composer's normal behavior whenever no native suggestion
is available.

**User story:** As a user running an agent without native suggestions, I want
the composer to behave as it always has, so that Kandev never invents a reply
on my behalf.

#### Acceptance criteria

- **AC-UI-PROMPT-SUGGEST-003.1:** For a session without native support, the
  system shall not generate, request, or display a prompt suggestion. The
  composer shall show its normal placeholder, and Tab, the submit shortcut, and
  Escape shall keep their existing behavior.
- **AC-UI-PROMPT-SUGGEST-003.2:** When a session with native support ends a
  turn without a native suggestion, the system shall show none for that turn
  and shall not generate a replacement.
- **AC-UI-PROMPT-SUGGEST-003.3:** The system shall never show a stale
  suggestion. When the session has a newer turn than the suggestion's turn, the
  suggestion shall not render, including after a page reload.

### REQ-UI-PROMPT-SUGGEST-004: Show, accept, and send the suggestion

**Intent:** Present the suggestion where the user types and make using it a
single key or tap.

**User story:** As a user, I want to see the suggested reply in the prompt box,
press Tab to edit it or Enter to send it, so that obvious follow-ups take no
typing.

#### Acceptance criteria

- **AC-UI-PROMPT-SUGGEST-004.1:** When a valid prompt suggestion exists for the
  current suggestion turn and the draft is empty, the composer shall render the
  suggestion as ghost text in place of the placeholder. An accept control
  showing the Tab key shall appear next to it.
- **AC-UI-PROMPT-SUGGEST-004.2:** When ghost text is visible and no `@`, `#`, or
  `/` suggestion menu is open, Tab shall insert the suggestion as editable draft
  text, with the caret at the end and nothing sent.
- **AC-UI-PROMPT-SUGGEST-004.3:** When ghost text is visible and no suggestion
  menu is open, the configured submit shortcut (Enter or Cmd/Ctrl+Enter) shall
  send the suggestion as the next prompt through the normal submit path,
  including queueing rules. The send button shall do the same.
- **AC-UI-PROMPT-SUGGEST-004.4:** Clicking or tapping the accept control shall
  behave like Tab. On a phone viewport, the control shall be at least 44 CSS
  pixels tall and shall not need a hardware keyboard.
- **AC-UI-PROMPT-SUGGEST-004.5:** When the user types or inserts any content,
  the ghost text and accept control shall disappear. When the draft becomes
  empty again for the same suggestion turn, they shall reappear.
- **AC-UI-PROMPT-SUGGEST-004.6:** When ghost text is visible, Escape in the
  composer shall dismiss the suggestion for its turn. The surrounding dialog
  shall stay open. Whenever ghost text is not visible, Tab, the submit shortcut,
  and Escape shall keep their existing behavior.
- **AC-UI-PROMPT-SUGGEST-004.7:** When a prompt is submitted, a new agent turn
  starts, or the session leaves waiting-for-input, the suggestion shall
  disappear. It shall not reappear for a later turn.
- **AC-UI-PROMPT-SUGGEST-004.8:** Task chat and Quick Chat shall offer the same
  behavior on desktop and phone viewports.
- **AC-UI-PROMPT-SUGGEST-004.9:** The accept control's accessible name shall
  include the suggestion text. All new user-facing copy shall be localized.
- **AC-UI-PROMPT-SUGGEST-004.10:** Accepting or sending a suggestion shall use
  exactly the displayed text. Angle brackets, HTML entities, and line breaks
  shall be kept literally and never interpreted as markup.

## Out of scope

- Model-generated or utility-agent suggestions, for any agent. Kandev shows only
  what the agent's own runtime delivers.
- Native sources other than Claude Code. Each needs an adapter integration that
  supplies the generic capability and event, without provider-specific composer logic.
- Pausing native generation when no browser shows the session. Claude Code
  offers `set_prompt_suggestions_paused`, but the ACP adapter does not expose it.
- Back-off after unused suggestions in Kandev. Claude Code already applies its
  own back-off to native suggestions.
- Multiple or cycling suggestions, word-by-word acceptance, and completion while
  typing.
- Passthrough, run-transcript, and task-creation composers.
- A runtime feature flag. The per-user opt-in preference is the control.

## Implementation plans

- [Original composer delivery](../../../plans/composer-prompt-suggestions/plan.md)
- [Native capability follow-up](../../../plans/native-prompt-suggestion-capability/plan.md)
