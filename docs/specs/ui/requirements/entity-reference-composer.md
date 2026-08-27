---
status: active
system: ui
created: 2026-07-21
updated: 2026-08-24
owners:
  - kandev
---
# Entity Reference Composer Requirements

## Overview

Users discuss external work items in agent chat, but plain ticket keys and pull-request numbers are ambiguous and easy to mistype. They need one fast way to search the active workspace's connected systems and insert a durable reference that both people and agents can resolve. Kandev tasks already have the established `@` mention path.

## Requirements

### REQ-UI-ENTITY-REFERENCE-COMPOSER-001: Entity Reference Composer

**Intent:** Users discuss external work items in agent chat, but plain ticket keys and pull-request numbers are ambiguous and easy to mistype. They need one fast way to search the active workspace's connected systems and insert a durable reference that both people and agents can resolve. Kandev tasks already have the established `@` mention path.

#### Acceptance criteria

- **AC-UI-ENTITY-REFERENCE-COMPOSER-001.1:** Task chat and Quick Chat support a `#` entity-reference trigger in their shared TipTap composer. Passthrough, task creation, comments, plans, Office text inputs, and other editors remain unchanged.
- **AC-UI-ENTITY-REFERENCE-COMPOSER-001.2:** Typing `#` at the start of a text block or after whitespace opens a search menu directly above the composer. Its shared geometry and visible-viewport containment follow the [composer suggestion overlay requirements](composer-suggestion-overlays.md). Its rendered bottom edge stays anchored to a composer anchor inside the visible viewport even when only a short result set is visible; an anchor below that viewport is clamped to its padded bottom edge. A `#` inside another token or a code block remains literal text.
- **AC-UI-ENTITY-REFERENCE-COMPOSER-001.3:** A trigger starts only when the user enters a new `#` character. Pasted, dropped, restored, or programmatically inserted text remains literal and does not open the menu.
- **AC-UI-ENTITY-REFERENCE-COMPOSER-001.4:** After the user types at least one query character, search covers the active workspace's connected, searchable sources:
- **AC-UI-ENTITY-REFERENCE-COMPOSER-001.5:** Jira tickets;
- **AC-UI-ENTITY-REFERENCE-COMPOSER-001.6:** Linear issues;
- **AC-UI-ENTITY-REFERENCE-COMPOSER-001.7:** GitHub issues and pull requests;
- **AC-UI-ENTITY-REFERENCE-COMPOSER-001.8:** GitLab issues and merge requests;

## System design

The migrated technical source is split into [part 1](../system-design/entity-reference-composer.md).
