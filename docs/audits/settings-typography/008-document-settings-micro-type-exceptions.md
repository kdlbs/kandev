# TYPO-008: Define limits for micro-type and metadata

- Priority: P2
- Status: Resolved
- Scope: Settings navigation, badges, technical metadata, and compact status rows
- Related: TYPO-001, TYPO-005, TYPO-009, TYPO-010

## Finding

Settings contains many intentional compact values, but the exceptions are expressed as ad hoc sizes such as `text-[10px]`, `text-[11px]`, and `text-[0.7rem]`. There is no documented boundary that distinguishes technical metadata from user-facing explanatory copy.

## Evidence

- `apps/web/components/app-sidebar/sections/settings/settings-nav-primitives.tsx:270` uses `text-[10px]` for navigation group headers, while `settings-nav-primitives.tsx:135` and `settings-nav-primitives.tsx:212` use `text-[13px]` for rows.
- `apps/web/components/app-sidebar/sections/settings/settings-tree.tsx:89` uses `text-[11px]` for count badges.
- `apps/web/components/settings/system/licenses-list.tsx:54`, `system/licenses-list.tsx:60`, `system/licenses-list.tsx:64`, and `system/licenses-list.tsx:84` use 14px, 12px, 10px, and 10px technical text in one list.
- `apps/web/components/settings/plugins/plugin-manifest-card.tsx:97` uses `text-[11px] font-mono` for capability badges.
- `apps/web/components/settings/sleep-inhibition-settings.tsx:32` defines a `text-[0.7rem]` code class.
- `apps/web/components/settings/profile-form-fields.tsx:141-145` uses 10px for compact helper copy, which is not technical metadata.
- `apps/web/components/settings/record-badges.tsx:9` defines a 9px badge base while `:36` overrides Active Workspace to 10px, and `apps/packages/ui/src/badge.tsx:7-9` already establishes a 10px default.
- `apps/web/components/settings/installed-agent-card.tsx:72-76` places 9px and 10px status badges in the same identity row, creating a second metadata-scale decision for equivalent statuses.
- Type badges override the shared 10px baseline to `text-xs` in `apps/web/components/settings/profile-edit/profile-edit-page-chrome.tsx:74-77`, executor creation/profile headers, and `apps/web/app/settings/executors/page.tsx:115-117`; MCP policy badges use `text-[10px] uppercase tracking-wide` while the built-in MCP chip uses `text-xs`.
- `apps/web/components/settings/profile-status-panels.tsx:141-162` renders the actionable Details trigger as `text-[10px] uppercase tracking-wide` beside a 14px title and 12px hint, making diagnostic content smaller than adjacent metadata.
- A static settings inventory found roughly 38 `text-[10px]`, 20 `text-[11px]`, and one `text-[9px]` occurrence. Additional clusters appear in workflow prompt/editor metadata, plugin rows and manifests, system user/health status, and profile compact helpers.

## User impact

Compact technical values can be appropriate, but the same small sizes can leak into labels or helpers. This makes the visual system difficult to reason about and risks unreadable settings copy.

## Proposed direction

Create named roles for `SettingsMeta`, `SettingsBadge`, `SettingsCode`, and `SettingsHelperCompact`. Use the shared 10px status-badge scale unless a badge has a documented reason to differ; define a readable 11/12px metadata variant where needed and reserve smaller micro-type for explicit dense technical surfaces. Make actionable diagnostic controls at least `text-xs font-medium` and normal case. Allow micro-type only for identifiers, counts, badges, code, logs, or dense technical tables. Do not use it for a decision explanation, field label, or destructive consequence. Keep mono and size as separate decisions.

## Verification

- Inventory every arbitrary settings font size before migration.
- Review each use against the exception policy.
- Review status-badge rows for one consistent metadata scale.
- Review diagnostic actions and MCP/type badges for readability beside their titles and helpers.
- Add a lint or review checklist guard for new arbitrary settings sizes.

## Progress

- 2026-08-16: Opened from the settings source audit.
