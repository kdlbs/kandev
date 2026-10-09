---
status: active
system: ui
created: 2026-09-15
owners:
  - kandev
---

# Settings Header Tabs Requirements

## Overview

Settings pages can group related content without separate navigation rows.
UI owns this reusable interaction independently of the first consuming pages.
The [system-page contract](../../system-page/requirements/system-data-storage-pages.md) defines those pages and their content.

## Requirements

### REQ-UI-SETTINGS-HEADER-TABS-001: Consistent header navigation

**Intent:** A user can recognize and operate the same tab control across settings pages.

#### Acceptance criteria

- **AC-UI-SETTINGS-HEADER-TABS-001.1:** Desktop pages shall place tabs at the right of the title and description, inside the page header above its divider.
- **AC-UI-SETTINGS-HEADER-TABS-001.2:** Below 768px, tabs shall occupy a separate row below the description within the same header. The content shall retain one page scroll owner.
- **AC-UI-SETTINGS-HEADER-TABS-001.3:** Tabs shall share typography, spacing, selected treatment, and focus treatment. Overflow shall scroll within the tab strip without document horizontal overflow.
- **AC-UI-SETTINGS-HEADER-TABS-001.4:** Users shall operate tabs with pointer, keyboard, and touch. The active tab and associated panel shall have accessible names and selection relationships.
- **AC-UI-SETTINGS-HEADER-TABS-001.5:** Fine-pointer desktop tab targets shall use the standard 28px control height. Phone and coarse-pointer targets shall measure at least 44px.
- **AC-UI-SETTINGS-HEADER-TABS-001.6:** Pages without tabs shall preserve their current headers and actions. Long translated labels shall remain readable without overlapping the title or actions.

### REQ-UI-SETTINGS-HEADER-TABS-002: Predictable selection and drafts

**Intent:** Tab navigation preserves both the destination and unsaved settings.

#### Acceptance criteria

- **AC-UI-SETTINGS-HEADER-TABS-002.1:** A direct URL or reload shall select the named available tab. An absent or invalid selection shall select the page default.
- **AC-UI-SETTINGS-HEADER-TABS-002.2:** A tab click shall preserve unsaved edits and save/discard availability across panels. It shall neither save nor discard automatically.
- **AC-UI-SETTINGS-HEADER-TABS-002.3:** Leaving the page shall preserve the existing unsaved-change guard, including save failure and continue-editing behavior.
- **AC-UI-SETTINGS-HEADER-TABS-002.4:** A settings-search target shall reveal its owning tab before focus and highlight. Repeating the same target request shall work.
- **AC-UI-SETTINGS-HEADER-TABS-002.5:** Browser history traversal shall restore the selection encoded in the destination URL. Inactive content shall not receive keyboard focus.

## Out of scope

- A migration of every settings page to tabs.
- Domain data, persistence, policy, or permissions owned by consuming pages.
- New nested sidebars or a separate mobile navigation overlay.
