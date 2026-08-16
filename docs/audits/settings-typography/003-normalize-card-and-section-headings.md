# TYPO-003: Normalize card and section heading roles

- Priority: P1
- Status: In progress
- Scope: Settings cards and section headings
- Related: TYPO-001, TYPO-002

## Finding

Equivalent settings cards do not share a predictable title role. Some use the base `CardTitle` size, some explicitly use `text-base`, and section headings use `text-lg`. This can be a valid hierarchy only when it is intentional and documented. Today it is mostly determined by which component authored the card.

## Evidence

- `apps/packages/ui/src/card.tsx:37` defaults `CardTitle` to `text-sm font-medium`.
- `apps/packages/ui/src/card.tsx:36-38` implements `CardTitle` as a non-heading `div`; only a small subset of settings cards manually nests `h3` elements, so equivalent cards differ in screen-reader heading navigation as well as visual type.
- `apps/web/components/settings/general-settings.tsx:74`, `apps/web/components/settings/general-settings.tsx:111`, and `apps/web/components/settings/general-settings.tsx:153` explicitly promote comparable settings card titles to `text-base`.
- A source inventory found 52 explicit `text-base` settings `CardTitle` usages mixed with plain default usages, including `apps/web/components/settings/profile-edit/profile-details-card.tsx:24-29` and `apps/web/components/settings/ssh-sessions-card.tsx:63-65`.
- `apps/web/components/settings/profile-edit/profile-details-card.tsx:27` uses the default `CardTitle` size for the same kind of settings card.
- `apps/web/app/settings/agents/[agentId]/profile-mcp-config-card.tsx:429` also uses the default title size, while `apps/web/components/settings/system/about-card.tsx:39` uses `text-base`.
- `apps/web/components/settings/settings-section.tsx:45` and `apps/web/components/settings/system/data-storage-settings.tsx:13` use `text-lg font-semibold` for section headings.
- `apps/web/app/settings/executors/page.tsx:106-115` and `:143-152` render executor card titles as un-sized paragraphs, so they inherit the card body's `text-xs/relaxed`; their descriptions are also `text-xs`.
- `apps/web/components/settings/plugins/marketplace-entry-row.tsx:27-43` leaves the marketplace entry name at the surrounding Plugins Browse `TabsContent` inherited `text-xs/relaxed` (`apps/packages/ui/src/tabs.tsx:70-75`) while explicitly rendering its description at `text-sm`, inverting the expected title-to-description hierarchy.
- `apps/web/components/settings/executor-profiles-card.tsx:65-79`, `apps/web/app/settings/agents/[agentId]/agent-setup-parts.tsx:225-237`, and profile-edit cards such as `env-vars-card.tsx:376-389` place title/description groups beside small action buttons without a shared `min-w-0` responsive header.
- `apps/web/components/sentry/sentry-settings.tsx:196-198` uses an ad-hoc `h3 text-sm font-semibold` for the Instances subsection, between the shared `text-lg font-semibold` section role and the `text-sm font-medium` card-title role.
- SSH cards at `apps/web/components/settings/ssh-connection-card.tsx:282-286`, `ssh-agent-readiness-card.tsx:191-196`, and `ssh-sessions-card.tsx:64-65` rely on default `CardTitle`/`CardDescription` sizing, while comparable settings cards use explicit `text-base` titles and `text-sm` descriptions.
- System Users and Backups headers at `apps/web/components/settings/system/users-table.tsx:270-293` and `backups-table.tsx:197-210` use fixed desktop-oriented title/action rows. The Account API token card has the same pattern at `apps/web/components/settings/account/api-tokens.tsx:230-243`.
- `apps/web/components/settings/system/storage/storage-policy-fields.tsx:23-34` and `storage-policy-card.tsx:505-509` use nested card titles at `text-sm`/`text-base` with `text-xs` descriptions, below the neighboring storage card and section hierarchy.

## User impact

Card titles that carry the same level of information can look different across Agents, Preferences, System, and Executor pages. The difference is easy to miss in code review because the default is hidden inside the shared UI package.

## Proposed direction

Define a settings-only semantic card-title primitive, or use `CardTitle` with `asChild`, so card titles render as `h3` where appropriate while the global CardTitle contract remains unchanged for non-settings consumers. Give it an explicit size, weight, and line-height variant. Keep the section-title role separate from the card-title role, then remove one-off `text-base`, `text-sm`, and weight overrides that duplicate the chosen role. Give executor and marketplace titles an explicit card-title role, with descriptions in the smaller description role. The shared card header should provide a `min-w-0` title group and stack actions at the canonical mobile boundary before using compact desktop actions.

## Verification

- Build a representative card gallery from Preferences, Agents, Executors, Workspace, Integrations, Account, and System.
- Include executor profile/type cards and marketplace plugin rows, and verify that each title is visually larger than its supporting description.
- Check long localized titles and descriptions beside add/delete actions at phone and narrow-tablet widths.
- Classify the Sentry Instances heading by its container and verify it matches either the card-subsection or full-section role.
- Include SSH, Users, Backups, API Tokens, and Storage Policy cards; verify technical values remain compact/mono without shrinking their surrounding card hierarchy.
- Add one cross-card semantic/typography contract test and verify heading navigation for settings cards.
- Confirm card titles, section titles, and page titles have a stable three-level hierarchy.
- Check icon alignment and wrapping after the class consolidation.

## Progress

- 2026-08-16: Opened from the settings source audit.
