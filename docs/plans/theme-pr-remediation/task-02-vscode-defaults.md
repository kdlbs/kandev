---
id: "02-vscode-defaults"
title: "Import unscoped VS Code token defaults"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-THEMES-002
  - REQ-PLUGINS-THEMES-003
acceptance_criteria:
  - AC-PLUGINS-THEMES-002.1
  - AC-PLUGINS-THEMES-002.2
  - AC-PLUGINS-THEMES-003.5
system_design:
  - ../../specs/plugins/system-design/theme-packages.md
---

# Task 02: Import Unscoped VS Code Token Defaults

## Summary

Accept valid unscoped token defaults in standalone VS Code JSON/JSONC themes.
Normalize them into the existing canonical roles and report approximations;
retain the strict native schema and current reviewed-import flow.

## In scope

- Distinguish absent/empty scope from malformed scope values. Validate all
  default settings even when a later default or explicit root color wins.
- Apply the default precedence and explicit unsupported-style reporting
  described in the plan; preserve explicit colors and scoped token styles.
- Extend converter and review tests with real exported-file shapes, mixed
  defaults/scoped rules, precedence and invalid defaults.
- Extend the existing shared browser fixture so desktop and phone imports
  exercise an unscoped rule. Check public-doc impact with `/docs-maintainer`.

## Out of scope

Includes, VSIX, arbitrary wildcard native rules, schema relaxation and new UI.

## Acceptance

1. A valid theme with an absent or empty scope reaches review and installs;
   its default foreground/background and style data are handled deterministically.
2. Explicit editor colors and scoped styles retain precedence; malformed or
   unsafe defaults fail rather than silently disappearing.
3. Existing desktop and phone import/save/reload/recovery scenarios pass.

## ASCII UI preview

UI-01: Existing theme import review, matching [the plan](plan.md#ascii-ui-preview).

```text
Desktop dialog / phone review drawer
[Theme file] [Base appearance] -> [Review theme]
Identity + preview + compatibility report
[Close] [Review theme] [Install themes]
```

Preparation now reaches this existing review instead of rejecting an unscoped
default. Preserve the current desktop dialog and phone drawer, internal scroll
owner and safe-area actions. AC-PLUGINS-THEMES-002.1 and 003.5 own these outcomes.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/plugins/themes -run TestConvertVSCode -count=1)
(cd apps/backend && go test -trimpath ./internal/plugins -run TestThemeReview -count=1)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/settings/application-themes.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/settings/mobile-application-themes.spec.ts)
```

Write `TestConvertVSCodeUnscopedDefaults` first and confirm the existing
`theme_scope` failure before correcting production conversion.

## Files likely touched

- `apps/backend/internal/plugins/themes/{vscode.go,vscode_test.go}`
- `apps/backend/internal/plugins/theme_reviews_test.go`
- `apps/web/e2e/tests/settings/theme-flow.ts`
- `docs/public/appearance.md` if needed to clarify supported import behavior

## Dependencies

None. Run after Task 01 in the primary conversation.

## Risks

Do not infer metadata or change source-derived identity. Report unsupported
style fidelity honestly; do not silently erase authored defaults.

## Parallelism

`sequential`

## Inputs

The package design's VS Code conversion section; strict canonical schema;
existing converter, review and desktop/phone import tests.

## Results

- Red: `TestConvertVSCodeUnscopedDefaults` and explicit-color precedence cases
  failed with `theme_scope` before the converter change.
- Green: absent/empty scopes map validated editor defaults; later defaults win,
  explicit editor colors retain precedence, scoped styles remain intact and
  unsupported default font styles are reported. Invalid colors/styles remain
  rejected even when an explicit editor color would override them.
- `TestConvertVSCode*`, reviewed-import tests and the full theme/plugin suites
  passed. The shared browser fixture includes unscoped defaults and explicit
  editor colors; desktop import/save/recovery and phone import/save checks passed.
- The final combined verification is recorded in Task 04.
