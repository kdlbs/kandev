---
status: active
system: ui
created: 2026-10-28
owners:
  - kandev
---

# Saved Prompt Search Requirements

## Overview

Settings users need to find a saved prompt by its name or full saved content
without opening each entry. UI owns this transient list-presentation interaction;
saved-prompt authority, permissions and delivery remain with their existing
owners, including [saved prompt delivery](../../tasks/requirements/saved-prompt-delivery.md).

## Requirements

### REQ-UI-SAVED-PROMPT-SEARCH-001: Search the loaded prompt list

**Intent:** Find an existing prompt without modifying its saved data or an
in-progress settings operation.

#### Acceptance criteria

- **AC-UI-SAVED-PROMPT-SEARCH-001.1:** Settings > Prompts shall offer an accessible, localized search input on desktop and phone for the currently loaded built-in and custom prompts.
- **AC-UI-SAVED-PROMPT-SEARCH-001.2:** A nonempty query, trimmed only at its ends, shall display entries whose saved name or raw saved content contains that literal substring, ignoring case, in their existing order. Stored names and content shall remain unchanged; references in content shall not be expanded for matching.
- **AC-UI-SAVED-PROMPT-SEARCH-001.3:** An empty or whitespace-only query shall show all loaded entries in their existing order. Clearing a query shall restore that list.
- **AC-UI-SAVED-PROMPT-SEARCH-001.4:** A nonempty query with no matches in a nonempty collection shall show the localized no-results message and leave search editable. A genuinely empty collection and loading retain their existing views.
- **AC-UI-SAVED-PROMPT-SEARCH-001.5:** Search shall be disabled during editing, creation, delete confirmation, or a prompt mutation in progress. A matched row entering edit shall remain mounted when its draft changes, and an edited row or open delete target shall remain mounted through saved-name or content refresh while its ID remains in the loaded collection. An entry removed from that collection shall not be retained. Existing save, cancel, discard, create, delete and agent-edit permission behavior shall remain available.
- **AC-UI-SAVED-PROMPT-SEARCH-001.6:** Changing the query shall not dirty settings, initiate a prompt mutation, or change saved-prompt delivery, permissions, loading, or error behavior.
- **AC-UI-SAVED-PROMPT-SEARCH-001.7:** The phone or coarse-pointer input shall have an actual hit area at least 44 pixels high, remain contained in its settings surface without horizontal page overflow, and support native text entry and clearing. Fine-pointer desktop retains ordinary settings input density.

## Out of scope

- Remote search, regular expressions, reference expansion, query persistence, counts, new settings controls or keyboard shortcuts.
- Changes to prompt storage, mutation, authorization, delivery, or asynchronous lifecycle behavior.
