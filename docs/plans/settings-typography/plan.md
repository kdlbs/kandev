---
spec: docs/specs/ui/requirements/settings-typography.md
created: 2026-08-16
status: building
---

# Implementation Plan: Consistent settings typography

## Overview

Introduce a settings-specific typography layer that gives equivalent settings
content stable semantic roles without changing the global `@kandev/ui`
contract. Migrate shared shells and cards first, then forms and controls,
provider/workspace surfaces, system/account/technical surfaces, and finally
rendered regression coverage. The sequence keeps each migration reviewable and
preserves technical mono exceptions and mobile anti-zoom behavior.

## Frontend

### Shared role primitives and shells

- Add `apps/web/components/settings/settings-typography.tsx` for named settings
  roles: page title/description, section heading/description, field label,
  description, helper, error, metadata, badge, and technical code/value.
- Add `apps/web/components/settings/settings-card-header.tsx` for semantic
  card title/description/action composition, including `min-w-0`, wrapping,
  and narrow-screen action stacking.
- Update `apps/web/components/settings/settings-page-template.tsx`,
  `settings-section.tsx`, `system/system-page-shell.tsx`,
  `profile-edit/profile-edit-page-chrome.tsx`, and
  `workspaces/workspace-section-header.tsx` to consume the shared roles.
- Normalize Utility Agents, legacy global Integrations, legacy executor
  profile chrome, and the combined Terminal Editors route to one page-level
  header and appropriate section-level headings.
- Keep global `apps/packages/ui/src/card.tsx` behavior unchanged for its
  non-settings consumers. Use settings composition or `asChild` semantics for
  accessible card headings.

### Forms and controls

- Add `apps/web/components/settings/settings-field.tsx` and
  `settings-control.tsx` only where shared composition is needed for labels,
  helpers, errors, editable controls, selectors, and action variants.
- Add focused role, field-composition, and semantic card-header tests so new
  settings surfaces can reuse the contract without depending on global
  `@kandev/ui` defaults.
- Migrate profile, agent, executor, MCP, environment-variable, model/mode,
  account, system, and workflow fields to the field/control roles.
- Preserve the coarse-pointer 16px editable-control rule and define named
  mobile/desktop selector and action variants rather than relying on primitive
  defaults.

### Workspace, provider, and integration surfaces

- Migrate workspace headers, repository cards/dialogs, GitHub/GitLab/Jira/
  Linear/Azure DevOps/Sentry forms, automation helpers, and workflow editor
  labels to shared settings roles.
- Choose and apply one documented credential/value family and size across all
  providers. Keep technical values mono where the role requires it.

### System and account surfaces

- Migrate SSH cards, system Users/Backups/Licenses/Storage Policy surfaces,
  Account token/security tables and dialogs, and diagnostic actions.

### Technical surfaces

- Migrate plugin metadata, badges, technical values, SSH technical details,
  PTY/Monaco editors, and editor previews.
- Use named metadata/badge/code roles and preserve compact mono treatment for
  fingerprints, IDs, paths, tokens, logs, code, YAML, scripts, queries, and
  editor content.
- Replace hard-coded PTY/Monaco editor family or size values with the existing
  terminal/editor theme tokens, unless the surface is documented as a distinct
  role.

## Tests

- Add focused component tests for the shared settings role primitives and
  semantic card headings.
- Update existing settings tests for notification event/group title roles and
  preserve render-time i18n behavior.
- Add unit/component coverage for responsive control variants, long-name
  wrapping, primary versus inline error roles, and provider credential role
  parity where a deterministic rendered assertion is practical.
- Run the web typecheck, i18n check, and i18n ratchet after each migration wave.

## E2E Tests

- **Scenario:** Representative desktop settings routes expose one page header
  and stable page/section/card/field roles.
  **File:** `apps/web/e2e/tests/settings/settings-typography.spec.ts`.
  **What to verify:** Appearance, Agents/Executors, Workspace/provider,
  System/Account, SSH, and Terminal Editors render the intended hierarchy,
  long labels remain contained, and technical values retain their role.

- **Scenario:** Phone settings preserve the same capabilities with intentional
  stacking, 44px actions/navigation, and no document-level horizontal overflow.
  **File:** `apps/web/e2e/tests/settings/mobile-settings-typography.spec.ts`.
  **What to verify:** Pixel 5 page navigation, 640–767px tablet composition,
  mobile selectors/actions, SSH/system/account card headers, editor rows, and
  long translated/pseudo-locale content.

- **Scenario:** Existing Notifications type-scale coverage validates the
  shared role contract rather than page-specific descendant sizes.
  **File:** `apps/web/e2e/tests/settings/notifications-type-scale.spec.ts` and
  `mobile-notifications-type-scale.spec.ts`.
  **What to verify:** Event titles, group headings, descriptions, provider
  rows, and table rows at desktop and mobile widths.

## Mobile design contract

- Desktop outcome: settings pages expose the same hierarchy and actions with
  compact, information-dense composition.
- Phone entry point: the existing settings navigation/search surfaces remain
  the entry point; no new drawer or route is needed for typography migration.
- Nearest shipped exemplars: `useResponsiveBreakpoint`,
  `settings-section.tsx`, existing mobile settings navigation/search tests,
  and the Notifications type-scale tests.
- Mobile hierarchy: page title and primary action remain first; section/card
  title groups stack before controls or secondary actions; technical metadata
  remains subordinate.
- Touch and geometry: active controls/navigation rows are at least 44px,
  editable controls preserve anti-zoom sizing, title groups use `min-w-0`,
  and document horizontal overflow remains zero.
- Scroll ownership: preserve existing settings page scroll ownership; do not
  introduce nested scrolling for typography-only changes.
- Shared logic: state, mutations, permissions, filtering, and i18n keys remain
  shared across viewports; only presentation and named responsive variants
  change.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-typography-primitives-and-shells](task-01-typography-primitives-and-shells.md)

Wave 2 (parallel-safe after Wave 1; user authorization is still required for
delegation):

- [ ] [task-02-form-controls-and-profile-migration](task-02-form-controls-and-profile-migration.md)
- [ ] [task-03-workspace-provider-migration](task-03-workspace-provider-migration.md)
- [ ] [task-04-system-account-migration](task-04-system-account-migration.md)
- [ ] [task-05-technical-surfaces-migration](task-05-technical-surfaces-migration.md)

Wave 3:

- [ ] [task-06-typography-regression-coverage](task-06-typography-regression-coverage.md)

Wave 2 tasks touch disjoint feature components but all depend on the shared
roles from Wave 1. Wave 3 depends on the final markup and classes from every
migration task.

## Verification Results

- Focused settings, field-composition, semantic card-header, integrations, and
  notification component tests: 5 files and 20 tests passed.
- Web typecheck: passed.
- Web lint: passed with zero warnings.
- i18n check: passed; existing advisory locale parity warnings remain.
- i18n ratchet: passed.
- Desktop settings typography E2E: 2 passed for Appearance and Terminal
  Editors.
- Mobile settings typography E2E: 2 passed for Pixel 5 and the 700px
  narrow-tablet viewport.
- Existing Notifications type-scale E2E: 1 desktop and 1 mobile test passed;
  provider metadata is explicitly 12px while event titles are 14px.
- `git diff --check`: passed.

The shared contract and shell wave is complete. High-impact form, provider,
system/account, technical, and responsive surfaces now consume the shared
roles. Dedicated representative E2E coverage for system/account/SSH and the
remaining audit callsites are still in progress and are explicitly tracked in
the audit README rather than treated as complete. The full web test suite was
not completed because the existing plugin-host happy-dom stylesheet tests
emitted external-stylesheet warnings and did not finish in the available run.
