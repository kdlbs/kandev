---
status: active
system: ui
created: 2026-08-27
owners:
  - web
---

# Composer Mention Recency Requirements

## Overview

Chat composers show tasks, saved prompts, files, and the Plan action in one
`@` suggestion menu. The current menu ranks matches by text only. Users who
mention the same items repeatedly must find them again for each message.

The UI system owns this behavior because the selection history is local
presentation state. The task, settings, and workspace-file systems continue to
own the available candidates.

## Terminology

- **Mention candidate:** A task, saved prompt, or file that the current chat
  composer can show in its `@` suggestion menu.
- **Recent selection:** A mention candidate that the user selected from a chat
  composer on the current device.
- **Baseline rank:** The existing text-match score and stable source order.

## Requirements

### REQ-UI-COMPOSER-MENTION-RECENCY-001: Rank chat mentions by recent selection

**Intent:** Make repeated mentions faster without changing candidate sources or
the menu interaction.

**User story:** As a user, I want recent task, prompt, and file selections first,
so that I can reuse them with less navigation.

#### Acceptance criteria

- **AC-UI-COMPOSER-MENTION-RECENCY-001.1:** When the `@` menu contains recent
  and unselected mention candidates, the menu shall show recent candidates
  before unselected candidates.
- **AC-UI-COMPOSER-MENTION-RECENCY-001.2:** When two recent candidates are
  available, the menu shall show the most recently selected candidate first.
  Text-match quality shall rank candidates only after selection recency.
- **AC-UI-COMPOSER-MENTION-RECENCY-001.3:** When two unselected candidates have
  different text-match quality, the menu shall preserve the existing text
  ranking. Equal candidates shall preserve the existing stable source order.
- **AC-UI-COMPOSER-MENTION-RECENCY-001.4:** When the user selects a task, prompt,
  or file by pointer, touch, Enter, or Tab, it shall become the newest recent
  selection. Selecting it again shall move it to the front.
- **AC-UI-COMPOSER-MENTION-RECENCY-001.5:** When recency changes other results,
  the Plan action shall keep the position that the baseline rank assigned for
  the current query. The Plan action shall not enter selection history.
- **AC-UI-COMPOSER-MENTION-RECENCY-001.6:** When the user reloads Kandev on the
  same device, the bounded selection history shall continue to rank chat
  suggestions. A missing, malformed, or unavailable history shall restore the
  baseline rank without blocking selection.
- **AC-UI-COMPOSER-MENTION-RECENCY-001.7:** Task and saved-prompt identities
  shall use their stable IDs. File identities shall include the workspace, so
  equal paths in different workspaces do not share recency.
- **AC-UI-COMPOSER-MENTION-RECENCY-001.8:** Task chat, Quick Chat, and
  passthrough chat shall use the same ranking on desktop and phone. The current
  popup, keyboard navigation, focus, insertion, and touch behavior shall not
  change.

## Out of scope

- Changing task, saved-prompt, or workspace-file search sources and limits.
- Showing a recent file that the current file search did not return.
- Applying recency to task creation, agent launch, `#` references, or `/`
  commands.
- Server storage, user sync, device sync, frequency scoring, or time decay.
- Adding a recent label, group, setting, clear action, or other menu copy.
