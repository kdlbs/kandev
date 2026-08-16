# TYPO-010: Align mobile and desktop settings type roles

- Priority: P1
- Status: In progress
- Scope: Phone settings index, settings sidebar, workspace tabs, and mobile settings pages
- Related: TYPO-001, TYPO-006, TYPO-008, TYPO-012

## Finding

Mobile settings uses a larger navigation row type than the desktop settings tree, while workspace tabs and other mobile surfaces add their own type classes. The larger mobile row is reasonable for touch use, but it is not represented as a semantic responsive role. Existing mobile tests mostly assert visibility and geometry, so typography drift can ship without detection.

## Evidence

- `apps/web/components/app-sidebar/sections/settings/settings-nav-primitives.tsx:135` and `settings-nav-primitives.tsx:212` use `text-[13px]` for desktop settings rows.
- `apps/web/components/app-sidebar/sections/settings/settings-nav-primitives.tsx:270` uses `text-[10px]` for desktop section headers.
- `apps/web/components/settings/settings-page-nav.tsx:13` forces phone links and buttons to `text-sm` and `min-h-10`.
- `apps/web/components/settings/workspaces/workspace-settings-shell.tsx:187` uses `text-sm` for workspace tabs at both widths, then changes the visual container at `md`.
- `apps/web/hooks/use-responsive-breakpoint.ts:3,19-28` defines `<768px` as mobile, but shared settings compositions switch at `sm/640px` in `components/settings/settings-section.tsx:42-53`, `components/settings/agent-profile-page.tsx:95-105`, `components/settings/profile-edit/profile-edit-page-chrome.tsx:70-91`, and related form rows. At 640-767px, titles, descriptions, and actions can remain side-by-side while the app treats the viewport as mobile/tablet.
- `apps/web/e2e/tests/settings/mobile-settings-index.spec.ts` and `apps/web/e2e/tests/settings/mobile-settings-discovery.spec.ts` verify routes, visibility, bounds, and focus, but do not verify computed typography.
- `apps/web/e2e/tests/settings/mobile-notifications-type-scale.spec.ts:28-43` is the exception: it measures one notification list's computed size.
- `apps/web/app/settings/agents/page.tsx:371` uses a small button without the `h-11` mobile floor used by installed-agent actions at `apps/web/components/settings/installed-agent-card.tsx:82-112`; `apps/web/components/settings/workspaces/workspaces-page-client.tsx:182` has the same unqualified small-button pattern for Add.
- Other top-level actions follow the same compact default: `apps/web/components/settings/plugins/plugins-settings.tsx:129-145` (Sync/Install) and `apps/web/components/settings/prompts-settings.tsx:543-549` (Add prompt) do not add a phone override, so they remain 12px labels in 24-28px controls.
- Primary model/mode selectors and executor environment selectors retain compact desktop-small treatments on phone, while profile headers and card actions can squeeze long names and descriptions.
- Phone settings navigation has competing owners: `apps/web/components/settings/settings-page-nav.tsx:11-15` applies descendant `text-sm`, while desktop leaf/branch rows and group headers hard-code 13px/10px in `apps/web/components/app-sidebar/sections/settings/settings-nav-primitives.tsx:127-142,211-215,268-271`. Settings search adds mobile `text-base` input, 10px group headings, 13px result labels, and 11px breadcrumbs at `apps/web/components/app-sidebar/sections/settings/settings-search.tsx:98-102,159-165,194-205`.
- `apps/web/components/settings/settings-page-nav.tsx:11-15` gives phone tree links/buttons `min-h-10` (40px), while settings search result rows use `min-h-11` (44px) at `apps/web/components/app-sidebar/sections/settings/settings-search.tsx:195-200`; the phone settings tree is therefore below the shared mobile navigation hitbox baseline.
- SSH card headers/actions at `apps/web/components/settings/ssh-connection-card.tsx:280-289,360-388`, `ssh-agent-readiness-card.tsx:189-209`, and `ssh-sessions-card.tsx:62-78` keep long descriptions and actions in single rows. System Users/Backups and Account API Token headers use similar fixed rows at `system/users-table.tsx:270-293`, `system/backups-table.tsx:197-210`, and `account/api-tokens.tsx:230-243`.
- Licenses filtering/count at `apps/web/components/settings/system/licenses-list.tsx:131-145` keeps an input beside a `text-xs whitespace-nowrap` count; editor previews/forms use non-wrapping action rows at `components/settings/editors-settings.tsx:205-267` and `components/settings/editor-form.tsx:358-370`.

## User impact

The phone composition can feel visually disconnected from the desktop tree, and a future change to one branch can create inconsistent type or wrapping on small screens without a focused regression.

## Proposed direction

Define a responsive settings navigation and composition role with an intentional desktop size, mobile size, line height, and hitbox. Normalize phone settings tree links/buttons to `min-h-11`/44px, while retaining desktop sidebar density. Align shared settings composition breakpoints with the canonical `md`/768px contract unless a `sm` exception is documented. Keep the 44px touch-target requirement separate from the font size, and give top-level primary actions, selectors, and card actions the same mobile minimum hitbox before applying a compact desktop override. Use the same semantic roles for workspace tabs and settings index rows where they carry the same navigation meaning.

## Verification

- Use the configured Pixel 5 project and a desktop viewport.
- Measure row labels, group labels, workspace tabs, and search results.
- Check long labels, pseudo-locale copy, focus rings, and absence of horizontal overflow.
- Check top-level action labels at phone widths for the 44px hitbox and consistent label density.
- Add a 640-767px viewport check for stacked headers, wrapped descriptions, selectors, and action controls.
- Add computed-style assertions for phone navigation rows, search input/results, and group headings so each responsive size has one owning primitive.
- Verify the 44px tree hitbox does not create undesirable scroll inflation or clip section headers.
- Include SSH, Users, Backups, Licenses, API Tokens, and editor previews/forms in mobile stacking and action-hitbox checks.

## Progress

- 2026-08-16: Opened from the settings source audit.
