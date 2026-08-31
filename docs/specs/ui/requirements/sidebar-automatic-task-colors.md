---
status: active
system: ui
created: 2026-08-30
owners:
  - kandev
---

# Sidebar Automatic Task Colors Requirements

## Overview

The sidebar can derive a personal task color from ordered rules. The rules use task data but do not change shared task records.

The UI system owns this presentation contract. The task, workflow, executor, and repository systems continue to own the source data.

## Terminology

- **Automatic color rule:** A personal condition that supplies a task color.
- **Effective color:** The first matching automatic color. If no rule matches, it is the manual color.
- **Fixed rule color:** A color token stored as part of an automatic color rule.
- **Manual color:** A device-local color that a user assigns directly to one task.
- **Repository target:** A stable reference to a workspace, local, built-in remote, or plugin repository.

## Requirements

### REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001: Compact sidebar view editor

**Intent:** Reduce the editor height without hiding the current sort, group, task-row, or automatic-color state.

#### Acceptance criteria

- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.1:** The editor shall show Sort and Group by as collapsed summary rows before Task row.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.2:** Each summary shall show the current value. The Sort summary shall also show its direction.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.3:** Expanding or collapsing a summary shall not create or change a saved-view draft.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.4:** The editor shall show Automatic colors after Task row. It shall identify the setting as personal and global across sidebar views and workspaces.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.5:** The Automatic colors summary shall show whether automation is off or how many rules are enabled.

### REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002: Ordered personal color rules

**Intent:** Let a user recognize task categories without changing colors for other users.

**User story:** As a user, I want task colors to follow task data, so that the sidebar stays organized as tasks change.

#### Acceptance criteria

- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.1:** A user shall be able to enable, disable, add, remove, and reorder automatic color rules.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.2:** The first enabled matching rule shall supply the effective color. Later matching rules shall have no effect.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.3:** Rules shall support workflow step, repository, workflow, executor profile, task state, priority, and origin conditions.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.4:** If the step color is supported, a workflow-step rule shall use the current visible step color. Otherwise, it shall use gray.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.5:** If a task fact, rule order, output, or enabled state changes, the rule shall update the effective color.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.6:** If the required facts are available, rules shall apply to existing, new, active, and archived sidebar tasks.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.7:** A matching automatic rule shall take precedence over a manual color without erasing the manual value.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.8:** When no rule matches, the task shall show its manual color or no marker.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.9:** The task color menu shall identify an active automatic source and explain that manual colors do not override matching rules.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.10:** Automatic colors shall not mutate a task, workflow, executor, repository, or workspace setting.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.11:** Fixed rule colors shall support gray, red, orange, yellow, green, cyan, blue, indigo, purple, and pink.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.12:** The manual color menu shall keep its current seven colors. Manual color choices shall remain device-local.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.13:** An incomplete rule shall be disabled, shall match no task, and shall remain editable.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.14:** An origin rule shall offer Kanban as the value for a task that has no stored origin.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-002.15:** A task-state rule shall offer every raw task state. It shall not use the broader state groups from sidebar filters.

### REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003: Repository rule targets

**Intent:** Let repository rules cover current workspace sources and future task sources.

#### Acceptance criteria

- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.1:** The repository picker shall search workspace repositories and discovered local repositories.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.2:** The picker shall also search built-in remote providers and registered plugin repository providers.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.3:** The picker shall provide one refresh action and show source-specific loading, empty, unavailable, and error states.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.4:** A user shall be able to select a discoverable repository before the repository joins a workspace.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.5:** If any attached repository has the selected stable identity, the repository rule shall match.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.6:** An unavailable target shall remain editable and shall not match another repository with the same display name.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.7:** A workspace target shall not match a task from another workspace. It shall appear unavailable while another workspace is active.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-003.8:** A provider or local target shall match in each workspace that contains the same stable repository identity.

### REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004: Portable and responsive settings

**Intent:** Keep rule behavior consistent across devices and input methods.

#### Acceptance criteria

- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.1:** The enabled state and ordered rules shall persist in backend-owned personal settings.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.2:** Successful edits shall store rule order, targets, and output colors in backend settings. They shall appear after reload and in another browser for the same user.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.3:** A failed settings write shall restore the latest confirmed rules and show a localized error.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.4:** Missing or malformed settings shall resolve to disabled automation with no rules.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.5:** Desktop shall use the anchored popover and viewport-contained pickers.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.6:** Phone and tablet shall use the existing safe-area-aware drawer with one vertical scroll owner.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.7:** The mobile repository flow shall stay inside the current drawer instead of opening a nested drawer.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.8:** Touch rows, reorder controls, and standalone actions shall have a 44 CSS pixel target.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.9:** Rule order and repository selection shall support touch, pointer, and keyboard input.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.10:** The editor shall not cause document-level horizontal overflow at supported widths.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-004.11:** The editor shall disable Add rule at 50 rules and shall show a localized limit message.

## Out of scope

- Shared workspace color rules or a durable task color field.
- Rules for labels, projects, assigned agents, age, staleness, subtasks, pull requests, or CI status.
- Changes to workflow-step colors or executor configuration.
- Import or export of rule sets.
