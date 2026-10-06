---
status: current
system: plugins
requirements:
  - REQ-PLUGINS-VOICE-EXTRACTION-HOST-001
---

# Task-create dialog completion system design

## Purpose

Expose successful task creation to plugin contributions rendered by that same create dialog. This lets a plugin associate state with the task it created without observing unrelated task events.

## Design

`TaskCreateDialog` owns a registry of task-created handlers and provides its registration function through a scoped React context. The public `PluginComposerSlotProps` type declares this optional capability, but the host supplies it only to create-mode `task-create-input-actions`. A registration returns its cleanup function; unmounting the slot unregisters its handler. Handlers may be synchronous or return a promise.

After the dialog's create operation succeeds, the dialog notifies each registered handler once with a frozen copy of the created task identity (`id` and `workspace_id`). The registry is not connected to the application-wide task event stream. Failed or canceled requests do not notify; edit and new-session surfaces do not receive the create-only capability. Synchronous throws and asynchronous rejections are logged per handler, and dispatch continues without changing the completed creation.

Handlers are scoped to the owning dialog open cycle. Closing, reopening, changing modes, or replacing the dialog revokes that cycle's registry before a late response can notify handlers for a later draft.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-PLUGINS-VOICE-EXTRACTION-HOST-001 | Design |

See [requirements](../requirements/voice-extraction-host.md) and the [implementation plan](../../../plans/task-create-dialog-completion/plan.md).
