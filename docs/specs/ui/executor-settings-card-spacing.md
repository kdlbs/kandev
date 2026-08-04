---
status: shipped
created: 2026-08-03
owner: kandev
---

# Executor settings card spacing

## Why

Executor profile pages show several independently editable settings cards in one long form. On the modern profile edit and create routes, those cards touch edge-to-edge, which makes the form read as one dense surface and makes section boundaries harder to scan on desktop and phone screens.

## What

- Modern executor profile edit pages at `/settings/executors/:profileId` keep a consistent vertical gap between every visible settings card, including cards that appear only for a particular executor type.
- Modern executor profile create pages at `/settings/executors/new/:type` use the same vertical card spacing for every supported executor type.
- The spacing is the existing settings rhythm of `2rem` between adjacent cards and remains present at the configured phone viewport as well as desktop widths.
- Card-internal padding, form fields, manual-save behavior, disabled-fieldset behavior, routing, and executor-specific content remain unchanged.
- The profile form remains a single vertically scrolling surface and does not introduce document-level horizontal overflow on a phone viewport.

## Scenarios

- **GIVEN** a saved worktree executor profile, **WHEN** the user opens its modern profile editor, **THEN** the Profile Details, Environment Variables, Prepare Script, and any subsequent cards have a visible `2rem` vertical separation from the preceding card.
- **GIVEN** a new profile form for a supported executor type, **WHEN** the user opens the form, **THEN** every visible settings card is separated by the same vertical rhythm.
- **GIVEN** a modern executor profile editor or create form on a phone-sized viewport, **WHEN** the user scrolls through the form, **THEN** adjacent cards retain the separation, remain within the viewport, and the document has no horizontal overflow.
- **GIVEN** an executor profile form is disabled while it is loading or saving, **WHEN** the spacing layout is rendered, **THEN** the fieldset remains disabled without changing the card spacing or form state.

## Out of scope

- Changing the shared card primitive's internal padding or spacing for unrelated settings pages.
- Redesigning executor settings navigation, card content, form controls, save coordination, or executor-specific behavior.
- Adding a new mobile navigation surface or changing desktop preferences on phone viewports.
