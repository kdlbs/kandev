---
status: draft
system: plugins
created: 2026-09-23
owners:
  - kandev
---
# Plugin task menu submenus Requirements

## Overview

A plugin task-menu action has always been exactly one selectable item. A plugin
whose natural contribution is a short list of frequent choices therefore has
nowhere to put them: the Tags plugin opens a modal picker so the user can add one
of the workspace's recent tags to a card, one menu open plus one modal plus one
click. Registering five sibling actions instead would flood every card's menu
with rows that are mostly noise.

`registerTaskMenuAction` gains an optional `items(context)` beside its required
`run`. Declaring it renders the action as a one-level submenu whose children the
plugin supplies at menu-build time; the children reach the card menu, the shared
desktop/mobile task-row menu, the command palette and the sidebar's task
commands. `run` stays required and stays the flat behavior, which is the whole
compatibility story: a host that predates the field ignores it, and a build whose
`items()` yields nothing usable falls back to the same flat item, so an action
can ship both a quick child list and its existing behavior.

Plugin bundles are plain JavaScript, so the registration types are a promise
rather than a guarantee. Everything here is shaped by that: the host reads plugin
values defensively at one boundary, degrades to the flat item or drops a single
child instead of failing a render, and reports what it dropped.

## Requirements

### REQ-PLUGINS-TASK-MENU-SUBMENUS-001: A task menu action can declare a one-level submenu

**Intent:** A plugin with a short list of frequent, per-card choices needs to
offer them in one click each without adding sibling rows to every card's menu.
The host renders the action as a submenu when, and only when, the plugin supplies
children it can actually render.

#### Acceptance criteria

- **AC-PLUGINS-TASK-MENU-SUBMENUS-001.1:** `TaskMenuActionRegistration.items` SHALL be optional and synchronous, returning `readonly TaskMenuSubItemRegistration[]`; `run` SHALL remain required. A host that predates `items` SHALL ignore the unknown field and render the flat item it has always rendered, reading only `label`, `icon` and `run`.
- **AC-PLUGINS-TASK-MENU-SUBMENUS-001.2:** When `items()` returns at least one usable child, the action SHALL render as a submenu: `label` becomes an unselectable trigger, each child becomes a selectable entry in the returned order, each child's `run` is invoked with the same `PluginTaskMenuContext` as the action, and a child is unselectable when either the child's own `disabled` or the host's entry-level `disabled` is set. Nesting SHALL stop at one level: a child is never itself a submenu.
- **AC-PLUGINS-TASK-MENU-SUBMENUS-001.3:** A result with nothing usable SHALL fall back to the action's flat `run` item rather than producing a trigger nothing can open. Nothing usable means: `items` absent, `null`/`undefined`, a non-array, an empty array, a thrown exception, a thenable (the contract is synchronous), or an array whose every element is unusable. A thrown `items()` SHALL be caught and reported.
- **AC-PLUGINS-TASK-MENU-SUBMENUS-001.4:** A child the host cannot render SHALL be dropped, and the drop SHALL be reported once per action and defect kind rather than once per menu build. Unusable means: `id` or `label` absent, not a string, or blank after trimming; `run` not callable; `disabled` present and neither boolean nor `null`; `icon` present and neither a curated name, a component (including React's `memo` and `forwardRef`, which are objects at run time), a ready-made element, nor `null`. A child may be read only inside a guard, so a throwing getter or a Proxy fails that child instead of the card's render.
- **AC-PLUGINS-TASK-MENU-SUBMENUS-001.5:** A repeated child `id` SHALL keep its first occurrence and report the repeat, so the host never builds two children with one key. `icon: null` and `disabled: null` SHALL both mean "absent", matching how a JavaScript bundle spells an omitted field.
- **AC-PLUGINS-TASK-MENU-SUBMENUS-001.6:** `items()` SHALL be evaluated while the host builds that card's or row's menu entries, which happens on every render and for both the dropdown and the context variant whether or not a menu is open. The API documentation SHALL say so and SHALL tell authors to read cached state and memoize anything expensive.

### REQ-PLUGINS-TASK-MENU-SUBMENUS-002: An unrenderable registration never fails a host render

**Intent:** One plugin's malformed registration must not take down a card, a
board, or a command surface. The host's own contract says unusable input degrades;
these criteria make that true for the values a real bundle can produce, including
values whose own accessors throw.

#### Acceptance criteria

- **AC-PLUGINS-TASK-MENU-SUBMENUS-002.1:** A registration whose own `label` or `id` is not a usable string SHALL contribute no entry at all, and SHALL be reported once, instead of rendering an unselectable or unkeyable entry.
- **AC-PLUGINS-TASK-MENU-SUBMENUS-002.2:** The host's defect report SHALL NOT throw while reporting: a `Symbol` id, an object without a string form, or a throwing accessor on the registration SHALL still be reported or skipped without an exception escaping the builder.
- **AC-PLUGINS-TASK-MENU-SUBMENUS-002.3:** A group `edit` registration the host cannot render SHALL leave the card's native `Edit` item flat, exactly as if no plugin action were registered, rather than wrapping it in a submenu with no plugin child under it.
- **AC-PLUGINS-TASK-MENU-SUBMENUS-002.4:** An entry key SHALL be unique for any plugin-authored `pluginId`, action `id` and child `id`, without throwing for any string a bundle can produce, including a lone surrogate (the shape `slice` and `[0]` produce from an astral character). Because the key is also a palette row's React key and cmdk value, two entries SHALL never share one.
- **AC-PLUGINS-TASK-MENU-SUBMENUS-002.5:** A plugin-provided value that is not a usable icon SHALL resolve to the surface's fallback glyph rather than reaching `createElement` as an element type. Context menus SHALL additionally render a ready-made element unchanged, which is the shape plugins registered before icon resolution existed still send; that tolerance SHALL be documented as menu-only.

### REQ-PLUGINS-TASK-MENU-SUBMENUS-003: Submenu children are reachable from the command surfaces

**Intent:** A card's plugin contribution should be usable from the same
task-action surfaces that already list card actions, rather than only inside the
card's own menu.

#### Acceptance criteria

- **AC-PLUGINS-TASK-MENU-SUBMENUS-003.1:** A `primary`-group submenu SHALL contribute one command per child to the command palette and the sidebar's task commands, each carrying the trigger's label as its `context` so a bare child label stays identifiable; the sidebar SHALL keep the task title as the default `context` for commands that do not set their own.
- **AC-PLUGINS-TASK-MENU-SUBMENUS-003.2:** Group `edit` SHALL remain card-only: its submenu children SHALL NOT appear in the command palette or the sidebar's task commands.
- **AC-PLUGINS-TASK-MENU-SUBMENUS-003.3:** A surface that reuses already-built plugin entries SHALL produce them from the same inputs it renders with, and a caller that forces the flat `Edit` form SHALL NOT receive a prebuilt set that could contain the nested `Edit` submenu.

## Exclusions

- No nested plugin submenus: a child is a selectable item, never another submenu.
- No asynchronous `items()`: a menu entry cannot await, so a thenable is treated as a contract violation and falls back.
- The palette lists submenu children as flat commands; it does not render a nested palette submenu.
- Group `edit` gains no palette or sidebar placement.
- The host does not validate or repair a plugin's ids, labels or icons at registration time; malformed input is handled where it is used.

## System design

The technical source for these requirements is
[plugin-task-menu-submenus](../system-design/plugin-task-menu-submenus.md).
