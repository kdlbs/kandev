# TYPO-006: Normalize input, select, and textarea typography

- Priority: P1
- Status: In progress
- Scope: Settings form controls
- Related: TYPO-001, TYPO-004, TYPO-005, TYPO-009, TYPO-010

## Finding

The base control components do not share one settings control type scale. Inputs and textareas start at `text-sm` and shrink to `text-xs` at `md`; select triggers and items are `text-xs` at all widths. Feature-specific forms add their own `text-sm` and mono overrides.

## Evidence

- `apps/packages/ui/src/input.tsx:11` includes both `text-sm` and `md:text-xs/relaxed`.
- `apps/packages/ui/src/textarea.tsx:10` includes the same responsive size change.
- `apps/packages/ui/src/select.tsx:40` and `apps/packages/ui/src/select.tsx:114` use `text-xs/relaxed` without matching the Input default.
- `apps/web/components/settings/plugins/plugin-config-form.tsx:67-79` uses a `text-sm` label beside controls that inherit the smaller select/input scale.
- `apps/web/components/settings/system/storage/storage-policy-fields.tsx:87-95` uses a muted `text-xs` label beside a shared Input.
- `apps/web/components/settings/terminal-settings.tsx:115-134` places a shared Input beside `text-xs` range and helper copy, while `apps/web/components/settings/terminal-settings.tsx:194-223` uses the same controls for font family selection and custom values.
- `apps/web/components/model-config-selector.tsx:483-495`, `apps/web/components/settings/model-combobox.tsx:92-106`, and `apps/web/components/settings/mode-combobox.tsx:38-52` use compact Button treatments for primary model/mode fields even on mobile.
- Executor environment SelectTriggers at `apps/web/components/settings/profile-edit/env-vars-card.tsx:73-75,123-126,187-189,265-267` force `text-xs`; the coarse-pointer 16px rule in `apps/web/app/globals.css:339-346` does not apply to Radix trigger buttons.
- Comparable provider credential controls split between mono `text-sm` inputs in `apps/web/components/github/github-pat-form.tsx:44` and `apps/web/components/gitlab/gitlab-settings.tsx:150,303`, versus bare sans Inputs in Jira, Linear, Azure DevOps, and Sentry forms. GitLab therefore remains 14px on desktop while the other provider fields inherit the smaller desktop control size.
- The global coarse-pointer rule in `apps/web/app/globals.css:327-346` forces editable input, textarea, and select text to 16px on phones to prevent iOS zoom, but does not cover Radix trigger buttons. Settings therefore needs explicit editable-control, selector, and action roles rather than relying on primitive defaults.

## User impact

The visible value, its label, and its helper can use three different scales. Desktop controls can also become smaller than their mobile counterparts solely because of the `md:` override, which makes the settings UI feel denser on larger screens.

## Proposed direction

Define explicit settings control roles for editable fields, selectors, and actions. Keep the 16px coarse-pointer anti-zoom floor for editable fields, use an intentional mobile treatment such as `min-h-11 text-sm` or `text-base` for selectors/actions, and use `md:min-h-7 md:text-xs` only for documented compact desktop variants. Decide explicitly whether credential/value fields use mono or sans, then apply one size rule across GitHub, GitLab, Jira, Linear, Azure DevOps, and Sentry. Decide explicitly whether the app-wide responsive control defaults should change or whether settings should use a wrapper/class variant. Preserve technical mono values as a family choice, not as a reason to use a smaller size.

The final choice must also respect the existing coarse-pointer input behavior and avoid browser zoom on phone form focus.

## Verification

- Measure label, control value, placeholder, and helper sizes together on desktop, tablet, and Pixel 5.
- Check SelectContent and nested options, not only the closed trigger.
- Check custom font names, paths, secrets, and code fields for wrapping and readability.
- Check model/mode selectors and executor environment selectors at phone and narrow-tablet widths; confirm they do not retain desktop-small text or 24px hitboxes.
- Compare equivalent provider credential/value fields side by side for family, size, placeholder, and password/token readability.

## Progress

- 2026-08-16: Opened from the settings source audit.
