---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-TASK-MENU-SUBMENUS-001
  - REQ-PLUGINS-TASK-MENU-SUBMENUS-002
  - REQ-PLUGINS-TASK-MENU-SUBMENUS-003
created: 2026-09-23
owners:
  - kandev
---
# Plugin task menu submenus System Design

## Purpose and boundaries

This design covers the host half of plugin task-menu submenus: the registration
contract, the one boundary that reads plugin values, the entry builder every card
and row surface uses, and the flattening that makes submenu children reachable
from the command palette and the sidebar's task commands.

It does not cover the plugin-facing authoring guide itself, the plugin-side
implementation that consumes the contract, or host-owned menu entries other than
plugin actions. Rendering changed no component: `KanbanCardMenuEntry` already
models `kind: "submenu"` for the built-in Move to / Priority / Link menus and both
renderers recurse, so cards, the shared desktop/mobile task-row menu, the
preview/detail menus and the palette inherit submenus from one entry model.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-TASK-MENU-SUBMENUS-001` | [Registration contract](#registration-contract), [Entry building](#entry-building), [The reading boundary](#the-reading-boundary) |
| `REQ-PLUGINS-TASK-MENU-SUBMENUS-002` | [The reading boundary](#the-reading-boundary), [Keys and identity](#keys-and-identity), [Icon resolution](#icon-resolution) |
| `REQ-PLUGINS-TASK-MENU-SUBMENUS-003` | [Command surfaces](#command-surfaces), [Shared entry reuse](#shared-entry-reuse) |

## Registration contract

`TaskMenuActionRegistration` gains optional `items?(context)`. `run` stays
required, so there is one contract for two host generations: a host predating the
field reads only `label`, `icon` and `run` and renders the flat item, and a build
whose `items()` yields nothing usable does the same. The host never awaits a menu
item, so `items()` is synchronous by contract.

`TaskMenuSubItemRegistration` is deliberately smaller than the parent: `id`,
`label`, optional `icon` and `disabled`, and `run`. It has no `group` and no
`visible`, because a child exists only inside its parent's submenu, and its own
nesting is not supported.

## Entry building

`pluginMenuEntry` produces exactly one `KanbanCardMenuEntry | null` per
registration: a `submenu` when `pluginSubItems` returns usable children, otherwise
a flat `item`, otherwise `null` for a registration the host cannot render. Both
callers — `buildPrimaryPluginEntries` (group `primary`) and `buildEditMenuEntry`
(group `edit`, nested inside the native `Edit` item) — filter that `null`.

`buildEditMenuEntry` builds and filters the plugin entries **before** choosing
between the submenu and the flat item, so a registration list that yields nothing
still leaves the native `Edit` item exactly as it was rather than wrapping it in a
submenu with no plugin child (AC-PLUGINS-TASK-MENU-SUBMENUS-002.3).

`buildCardPluginEntries` builds the two plugin-derived pieces of a card menu once
per render from the card's own inputs, and `useKanbanCardMenus` passes that result
to both the dropdown and the context variant, so a plugin's `items()` runs once per
card per render rather than twice.

## The reading boundary

`pluginSubItems` is the single place where host code reads a plugin's `items()`
result, and its whole body is one `try`. A plugin object may throw from any
property access — including the array methods a naive implementation would call
and the `then` lookup a naive thenable check would perform — so:

- the result is copied with `Array.from`, which never calls a method on the
  plugin's own object;
- every child is read once, inside the guard, into a snapshot of plain values, so
  a throwing getter or a Proxy fails that child and nothing is read twice;
- `id` and `label` must be non-blank strings after trimming, `run` must be
  callable, `disabled` must be a boolean or `null`, and `icon` must be a curated
  name, a component, a ready-made element or `null`;
- duplicate ids keep their first occurrence and the repeat is reported;
- a throw, a thenable, a non-array, an empty list or an all-unusable list return
  `null`, which is AC-PLUGINS-TASK-MENU-SUBMENUS-001.3's flat fallback.

Defects are reported through one helper keyed by action identity **and kind**, so
a registration that fails every render is reported once while a later, different
defect on the same action still reports. The helper never interpolates a raw
plugin value into a message: template interpolation calls `ToString`, which throws
for a `Symbol` id or a null-prototype object, and the omission paths are exactly
where such a value arrives (AC-PLUGINS-TASK-MENU-SUBMENUS-002.2). The read of the
registration's own `label`, `id` and `icon` is guarded for the same reason, and a
registration missing any of them contributes no entry.

The registry also reads each action inside a guard when it filters by group and
copies the registration. It skips an action whose getter or Proxy throws, reports
that registration once, and keeps other actions available. It does not validate
the registration when the plugin registers it.

The set of already-reported defects is module state that is never cleared. A
defect of the *same* kind that recurs on the same action after a plugin generation
swap therefore stays silent for the life of the page; that is a deliberate
tradeoff for a diagnostic line, and it is documented at the helper rather than
implied.

## Keys and identity

A plugin entry key is `"<prefix>-<encoded pluginId>%<encoded action id>"`, and a
child's key is `"<parent key>#<encoded child id>"`. Both ids are plugin-authored
strings and the palette flattens every plugin entry into one list where a key is
both a React key and cmdk's value, so the encoding has to be injective for any
string a bundle can produce.

`encodeKeyPart` percent-encodes per UTF-16 code unit, escaping everything
`encodeURIComponent` would not leave alone. Two properties matter:

- it cannot throw. `encodeURIComponent` raises `URIError` on a lone surrogate,
  which `slice` or `[0]` produce from an astral character, and that exception
  leaves the builder a card calls during render with no error boundary above it.
- it is injective. `%` is itself escaped, and every escape is exactly four hex
  digits, which makes each token self-delimiting so no escaped unit can run into
  the literal characters that follow it — `"%0"` and `"\u0250"` must not share a
  key, and neither may two different `(pluginId, actionId)` pairs whose ids
  differ only in how a dash join would spell them.

A child key always contains the `#` separator and a flat key never does, because
encoded ids contain neither `#` nor a bare `%`, so a child key can never equal a
flat action's key.

## Icon resolution

`lookupPluginIcon` resolves a curated name for strings only, and passes through a
component — including React's exotic components. `memo` and `forwardRef` return
objects that are callable per their types but not at run time, and
`@tabler/icons-react`, the set the curated map itself is built from, produces
exactly that shape, so a plain `typeof` check would reject the host's own icons.
`lazy` is excluded deliberately: it cannot resolve synchronously in a menu and
would suspend the menu's content. Elements are excluded here too, because this
function's result is passed to `createElement` by every non-menu surface; the
menu's own path returns an element unchanged before reaching it, which is the
documented, menu-only element tolerance (AC-PLUGINS-TASK-MENU-SUBMENUS-002.5).
The lookup resolves own keys only, so `PLUGIN_ICONS["__proto__"]` or
`["constructor"]` cannot resolve through the prototype chain to a non-icon.

## Command surfaces

`pluginCommandChoices` walks a built entry and turns a submenu into one command
per selectable child, carrying the trigger's label as the command's `context`;
`buildSidebarTaskCommands` preserves a command's own `context` and keeps the task
title as the default for everything else. The palette feeds it group `primary`
entries only, so `edit` stays card-only
(AC-PLUGINS-TASK-MENU-SUBMENUS-003.1, AC-PLUGINS-TASK-MENU-SUBMENUS-003.2). A
command's id is the entry key, which is what makes the two properties above matter
to the palette as well.

## Shared entry reuse

`BuildKanbanCardMenuEntriesArgs` accepts prebuilt plugin entries so one card
render can share them between its variants. Because those entries carry a disabled
state, an edit handler and a menu context, `forceFlatEdit` outranks a prebuilt set
inside `buildKanbanCardMenuEntries`, and the non-card menu tiers additionally drop
any set they are handed: a surface that forces the flat `Edit` form must never
render the nested `Edit` submenu those entries could contain
(AC-PLUGINS-TASK-MENU-SUBMENUS-003.3). The field is documented as valid only for a
call whose inputs match the ones the entries were built from; it is not
runtime-guarded, because no caller passes a mismatch today and the guard would
need the inputs carried alongside for a hazard that does not exist yet.

## Compatibility and failure behavior

- A host predating `items` ignores it: the field is additive and `run` is
  unchanged, so every existing plugin keeps its exact behavior.
- A registration the host cannot render contributes nothing and is reported,
  rather than rendering a broken row.
- A child the host cannot render is dropped, reported once, and the remaining
  children still render; if none remain the action falls back to its flat item.
- An exception anywhere in the plugin's `items()`, or in reading a registration's
  own fields, degrades at the boundary instead of propagating out of a render.

## Amendment log

**Amendment 1 (2026-09-23), from the adversarial review loop on PR #3874.** The
first implementation of this design used `encodeURIComponent` for the key parts
and a variable-width escape for the fallback encoder, and reported defects with a
raw template interpolation of the action's identity. Three defects came out of
review, each with a reproduction: an id containing a lone surrogate threw
`URIError` out of the card's render; `"%0"` and `"\u0250"` collided because a
two-digit escape is not self-delimiting; and a `Symbol` or null-prototype id made
the defect report itself throw. The contract is unchanged; the implementation now
escapes per code unit at a fixed width, reads plugin values defensively on every
path that reports them, admits `memo`/`forwardRef` component objects while
excluding `lazy` and elements, treats `null` as absent for both `icon` and
`disabled`, and keeps the native `Edit` item flat when no plugin entry wraps it.
