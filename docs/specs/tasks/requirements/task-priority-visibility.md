---
status: active
system: tasks
created: 2026-09-03
owners:
  - kandev
---

# Task priority visibility Requirements

## Overview

Kanban tasks use four priority tokens: `critical`, `high`, `medium`, and `low`.
People can see non-medium priorities on task cards and can set the priority when
they create or triage a task. Existing task storage, APIs, and events already
carry the value. This capability makes that value visible and usable on the
kanban board without changing the task model or priority vocabulary.

The `tasks` system owns this capability because priority is part of the durable
task record. The board is one presentation and interaction surface for that
record.

## Terminology

- **Priority token:** one of the four persisted values `critical`, `high`,
  `medium`, `low`. A wire and storage value, never translated.
- **Priority label:** the localized, human-readable rendering of a token.
- **Default priority:** `medium`. Applied by the backend when a create request
  omits priority.
- **Priority indicator:** the card element that conveys a task's priority.
- **Card menu:** the task card's action menu. It has two triggers that render
  from one shared entry list: a desktop-only right-click context menu, and a
  dots-button dropdown available at every breakpoint.
- **Board order:** the order cards appear within a workflow step, driven by the
  task's `position` and controlled by drag and drop.

## Prior art

The Office task surface already uses a priority picker and localized labels. It
has a separate vocabulary that includes `none` and supports workspace-defined
labels. Kanban keeps its own four-token vocabulary and localized labels, so the
two surfaces remain independent while following the same token-versus-label
convention.

## Requirements

### REQ-TASKS-PRIORITY-VISIBILITY-001: A task's priority is visible on its card

**Intent:** Let a person scanning the board see which tasks are elevated or
deprioritized without opening anything, while keeping the common case silent so
the indicator reads as an exception rather than as decoration on every card.

**User story:** As someone picking up work from the board, I want to see at a
glance which tasks are urgent, so that I choose what to start without opening
each card in turn.

#### Acceptance criteria

- **AC-TASKS-PRIORITY-VISIBILITY-001.1:** When a task's priority token is
  `critical`, `high` or `low`, the system shall render a priority indicator on
  that task's card.
- **AC-TASKS-PRIORITY-VISIBILITY-001.2:** When a task's priority token is
  `medium`, the system shall render no priority indicator on that task's card.
  `medium` is the default and the majority case, so its indicator would carry no
  information.
- **AC-TASKS-PRIORITY-VISIBILITY-001.3:** When a task's priority is absent, empty
  or is not one of the four priority tokens, the system shall render no priority
  indicator and shall not render the raw value as text. An unrecognized value
  shall not produce an error, a blank indicator or a broken card.
- **AC-TASKS-PRIORITY-VISIBILITY-001.4:** The system shall render a visually
  distinct indicator for each of `critical`, `high` and `low`, distinguishable
  from each other by more than color alone.
- **AC-TASKS-PRIORITY-VISIBILITY-001.5:** The system shall give the priority
  indicator an accessible name that includes the localized priority label, so a
  screen reader conveys the priority without relying on the visual treatment.
- **AC-TASKS-PRIORITY-VISIBILITY-001.6:** The presence, absence or value of the
  priority indicator shall not change board order. Rendering priority shall not
  reorder, regroup or re-position any card.
- **AC-TASKS-PRIORITY-VISIBILITY-001.7:** The indicator shall be correct on the
  first render of the board after a full page load, without waiting for a
  subsequent event, refetch or user action. Whatever payload hydrates the board
  initially shall carry each task's priority. A task that was already `critical`
  before the page was opened shall render as `critical` on first paint.
- **AC-TASKS-PRIORITY-VISIBILITY-001.8:** When a task's priority changes from any
  source, including another browser client, the Office task UI or a REST API
  caller, the card shall reflect the new value without a page reload. That list is
  illustrative: the card shall follow the stored value regardless of which writer
  produced it. Agents cannot currently set priority, because the MCP task tools do
  not accept the field.
- **AC-TASKS-PRIORITY-VISIBILITY-001.9:** A message that refreshes the board
  without carrying priority shall not clear, blank or alter the priority already
  displayed for a task. A client shall either receive priority in such a message
  or preserve the value it already holds. No board refresh, workflow switch or
  reconnect shall silently drop the indicator from a card whose priority has not
  changed.
- **AC-TASKS-PRIORITY-VISIBILITY-001.10:** The indicator shall be rendered at
  every breakpoint at which the card is rendered. There shall be no breakpoint at
  which a `critical`, `high` or `low` task appears identical to a `medium` one.

### REQ-TASKS-PRIORITY-VISIBILITY-002: A person can set priority when creating a task

**Intent:** Let the person who knows the urgency record it at the moment they
create the work, rather than creating the task and then correcting it.

**User story:** As someone filing a task I already know is urgent, I want to set
its priority while creating it, so that it is visible to everyone from the moment
it appears on the board.

#### Acceptance criteria

- **AC-TASKS-PRIORITY-VISIBILITY-002.1:** The primary task creation dialog shall
  offer a priority control presenting exactly the four priority tokens, each shown
  by its localized label. This is the shared dialog component, so the control shall
  appear no matter which entry point opened it (the board, the sidebar's new-task
  action, or an integration's task launcher). Subtask creation is a separate flow
  and is excluded; see `## Out of scope`.
- **AC-TASKS-PRIORITY-VISIBILITY-002.2:** The control shall default to `medium`.
  A person who ignores it shall create a `medium` task, which is the behavior
  before this capability.
- **AC-TASKS-PRIORITY-VISIBILITY-002.3:** When a task is created, the system shall
  submit the selected priority token, and the created task shall carry it. The
  created card shall then satisfy REQ-TASKS-PRIORITY-VISIBILITY-001.
- **AC-TASKS-PRIORITY-VISIBILITY-002.4:** The priority control shall be reachable
  and operable at every breakpoint at which the primary task creation dialog is
  available.
- **AC-TASKS-PRIORITY-VISIBILITY-002.5:** Selecting a priority shall not change any
  other field of the creation request, and shall not block, gate or reorder any
  other step of task creation.

### REQ-TASKS-PRIORITY-VISIBILITY-003: A person can change an existing task's priority

**Intent:** Let priority be corrected as work is triaged, from the board, without
navigating away from it.

**User story:** As someone triaging the board, I want to raise or lower a task's
priority in place, so that the board reflects the current plan without my leaving
it.

#### Acceptance criteria

- **AC-TASKS-PRIORITY-VISIBILITY-003.1:** The card menu shall offer a priority
  action presenting exactly the four priority tokens, each shown by its localized
  label.
- **AC-TASKS-PRIORITY-VISIBILITY-003.2:** The priority action shall indicate which
  token is the task's current priority, so a person can read the current value
  without changing it. When the task's priority is absent, empty or is not one of
  the four priority tokens, the action shall indicate no token as current, and
  shall not indicate `medium` or any other token in its place. Indicating a token
  the client does not hold would assert a current value that may be wrong, so
  indicating none is the honest rendering; all four tokens shall remain
  selectable in that state.
- **AC-TASKS-PRIORITY-VISIBILITY-003.3:** The priority action shall be available
  from both card-menu triggers. Because the right-click context menu is rendered
  only at desktop breakpoints, the dots-button dropdown shall carry the same
  priority action, so the capability is reachable on touch devices.
- **AC-TASKS-PRIORITY-VISIBILITY-003.4:** When a person selects a priority, the
  system shall persist that token against the task and shall change no other
  field of the task. It shall not change the task's title, description, state,
  workflow step, parent or position.
- **AC-TASKS-PRIORITY-VISIBILITY-003.5:** When a person selects the token that is
  already the task's priority, the system shall complete without error and leave
  the task's priority unchanged. Repeating the same selection any number of times
  shall leave the task in the same state as applying it once.
- **AC-TASKS-PRIORITY-VISIBILITY-003.6:** When two clients set different
  priorities on the same task, the system shall accept both writes in arrival
  order and the last write shall determine the stored value. Each client shall
  converge on the stored value from the resulting task event. No client shall
  merge, re-apply or resurrect its own pending value after receiving an event
  carrying a different one.
- **AC-TASKS-PRIORITY-VISIBILITY-003.7:** When persisting the priority fails, the
  system shall surface the failure to the person and the card shall continue to
  display the task's last known stored priority. A failed change shall not be
  presented as having succeeded.
- **AC-TASKS-PRIORITY-VISIBILITY-003.8:** Changing a task's priority shall not
  change board order, shall not move the card between workflow steps, and shall
  not start, stop or otherwise disturb any session on that task.

### REQ-TASKS-PRIORITY-VISIBILITY-004: Priority tokens are persisted and priority labels are localized

**Intent:** Keep the machine-readable token and the human-readable label
separate, so that translating the interface can never change what is stored,
compared or sent.

#### Acceptance criteria

- **AC-TASKS-PRIORITY-VISIBILITY-004.1:** The system shall treat `critical`,
  `high`, `medium` and `low` as the complete priority vocabulary for kanban
  tasks, and shall persist and transmit them verbatim in lower case.
- **AC-TASKS-PRIORITY-VISIBILITY-004.2:** The system shall never store, transmit
  or compare a translated priority string. Every comparison, selection and
  request shall use the token.
- **AC-TASKS-PRIORITY-VISIBILITY-004.3:** The system shall resolve every priority
  label at render time, so that changing the active locale re-renders every
  priority label without a reload.
- **AC-TASKS-PRIORITY-VISIBILITY-004.4:** The system shall provide priority labels
  in every supported locale. The labels shall be:

  | Token | `en` | `pt-pt` | `zh-cn` | `zh-hk` | `zh-tw` |
  | --- | --- | --- | --- | --- | --- |
  | (field name) | Priority | Prioridade | 优先级 | 優先級 | 優先順序 |
  | `critical` | Critical | Crítica | 紧急 | 緊急 | 緊急 |
  | `high` | High | Alta | 高 | 高 | 高 |
  | `medium` | Medium | Média | 中 | 中 | 中 |
  | `low` | Low | Baixa | 低 | 低 | 低 |

  These are the strings already carried in the `office` namespace in all five
  locales, reused here so this capability introduces no untranslated copy.
- **AC-TASKS-PRIORITY-VISIBILITY-004.5:** No priority label or related copy shall
  contain a Unicode em dash (U+2014).

## Out of scope

Each exclusion below is a decision, not an omission.

- **Sorting or filtering the board by priority.** The board remains manually
  ordered. Priority sorting or filtering needs its own product decision about how
  it interacts with that order and its own requirement.
- **Setting priority while creating a subtask.** Subtasks keep their existing
  creation flow and receive the default `medium` priority. A person can change a
  subtask's priority later from its card.
- **Server-side rejection of an invalid priority token.** This capability adds
  fixed four-option controls, so its user-facing writers cannot emit an invalid
  token. Consistent validation for every API writer is a separate hardening task.
- **MCP priority write support.** The MCP task contract is unchanged. Adding a
  priority argument to MCP tools is a separate contract change.
- **The Office task UI.** Office already surfaces priority, over a different
  five-token vocabulary that includes `none` and supports workspace-supplied
  labels. Nothing here changes it, and the two vocabularies are deliberately not
  merged.
- **A `none` priority, or any change to the vocabulary.** The four tokens are
  fixed by the database `CHECK` constraint. Adding a fifth would need a migration
  and is a different capability.
- **Any schema or migration work.** The column, its type, its default and its
  constraint already exist and are not touched.
- **Labels and tags.** Priority remains a task field. It is not modelled as a
  label or tag.
- **Changing priority from anywhere other than the board.** A control on the task
  detail page, in the session sidebar or in a command palette is additive and not
  required to satisfy REQ-TASKS-PRIORITY-VISIBILITY-003.
- **Bulk priority changes.** Setting priority across a multi-selection is a
  separate interaction with its own partial-failure semantics.
- **Priority on ephemeral tasks.** Quick-chat and other ephemeral tasks are
  filtered out before the board renders, so they have no card to carry an
  indicator.
- **Task-creation retry and idempotency semantics.** Adding a priority field to
  the creation request does not change how a repeated create is deduplicated. A
  deduplicated create returns the existing task and does not re-apply the
  submitted priority.
- **Priority as an input to scheduling.** Nothing here changes which task an
  agent, a WIP-queue pull or any automation selects. The existing pull-query
  ordering is unchanged.
- **A default priority per workflow, step or workspace.** The default stays the
  single backend value `medium`.
