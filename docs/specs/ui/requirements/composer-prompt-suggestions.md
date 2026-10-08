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

Kandev uses two sources:

- **Native.** Claude sessions use Claude Code's own suggestion. Claude Code
  forks the finished conversation, with the prompt cache warm, and asks the
  same model for the user's likely next message. In local measurements it
  arrived about 1.7 seconds after the turn result, at almost no token cost.
- **Utility fallback.** Agents without a native source use a built-in utility
  agent. It follows the same prediction rules and has a strict time budget.

UI owns this contract because the observable outcome is a composer presentation
and interaction. Agents keeps ownership of ACP transport and utility-agent
execution. Tasks keeps ownership of turn and session lifecycle. A suggestion
never changes the session, turn, or prompt lifecycle until the user submits it.

## Terminology

- **Prompt suggestion:** One predicted next user prompt for a session, tied to
  the agent turn that produced it.
- **Suggestion turn:** The latest completed agent turn of a session. A prompt
  suggestion is valid only for its suggestion turn.
- **Native source:** A suggestion produced by the session's own agent runtime,
  such as Claude Code's `prompt_suggestion` message.
- **Utility fallback:** A suggestion produced by the `suggest-next-prompt`
  built-in utility agent for a session without a native source.
- **Ghost text:** The dimmed, non-editable rendering of a prompt suggestion in
  an empty composer. It is not part of the draft.
- **Accept:** Copy the suggestion into the composer as editable draft text.
- **Send:** Submit the suggestion as the next prompt without editing it.

## Requirements

### REQ-UI-PROMPT-SUGGEST-001: Opt-in prompt suggestion preference

**Intent:** Let the user decide whether Kandev predicts next prompts.

**User story:** As a user, I want to turn next-prompt suggestions on or off, so
that I control their cost and visual noise.

#### Acceptance criteria

- **AC-UI-PROMPT-SUGGEST-001.1:** Settings > Task behavior shall show a "Suggest next
  prompt" switch. The switch shall be off for new users and for existing users
  who never set it.
- **AC-UI-PROMPT-SUGGEST-001.2:** When the preference is off, the composer shall
  render no ghost text and shall request no utility fallback. Sessions launched
  or resumed while it is off shall not ask their agent for native suggestions.
  The fallback preference shall have no effect while the main preference is
  off.
- **AC-UI-PROMPT-SUGGEST-001.3:** When the user saves the preference, it shall
  persist as a per-user setting and apply after a page reload and on another
  browser.
- **AC-UI-PROMPT-SUGGEST-001.4:** After the preference changes, composer
  rendering and the utility fallback shall follow the new value immediately.
  Native generation shall follow it from the session's next launch or resume.
  The setting's help text shall say so.
- **AC-UI-PROMPT-SUGGEST-001.5:** When the main preference is on, Settings >
  General shall show a second switch, "Use a utility agent when the agent has no
  native suggestions". It shall be off for new users and for users who never set
  it, and it shall be hidden while the main preference is off. It shall persist
  as a per-user setting.
- **AC-UI-PROMPT-SUGGEST-001.6:** When the fallback switch is on, an agent
  profile selector shall appear below it. It shall show and edit the profile
  binding of the `suggest-next-prompt` built-in utility agent, defaulting to the
  default utility agent profile. A change made here shall appear in Settings >
  Utility Agents, and the reverse. When neither a bound profile nor a default
  utility agent profile exists, the selector shall say that the session's own
  agent profile is used, instead of reporting an error. The selector's help
  shall sit behind the info icon used by the other settings rows. It shall
  advise a fast model and state the fallback time budget in seconds.

### REQ-UI-PROMPT-SUGGEST-002: Native suggestions from Claude sessions

**Intent:** Deliver Claude Code's own suggestion with no extra model call and
within about two seconds of the turn ending.

**User story:** As a user running Claude, I want the same next-prompt
suggestions that Claude Code shows, so that I can continue with one key.

#### Acceptance criteria

- **AC-UI-PROMPT-SUGGEST-002.1:** When the preference is on and Kandev launches
  or resumes an interactive task or Quick Chat session whose agent identifies
  itself as Claude Code during the ACP handshake, the session shall request
  native prompt suggestions. This includes custom ACP agents that run Claude
  Code. Agents that do not identify as Claude Code shall not be asked, whatever
  model they use.
- **AC-UI-PROMPT-SUGGEST-002.2:** When the agent emits a native prompt
  suggestion after a turn result, the system shall deliver it to every browser
  that shows the session. It shall be bound to that turn, and it shall survive a
  page reload until a newer turn starts.
- **AC-UI-PROMPT-SUGGEST-002.3:** When a native suggestion arrives after the
  user already sent a newer prompt to that session, the system shall discard it.
- **AC-UI-PROMPT-SUGGEST-002.4:** Office runs, automation sessions, utility
  calls, and passthrough terminal sessions shall not request native
  suggestions.
- **AC-UI-PROMPT-SUGGEST-002.5:** When a session requests native suggestions,
  the composer shall not issue a utility fallback request for that session. An
  absent native suggestion means the agent chose silence.

### REQ-UI-PROMPT-SUGGEST-003: Utility fallback for agents without a native source

**Intent:** Offer suggestions for other agents with comparable quality and a
bounded delay.

**User story:** As a user running a non-Claude agent, I want a next-prompt
suggestion too, so that the feature works with any agent.

#### Acceptance criteria

- **AC-UI-PROMPT-SUGGEST-003.1:** When the main and fallback preferences are
  on and a session without native suggestions reaches
  waiting-for-input after an agent turn in a browser that shows it, the system
  shall request one suggestion from the `suggest-next-prompt` built-in. The
  request shall include the task title and the latest exchange: the user's last
  prompt and the agent's reply. It shall start within 500 milliseconds of the
  browser observing waiting-for-input.
- **AC-UI-PROMPT-SUGGEST-003.2:** The system shall make at most one fallback
  request per suggestion turn per browser. A reload or reopen for the same turn
  shall reuse the stored result.
- **AC-UI-PROMPT-SUGGEST-003.3:** The system shall not request a fallback while
  the session is starting, busy, or needs recovery. It shall also not request
  one while a clarification or permission question is pending, or a prompt is
  queued.
- **AC-UI-PROMPT-SUGGEST-003.4:** When no usable response arrives within 15
  seconds of the request start, the system shall abandon the request and show
  no suggestion for that turn.
- **AC-UI-PROMPT-SUGGEST-003.5:** The built-in prompt shall follow Claude Code's
  prediction rules, except that it may predict a follow-up question, because
  Kandev conversations are not limited to coding:
  - predict what the user would type, not what the agent thinks they should do;
  - use 2 to 12 words in the user's language and style;
  - never offer an evaluation, agent voice, a new idea, or several sentences;
  - stay silent when the next step is not obvious, after an error, or when the
    suggestion could be unsafe.
- **AC-UI-PROMPT-SUGGEST-003.6:** The system shall discard a fallback response
  in any of these cases:
  - it is empty after removing labels and wrapping tags;
  - it is meta text such as "no suggestion" or "silence";
  - it has more than 12 words or 100 or more characters;
  - it contains several sentences, line breaks, or Markdown;
  - it is evaluative ("looks good", "thanks"), or starts in agent voice ("Let
    me", "I'll").
- **AC-UI-PROMPT-SUGGEST-003.7:** When the fallback fails, the system shall show
  no error notification, keep the normal placeholder, and keep the composer
  usable. A request that never reached the server, for example because the
  page was reloaded, shall not count as a result for the turn.
- **AC-UI-PROMPT-SUGGEST-003.8:** The `suggest-next-prompt` built-in shall
  appear in Settings > Utility Agents for new and existing installations. Its
  prompt and profile binding shall be editable.
- **AC-UI-PROMPT-SUGGEST-003.9:** The fallback shall run on the first usable
  agent profile in this order: the profile bound to `suggest-next-prompt`, the
  default utility agent profile, then the session's own agent profile. When none
  of them is usable, for example a passthrough or workspace-scoped profile, the
  system shall show no suggestion. Other utility actions shall keep their
  current resolution, which fails without a configured profile.

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

## Out of scope

- Native sources other than Claude Code. Each needs its own design when an agent
  exposes one.
- Pausing native generation when no browser shows the session. Claude Code
  offers `set_prompt_suggestions_paused`, but the ACP adapter does not expose it.
- Back-off after unused suggestions in Kandev. Claude Code already applies its
  own back-off to native suggestions.
- Multiple or cycling suggestions, word-by-word acceptance, and completion while
  typing.
- Passthrough, run-transcript, and task-creation composers.
- A runtime feature flag. The per-user opt-in preference is the control.
