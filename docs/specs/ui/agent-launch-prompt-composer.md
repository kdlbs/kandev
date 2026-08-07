---
status: shipped
created: 2026-08-02
owner: kandev
---

# Agent Launch Prompt Composer

## Why

Starting another agent inside an existing task is another prompt-authoring flow, but its editor currently supports fewer authoring tools than the prompt editor used to create a task. Users should not lose saved-prompt autocomplete, image and file input, prompt enhancement, or voice input just because the task already exists.

## What

- The New Agent dialog and the session handoff dialog use the same shared prompt composer as task creation, in session mode.
- Typing `@` at the start of the prompt or after whitespace opens the existing mention menu. Selecting a saved custom prompt with Enter, Tab, pointer, or touch replaces only the active mention query with the prompt content and does not launch an agent.
- The launch composer supports the shared task composer capabilities: pasted images, file selection, drag-and-drop attachments, attachment removal, prompt enhancement with retained-result recovery, voice transcription, and configured voice auto-send.
- Copying the initial prompt, choosing blank context, or generating a session summary updates that same composer without losing its authoring tools.
- The Start Agent button and the existing Ctrl/Cmd+Enter shortcut launch with the current prompt and attachments. Enter used to select an open mention result never launches the agent.
- The Start Agent action remains disabled while the prompt is empty, while context is being summarized, while launch is in progress, or when no compatible agent profile is available.
- Desktop and phone layouts expose the same capabilities and launch result. On phone, the existing mobile sessions entry point opens the dialog; its prompt menu and controls remain touch reachable without horizontal document overflow.
- Agent launch continues to use the existing session launch contract, task environment, context selection, profile compatibility rules, and guarded enhancement delivery.

## Failure modes

- If a saved prompt lookup fails or returns no matches, the typed draft remains editable and the dialog remains open.
- If an attachment is unreadable or exceeds the shared composer limits, the existing attachment feedback is shown and the invalid attachment is not submitted.
- If enhancement completes after the prompt changed or the editor is unavailable, the generated result remains available through the existing Apply or Copy recovery actions instead of overwriting the draft.
- If session launch fails, the dialog remains available with the authored prompt and attachments and shows the existing launch error toast.

## Scenarios

- **GIVEN** a task and a saved custom prompt, **WHEN** the user opens New Agent, types part of the prompt name after `@`, and selects the result, **THEN** the saved prompt content is inserted into the launch composer and no new session exists until the user explicitly starts the agent.
- **GIVEN** the saved-prompt menu is open, **WHEN** the user presses Enter on a result, **THEN** the result is inserted, the dialog stays open, and Enter does not submit the form.
- **GIVEN** a prompt with pasted images or attached files, **WHEN** the user starts the agent, **THEN** the new session receives the current prompt and the accepted attachments through the existing session launch request.
- **GIVEN** prompt enhancement is available, **WHEN** the user enhances an unchanged launch prompt, **THEN** the enhanced result is applied to the same composer; if the prompt changed meanwhile, the result is retained for Apply or Copy.
- **GIVEN** voice mode is configured, **WHEN** the user dictates in the launch composer, **THEN** the transcript is inserted at the caret and configured auto-send starts the agent only after non-empty text was inserted.
- **GIVEN** a prior task prompt or session summary, **WHEN** the user selects that context, **THEN** the resulting text appears in the shared composer and remains editable with saved-prompt, attachment, enhancement, and voice controls available.
- **GIVEN** a phone viewport, **WHEN** the user opens New Agent from the mobile session controls, inserts a saved prompt by touch, and starts the agent, **THEN** the dialog remains viewport-contained without document horizontal overflow and the new session becomes active.
- **GIVEN** a session handoff, **WHEN** the handoff dialog opens, **THEN** it exposes the same shared launch composer while preserving the existing automatic summary and target-profile behavior.

## Out of scope

- Changing saved-prompt storage, mention search, attachment limits, utility-agent enhancement, voice settings, or the session launch API.
- Changing task creation, subtask creation, task chat, Quick Chat, context selection semantics, agent profile selection, or environment reuse.
- Adding rich inline chips or external `#` entity references to the plain launch prompt.
