---
status: active
system: ui
created: 2026-08-16
owners:
  - Kandev frontend
---
# Consistent settings typography Requirements

## Overview

Settings pages currently make equivalent content look different. Users move between labels, helpers, cards, provider forms, and system pages without a stable visual hierarchy, and narrow screens amplify the problem through cramped actions, small controls, and inconsistent wrapping.

## Requirements

### REQ-UI-SETTINGS-TYPOGRAPHY-001: Consistent settings typography

**Intent:** Settings pages currently make equivalent content look different. Users move between labels, helpers, cards, provider forms, and system pages without a stable visual hierarchy, and narrow screens amplify the problem through cramped actions, small controls, and inconsistent wrapping.

#### Acceptance criteria

- **AC-UI-SETTINGS-TYPOGRAPHY-001.1:** Settings pages SHALL use a shared semantic typography contract for page titles, page descriptions, section headings, card titles, field labels, helpers, errors, controls, metadata, badges, and technical values.
- **AC-UI-SETTINGS-TYPOGRAPHY-001.2:** The settings contract SHALL preserve the existing application sans family for normal UI copy and SHALL use the global or configured monospace family for technical values such as paths, identifiers, tokens, code, logs, and terminal/editor content.
- **AC-UI-SETTINGS-TYPOGRAPHY-001.3:** Page headers SHALL expose one page-level heading and description per route. Section headings and card headings SHALL preserve the document's semantic hierarchy instead of relying on visual classes alone. Card titles SHALL be rendered as an appropriate heading element when the card represents a titled content section.
- **AC-UI-SETTINGS-TYPOGRAPHY-001.4:** Equivalent settings cards SHALL share one title, description, weight, and line-height role. Card titles SHALL remain visually distinct from their metadata and helper text.
- **AC-UI-SETTINGS-TYPOGRAPHY-001.5:** Equivalent form fields SHALL share one label role, helper role, error role, and control role. Primary labels SHALL not use muted helper styling unless a documented metadata variant applies.
- **AC-UI-SETTINGS-TYPOGRAPHY-001.6:** Page and section descriptions SHALL use the readable description role. Field helpers SHALL use the compact readable helper role. Primary user-facing errors SHALL remain readable, while inline technical diagnostics MAY use a documented compact status role.
- **AC-UI-SETTINGS-TYPOGRAPHY-001.7:** Editable controls SHALL preserve the existing coarse-pointer anti-zoom behavior. Mobile selectors, buttons, standalone actions, and navigation rows SHALL provide an active hitbox of at least 44px unless an explicitly documented compact chrome exception applies.
- **AC-UI-SETTINGS-TYPOGRAPHY-001.8:** Settings composition SHALL use the canonical `md`/768px boundary for mobile/tablet layout changes unless a component documents a different boundary. Mobile layouts SHALL stack or wrap title/action groups when the available width would squeeze labels or descriptions.

## Migrated source detail

## Why

Settings pages currently make equivalent content look different. Users move
between labels, helpers, cards, provider forms, and system pages without a
stable visual hierarchy, and narrow screens amplify the problem through
cramped actions, small controls, and inconsistent wrapping.

## What

- Settings pages SHALL use a shared semantic typography contract for page
  titles, page descriptions, section headings, card titles, field labels,
  helpers, errors, controls, metadata, badges, and technical values.
- The settings contract SHALL preserve the existing application sans family
  for normal UI copy and SHALL use the global or configured monospace family
  for technical values such as paths, identifiers, tokens, code, logs, and
  terminal/editor content.
- Page headers SHALL expose one page-level heading and description per route.
  Section headings and card headings SHALL preserve the document's semantic
  hierarchy instead of relying on visual classes alone. Card titles SHALL be
  rendered as an appropriate heading element when the card represents a
  titled content section.
- Equivalent settings cards SHALL share one title, description, weight, and
  line-height role. Card titles SHALL remain visually distinct from their
  metadata and helper text.
- Equivalent form fields SHALL share one label role, helper role, error role,
  and control role. Primary labels SHALL not use muted helper styling unless a
  documented metadata variant applies.
- Page and section descriptions SHALL use the readable description role. Field
  helpers SHALL use the compact readable helper role. Primary user-facing
  errors SHALL remain readable, while inline technical diagnostics MAY use a
  documented compact status role.
- Editable controls SHALL preserve the existing coarse-pointer anti-zoom
  behavior. Mobile selectors, buttons, standalone actions, and navigation rows
  SHALL provide an active hitbox of at least 44px unless an explicitly
  documented compact chrome exception applies.
- Settings composition SHALL use the canonical `md`/768px boundary for
  mobile/tablet layout changes unless a component documents a different
  boundary. Mobile layouts SHALL stack or wrap title/action groups when the
  available width would squeeze labels or descriptions.
- Long translated, pseudo-locale, profile-name, path, and provider strings
  SHALL wrap or truncate within their owning surface without document-level
  horizontal overflow.
- Provider credential/value fields SHALL use one documented family and size
  role across GitHub, GitLab, Jira, Linear, Azure DevOps, and Sentry.
- Micro-type SHALL be limited to documented metadata, badges, counts, dense
  technical tables, code, logs, and similar specialized surfaces. It SHALL NOT
  be used for ordinary field labels, explanatory helpers, or primary errors.
- Existing settings behavior, routes, permissions, state, mutations, and
  translated message keys SHALL remain unchanged. Translation lookup SHALL
  continue at render time.
- The shared contract SHALL be verified on representative preference,
  agent/executor, workspace/provider, system, account, SSH, terminal/editor,
  and mobile navigation surfaces.

## Initial role contract

These are the starting roles for implementation. A role may have a named
responsive variant, but pages SHALL not introduce ad hoc values for the same
role.

| Role | Starting treatment | Notes |
| --- | --- | --- |
| Page title | 24px, bold, `h2` | One per settings route |
| Page description | 14px, muted, relaxed line height | Readable explanatory copy |
| Section heading | 18px, semibold, `h3` | Section actions stack on narrow widths |
| Card title | 16px, semibold, semantic heading | Heading level follows containment |
| Field label | 12px, medium, foreground | Compact variant must be explicit |
| Field helper | 12px, muted, relaxed line height | Not smaller than ordinary help |
| Primary error | 14px, destructive, readable line height | Inline technical diagnostics may be compact |
| Control value | One settings control variant | Editable phone controls retain anti-zoom sizing |
| Mobile action/navigation | Minimum 44px active hitbox | Font size does not replace touch geometry |
| Metadata/badge | Named 10–12px roles | No arbitrary 9/10/11px repeats |
| Technical value | Mono with named size role | Paths, IDs, tokens, code, logs, terminals |

## Scenarios

- **GIVEN** a top-level settings route on desktop, **WHEN** the page renders,
  **THEN** it shows one page title and description using the page-header roles,
  followed by section and card content with a stable hierarchy.
- **GIVEN** the same settings route on a Pixel 5 viewport, **WHEN** the page
  renders, **THEN** the same capabilities remain reachable, title/action groups
  stack or wrap intentionally, and primary controls meet the 44px hitbox rule.
- **GIVEN** a settings viewport between 640px and 767px, **WHEN** a section
  contains actions or add-row controls, **THEN** the canonical mobile/tablet
  composition prevents cramped side-by-side content and horizontal overflow.
- **GIVEN** equivalent cards from Preferences, Agents, Executors, Workspace,
  System, and Account, **WHEN** their titles and descriptions render, **THEN**
  they use the same settings card roles and appropriate semantic headings.
- **GIVEN** a form field with a label, helper, and validation error, **WHEN** it
  renders in account, profile, provider, workspace, or system settings,
  **THEN** each piece uses its shared role and the label is not visually
  downgraded to helper text.
- **GIVEN** a provider credential field, **WHEN** GitHub, GitLab, Jira, Linear,
  Azure DevOps, and Sentry forms render, **THEN** equivalent credential values
  use the same documented family and size treatment.
- **GIVEN** a long translated label, description, profile name, path, or
  pseudo-locale string, **WHEN** it renders on desktop, tablet, or phone,
  **THEN** it wraps or truncates within its surface without clipping or
  document-level horizontal scrolling.
- **GIVEN** a technical path, token, identifier, code block, log, terminal, or
  editor value, **WHEN** it renders, **THEN** it retains an intentional
  monospace/technical role without changing neighboring ordinary copy.
- **GIVEN** the combined Terminal Editors route, **WHEN** it renders, **THEN**
  it has one page-level title and separate section-level headings rather than
  a second page title nested halfway down the page.
- **GIVEN** Notifications and representative system/account/SSH/editor
  surfaces, **WHEN** computed-style tests run on desktop and mobile, **THEN**
  the tested semantic roles match the contract and remain covered against
  future drift.

## Out of scope

- Changing settings behavior, APIs, state management, permissions, persistence,
  or route information architecture beyond consolidating duplicate visual
  headers.
- Changing the global `@kandev/ui` typography contract for unrelated consumers.
- Rewriting translated content except for punctuation corrections required by
  the existing user-facing copy rules.
- Removing intentional technical/code/query/YAML/script/status typography
  exceptions.
- Introducing a new theme, color system, or general application-wide design
  system.

## Implementation plan

See [docs/plans/settings-typography/plan.md](../../../plans/settings-typography/plan.md).
