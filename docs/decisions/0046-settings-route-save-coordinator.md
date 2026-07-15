# 0046: Settings route save coordinator

**Status:** accepted
**Date:** 2026-07-14
**Area:** frontend

## Context

Settings pages use several persistence models: immediate writes from controls, local Save buttons inside sections, and page-header Save buttons. Long pages can scroll the required action out of view, while pages with several independently owned cards need one predictable way to report and persist dirty state. Moving every settings form into one global data model would couple unrelated domains and make incremental migration difficult.

## Decision

The settings shell will own a route-scoped save coordinator. Descendant forms register stable contributors containing an identity, dirty and validation state, a save callback, and a discard callback. The coordinator renders one fixed bottom-right action, saves all dirty contributors in stable order, preserves per-contributor success or failure, and supplies dirty-route navigation protection.

Draft state and API payload construction remain with the domain component that owns the setting. A page-level owner combines sections that write the same backend resource so the coordinator does not issue competing replacements. Immediate named commands and dialog submissions do not register as dirty contributors.

The user-visible contract is defined by [Settings Manual Save](../specs/ui/settings-manual-save.md).

## Consequences

- Settings routes gain consistent save feedback and a viewport-reachable action without centralizing every domain model.
- Multiple cards, including workflow cards, can participate in one Save while retaining independent failure state.
- Route navigation can use the same dirty registry as the floating action.
- Components must maintain a saved baseline and compare submitted revisions so an in-flight completion cannot clear newer edits.
- Multi-resource saves remain non-atomic, and contributors that share a backend replacement contract must be composed before registration.

## Alternatives Considered

1. **A global settings draft store.** Rejected because unrelated user, workspace, workflow, integration, and runtime resources have different lifecycles and validation rules.
2. **One sticky Save button per card.** Rejected because a long route can contain several dirty cards and still leaves users hunting for the relevant action.
3. **A transient toast with Save.** Rejected because a toast can expire or be dismissed while unsaved state still exists.
4. **Keep autosave and add better progress feedback.** Rejected because it continues persisting intermediate configurations and does not establish explicit user intent.
