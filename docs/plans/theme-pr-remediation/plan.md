---
created: 2026-10-23
status: complete
requirements:
  - REQ-PLUGINS-THEMES-001
  - REQ-PLUGINS-THEMES-002
  - REQ-PLUGINS-THEMES-003
  - REQ-UI-APP-THEMES-001
  - REQ-UI-APP-THEMES-002
system_design:
  - ../../specs/plugins/system-design/theme-packages.md
  - ../../specs/ui/system-design/application-themes.md
legacy_specs: []
---

# Implementation Plan: Theme PR Remediation

## Overview

Repair the four confirmed blockers in draft PR #4731, reviewed at
`3df5376052aab41851df584802dcafb68d7db46e`. Execute the work orders in the
listed order: constrain updates, accept VS Code token defaults, preserve Pierre
options, then repair Appearance fixtures and record final integration evidence.
The existing active requirements and current designs remain authoritative;
these are implementation conformance repairs, not new product contracts.

## Scope

### In scope

- Bind every theme update to its expected package kind, ID, version and digest.
- Handle valid unscoped VS Code token defaults without weakening native schema
  or scoped-token validation.
- Change Pierre palettes without changing non-theme rendering options.
- Restore the existing Appearance save, discard and concurrent-edit regression
  suite with representative auth and theme fixtures.
- Reconcile the original application-themes delivery record and live PR body
  with the final verification evidence, preserving screenshots and bot sections.

### Out of scope

New marketplace infrastructure, rollout flags, general executable-plugin
installer redesign, visual layout changes, unrelated PR findings, automatic
merge, and native subagents.

## Root causes and existing evidence

1. `applyAutoUpdates` passes theme listings to generic `InstallFromURL`; the
   Installed-tab action also discards all listing metadata except its URL.
   A temporary service probe made an opted-in theme update invoke the fake
   runtime for `unexpected-runtime` and report success. The guard against
   replacing a theme record does not protect a differently named package.
2. `convertTokens` requires every rule to have a scope. An otherwise valid
   exported theme with an unscoped default rule fails with
   `theme_scope: string or array required`.
3. `updatePierreTheme` supplies only `theme` to `setRenderOptions`. The pinned
   Pierre API replaces omitted options with defaults. A real manager probe
   changed the configured `word` algorithm to `word-alt` on a built-in switch.
4. `general-settings.test.tsx` supplies neither the auth state consumed by the
   new theme card nor the complete theme context and optional-store hook
   surface. Thirteen existing Appearance tests fail before their assertions.

## Technical approach

### Theme release admission

Factor the existing bounded theme downloader into a service-owned helper that
accepts `context.Context` and neutral expected-release values. Reuse it from
the import controller and automatic updates; keep Gin DTO mapping in the
controller. Verify digest, manifest ID/version, and the theme-only profile
against the exact immutable bytes before any installation or runtime admission.
Keep the existing lifecycle locks, version publication and inventory events.

Automatic updates must consider the installed record's profile as well as the
listing kind. A listing changing from theme to plugin must fail closed for an
installed theme, rather than selecting the executable installer. Require the
theme profile for every entry explicitly labelled as a theme, too.

For manual Installed-tab updates, preserve the complete marketplace entry in
the update callback. Add a distinct marketplace-update action that resolves the
installed record and routes theme updates through the existing URL prepare and
commit APIs. Mirror existing distribution metadata in `PluginRecord` where
needed; do not introduce a new endpoint. Retain ordinary plugin Browse installs
and their existing result/toast handling. A failed theme update leaves the old
record, files, selection and inventory usable.

| Entry point | Trusted input | Required admission | Primary evidence |
| --- | --- | --- | --- |
| Automatic update | Installed profile and catalog release identity | Exact theme kind, ID, version, SHA-256 | Real service/store with controlled HTTP transport |
| Installed-tab Update | Installed record and full catalog entry | Existing theme prepare/commit validation | Hook regression plus desktop/phone browser update |
| Themes review | Full selected release | Preserve existing bounded prepare/commit checks | Existing review and handler regressions |
| Ordinary plugin install | Existing plugin URL/upload contract | Preserve existing executable behavior | Existing plugin install/update suites |

### VS Code defaults

Treat an absent scope or an empty scope string as an unscoped default rule;
validate its settings before normalization. Fold default foreground/background
into missing editor roles, while explicit `colors.editor.*` values retain
precedence. Later unscoped defaults win over earlier defaults. Validate unscoped
font-style defaults and report them as ignored where the existing canonical
scoped-rule format cannot represent them; preserve explicit scoped styles.
Never invent a wildcard native scope or silently drop an unsupported default.
Preserve source-derived package identity and reject malformed settings and
unsafe values, including in defaults that an explicit color would override.

### Renderer and test fixtures

Read Pierre's current diff options inside the serialized update operation and
merge only the requested palette. Cover the actual manager API, not only a fake
that records theme names. Preserve the existing stale-switch test.

Repair the Appearance test setup with a representative authenticated admin state,
complete built-in theme context and optional-store accessors. Preserve all
eighteen existing test cases and their behavior assertions. Do not weaken the
production admin guard or replace the theme card with a null mock.

## ASCII UI preview

UI-01: Existing theme import review, after a valid VS Code file with defaults.

```text
Desktop dialog / phone review drawer
[Theme file] [Base appearance]
[Review theme]
  Identity + preview + compatibility report
[Close] [Review theme] [Install themes]
                       -> Appearance -> Select -> Save changes
```

Before: preparation rejects the default rule and never reaches review.
After: the same desktop dialog and phone drawer reach the existing review.
Control order, internal scrolling, safe-area actions and responsive geometry are
unchanged. This is the existing import surface, not a redesign. The original
UI-03 preview in `../application-themes/plan.md` remains the geometry reference.
Maps to AC-PLUGINS-THEMES-002.1 and AC-PLUGINS-THEMES-003.5.

## Tests

- AC-PLUGINS-THEMES-001.2, 003.1 and 003.3:
  `TestThemeMarketplaceUpdate*` in `service_theme_updates_test.go`; table-driven
  rejection of wrong kind/ID/version/digest, missing identity, listing-kind
  drift, and a successful update with no runtime or bundle admission.
- AC-PLUGINS-THEMES-003.1 and 003.3: Installed-tab hook tests preserve the full
  entry and route theme preparation/commit failures to the existing error state.
- AC-PLUGINS-THEMES-002.1 and 002.2: `TestConvertVSCode*` covers unscoped defaults,
  explicit-color precedence, later defaults, scoped styles and invalid inputs.
- AC-UI-APP-THEMES-001.3: real Pierre manager regression retains all non-theme
  options on built-in/custom changes; existing serialization coverage remains.
- AC-UI-APP-THEMES-002.1: all existing Appearance tests pass with real guard
  inputs, including delayed acknowledgements, draft preservation and failures.

## E2E tests

- Extend `apps/web/e2e/tests/plugins/theme-marketplace.spec.ts` with a real
  catalog-backed manual theme update. Add its phone counterpart as
  `mobile-theme-marketplace-update.spec.ts`, sharing a focused flow helper.
- Extend the existing VS Code fixture in `tests/settings/theme-flow.ts` with
  valid unscoped defaults and retain the existing explicit root colors. Run
  `application-themes.spec.ts` and `mobile-application-themes.spec.ts`.
- These prove the same existing controls and outcomes on both viewports; do
  not add layout changes or unrelated visual assertions.

## Work orders

- [x] [Task 01: Constrain theme update admission](task-01-theme-update-admission.md)
- [x] [Task 02: Import unscoped VS Code token defaults](task-02-vscode-defaults.md)
- [x] [Task 03: Preserve Pierre rendering options](task-03-pierre-options.md)
- [x] [Task 04: Repair Appearance fixtures and reconcile evidence](task-04-appearance-evidence.md)

All work stays sequential in the primary session. Task 04 records the combined
result after Tasks 01-03; those repairs otherwise have independent test boundaries.

## Verification results

All four work orders are done. The combined frontend run passed 89 tests in
15 files, including all 18 existing Appearance cases. Full plugin/CLI suites,
focused race tests, theme boot/gateway checks, 28 registry tests, web typecheck,
changed-file lint and documentation checks passed. Three desktop and two phone
E2E scenarios passed on production artifacts. Task 04 records the final evidence
and the increased Node heap needed for typechecking.

The repaired draft PR retains its screenshots and automated preview content.
Remote CI/review outcomes remain separate from this local verification.

## Risks

- A theme catalog's kind must not override its installed data-only profile.
- Unscoped defaults must not hide invalid input or override explicit editor colors.
- Pierre options must be read when the queued operation runs, not when queued.
- PR CI and review state remain external gates; local success alone does not
  make the draft ready to merge. Preserve the current PR body during updates.
