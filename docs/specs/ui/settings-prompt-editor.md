---
status: shipped
created: 2026-08-20
owner: Kandev
---

# Settings Prompt Editor

## Why

Prompt fields in Settings use different editors and completion rules. Users cannot reliably discover placeholders or reference saved prompts across these fields.

## What

- Kandev provides one shared prompt editor for prompt-authoring fields in Settings.
- The editor stores plain text and preserves each existing settings and save contract.
- Typing `{{` shows only the placeholders that the current prompt context supports.
- Typing `@` shows saved prompts when the runtime resolves saved-prompt references for that field.
- A selected saved prompt inserts its `@name` reference. The editor does not inline or copy the saved prompt content.
- The saved-prompt list updates when a prompt is added, edited, or removed.
- A custom prompt can reference other saved prompts. Its editor does not suggest the prompt that is currently open.
- Keyboard, pointer, and touch users can open, filter, and select completion items.
- The editor keeps completion providers scoped to their editor model. One open editor cannot show completion items from another editor.
- Each field keeps its existing dirty state, validation, reset behavior, save coordinator, and persisted text format.
- Desktop and mobile settings routes provide the same completion functions.

### Settings surfaces

| Surface | Placeholder completion | Saved-prompt completion |
|---|---|---|
| GitHub, GitLab, Jira, and Azure DevOps quick actions | Provider placeholders | Yes |
| GitHub, GitLab, Jira, Linear, Sentry, and Azure DevOps watch prompts | Provider placeholders when available | Yes |
| Workflow prompt | Workflow placeholders | Yes |
| Workflow step prompt | Step placeholders | Yes |
| Automation instructions | Trigger placeholders | Yes |
| Custom prompt content | None | Yes, excluding the open prompt |
| Utility agent prompt | Utility template variables | No |

Utility prompts run through the sessionless utility template engine. That engine does not resolve saved-prompt references.

The shared editor does not replace script, query, JSON, or description fields. Those fields have different languages and completion contracts.

## Failure modes

- If saved prompts fail to load, placeholder completion continues to work. The `@` menu shows no saved prompts.
- If a field has no configured placeholders, `{{` shows no placeholder items.
- If Monaco loads slowly, the field shows the existing localized editor loading state. The controlled draft remains unchanged.
- If two prompt editors are mounted together, each editor uses only its own placeholders and saved-prompt rules.

## Persistence guarantees

This feature adds no new persistence. Each settings surface continues to store its current plain-text value through its current API.

Selecting a completion item changes only the local draft. The existing settings save action remains the persistence boundary.

## Scenarios

- **GIVEN** a saved prompt named `review-helper`, **WHEN** a user types `@rev` in a GitHub quick action, **THEN** the editor offers `@review-helper`.
- **GIVEN** that suggestion, **WHEN** the user selects it, **THEN** the draft contains `@review-helper` and no save starts.
- **GIVEN** a GitHub quick action editor, **WHEN** a user types `{{`, **THEN** the editor offers `{{url}}` and `{{title}}`.
- **GIVEN** two open editors with different placeholder lists, **WHEN** a user types `{{` in either editor, **THEN** only that editor's placeholders appear.
- **GIVEN** an open workflow prompt, **WHEN** another route adds a saved prompt, **THEN** the next `@` list includes it.
- **GIVEN** a custom prompt named `release`, **WHEN** the user edits `release` and types `@`, **THEN** the list omits `@release` and includes other saved prompts.
- **GIVEN** a utility agent prompt, **WHEN** the user types `{{`, **THEN** utility variables appear and no saved-prompt completion is offered.
- **GIVEN** a failed saved-prompt request, **WHEN** the user types a supported placeholder prefix, **THEN** placeholder completion still works.
- **GIVEN** an unsaved prompt edit, **WHEN** the user leaves the route, **THEN** the existing settings navigation guard still handles the draft.
- **GIVEN** GitHub quick actions on a phone, **WHEN** the user selects both completion types, **THEN** no horizontal page overflow occurs.
- **GIVEN** a saved quick action that contains a placeholder and saved-prompt reference, **WHEN** the page reloads, **THEN** the editor shows the same stored text.

## Out of scope

- Rich-text formatting, Markdown preview, or Tiptap document storage.
- Changes to saved-prompt expansion, placeholder interpolation, or backend APIs.
- Prompt composers outside Settings, such as task creation and task chat.
- Completion for scripts, WIQL, JQL, JSON, descriptions, or confirmation fields.

## Implementation plan

See [`../../plans/settings-prompt-editor/plan.md`](../../plans/settings-prompt-editor/plan.md).
