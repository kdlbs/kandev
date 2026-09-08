# Settings typography improvement plan

Status: Reference audit plan

Formal spec: [../../specs/ui/requirements/settings-typography.md](../../specs/ui/requirements/settings-typography.md)

Canonical implementation plan: [../../plans/settings-typography/plan.md](../../plans/settings-typography/plan.md)

Related audit: [README.md](README.md)

## Goal

Create a predictable typography system for every settings page. Users should be
able to distinguish page titles, section titles, field labels, descriptions,
controls, status text, and technical values without each page inventing a
slightly different size or font family.

This plan preserves hierarchy. It does not make every settings string the same
size: each semantic role gets one default, with responsive changes only where
they improve readability or interaction.

## Design checkpoint

- Keep Figtree as the default settings UI family through the existing sans
  token.
- Use the existing global or configured monospace family for technical values,
  code, tokens, paths, and terminal content.
- Define shared roles for page headers, section/card titles, field labels,
  descriptions, helper text, controls, metadata, badges, errors, and code.
- Keep line-height and color paired with each role so a size change does not
  create a new one-off visual treatment.
- Use shared settings shells and field primitives instead of repeating local
  `text-*` decisions across routes.

## Implementation phases

### 1. Agree on the role contract and inventory

Start with findings 001, 008, and 009. Turn the role map into named shared
tokens or primitives, document the intentional micro-type exceptions, preserve
semantic h2/h3 heading levels, and confirm which technical/editor surfaces use
monospace or existing editor tokens. Record the final decisions in the shared
settings UI guidance before migrating callers.

### 2. Unify page and card hierarchy

Implement findings 002 and 003. Consolidate the duplicated page-header styles
used by the general, system, profile, and workspace shells, including Utility
Agents, legacy Integrations, legacy executor profile routes, and long-name
wrapping. Include SSH, Users, Backups, API Token, and Storage Policy card
headers. Then align card and section titles and descriptions so comparable
settings surfaces have the same hierarchy.

### 3. Migrate fields, helpers, controls, and profiles

Implement findings 004 through 007. Introduce shared label, helper, error, and
control roles. Migrate account, system, preference, and profile forms,
including mobile model/mode selectors and profile-form errors. Remove the
agent-profile compact 10px helper treatment unless it is reclassified as a
deliberate metadata role that remains readable and documented. Migrate account
and system labels, dense-table rows, visible guidance, and primary errors.

### 4. Migrate workspace, provider, and technical surfaces

Implement findings 008 through 011. Apply the contract to workspace sections,
repository and integration forms, plugin/system metadata, code blocks, and the
PTY/Monaco editors. Keep technical values monospace, but do not let technical
styling change ordinary labels or descriptions by accident.

### 5. Verify and prevent regression

Implement findings 012 and 013. Extend computed-style coverage beyond Notifications to
representative preference, system, account, agent, workspace, and integration
routes. Add mobile assertions for semantic roles, anti-zoom control relations,
navigation/search ownership, notification event titles, system/account tables,
SSH cards, licenses, and editor rows where a desktop/mobile drift could
reappear. Keep the tests focused on role contracts rather than fragile
implementation class names.

## Mobile contract

- Preserve the same semantic roles and hierarchy on desktop and mobile.
- Keep navigation and controls at least 44px tall; do not use a smaller font as
  a substitute for a touch target.
- Use the canonical `md`/768px responsive boundary for settings composition
  changes unless a component has a documented reason to differ.
- Allow only deliberate responsive type changes, such as a compact navigation
  label, and test them as role variants.
- Preserve one clear scroll owner and avoid typography changes that cause
  clipped labels, cramped tablet layouts, or horizontal overflow.

## Tracking and validation

| Phase | Findings | Status |
| --- | --- | --- |
| Role contract and exceptions | 001, 008, 009 | Proposed |
| Page and card hierarchy | 002, 003 | Proposed |
| Fields, helpers, controls, profiles | 004, 005, 006, 007 | Proposed |
| Workspace, providers, technical surfaces | 008, 009, 011 | Proposed |
| Verification and regression protection | 010, 012, 013 | Proposed |

Before implementation is considered complete, run the focused settings unit
and Playwright tests, the web typecheck, and the i18n checks. Update each
finding's status and evidence in [README.md](README.md) as work lands.
