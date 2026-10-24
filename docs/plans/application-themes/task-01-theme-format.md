---
id: "01-theme-format"
title: "Theme format and package validation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-THEMES-001
  - REQ-PLUGINS-THEMES-002
acceptance_criteria:
  - AC-PLUGINS-THEMES-001.1
  - AC-PLUGINS-THEMES-001.2
  - AC-PLUGINS-THEMES-001.3
  - AC-PLUGINS-THEMES-001.4
  - AC-PLUGINS-THEMES-002.1
  - AC-PLUGINS-THEMES-002.2
  - AC-PLUGINS-THEMES-002.4
system_design:
  - ../../specs/plugins/system-design/theme-packages.md
---

# Task 01: Theme format and package validation

## Summary

Define the data-only native theme contract and supported VS Code conversion.
Make host validation and contributor pack/verify tools accept this strict
profile without weakening executable-plugin or canvas validation.

## In scope

Manifest themes/distribution kind, deep clone and absence-of-other-contributions
validation; canonical schema/normalization/default-role metadata; JSONC conversion
and compatibility report; bounded archive/checksum validation and format fixtures;
plugin-pack and plugin-package-verify support with expected kind output.
Add at most the maintained JSONC parsing dependency named in the design after
checking existing module capabilities; do not handwrite a JSONC parser.

## Out of scope

Lifecycle activation, HTTP import, marketplace publication, browser selection,
renderer implementation and any production theme repository.

## Acceptance

1. A data-only two-variant pack can be packaged/verified on any host platform;
   mixed runtime/UI/capability files, unsafe paths, excessive sizes and unsupported
   schemas fail before extraction. Existing binary and canvas tests remain valid.
2. Native and supported VS Code inputs normalize deterministically, with schema/
   role mapping/report fixtures proving defaults, explicit base, unknown fields,
   duplicate names/keys, source attribution and rejected inheritance.
3. A machine-readable schema and reusable conformance fixture set define one
   public format; CLI and host use the same strict validator rather than a
   second registry-only parser.

## Verification

From repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/plugins/themes ./internal/plugins/manifest ./internal/plugins/pkgtar ./cmd/plugin-pack ./cmd/plugin-package-verify)
bash scripts/lint-go-changed apps/backend/internal/plugins/themes/*.go apps/backend/internal/plugins/manifest/*.go apps/backend/internal/plugins/pkgtar/*.go apps/backend/cmd/plugin-pack/*.go apps/backend/cmd/plugin-package-verify/*.go
git diff --check
```

Before lint, pass only files changed by this work order from the likely scopes
above if existing unchanged files produce unrelated lint noise. Record the exact
expanded file list. Runtime theme presentation is verified by later work orders.

## Files likely touched

- `apps/backend/internal/plugins/manifest/{manifest,validate,distribution,clone}.go`
  and new `themes.go`/`themes_test.go`, clone tests.
- `apps/backend/internal/plugins/themes/` (new schema, typed validator,
  normalization, portable-role and VS Code conversion tests, fixtures).
- `apps/backend/internal/plugins/pkgtar/pkgtar.go` and new `theme_test.go`.
- `apps/backend/cmd/plugin-pack/pack.go` and theme fixture tests.
- `apps/backend/cmd/plugin-package-verify/main.go` and theme profile tests.
- `apps/backend/go.mod`, `go.sum` only if JSONC needs a dependency.

## Dependencies

None. Read backend AGENTS and the package design; current installer assumes a
managed binary. Inspect canvas shared-integrity behavior before changing helpers.

## Risks

A broad static-package exception could accidentally admit runtime code. A second
parser could disagree with installer validation. Theme input may be malicious.

## Parallelism

`sequential`

## Inputs

Theme package design: Package profile, VS Code conversion, Primary references.
Requirements/criteria listed above; existing archive and packer tests.

## Results

Implemented strict theme manifests, canonical JSON Schema validation, VS Code JSONC conversion, bounded package verification/install and contributor CLI kind checks.

- Listed five-package `go test -trimpath` command: passed.
- Narrow `scripts/lint-go-changed` across the changed manifest, themes, pkgtar and CLI files: passed (0 issues).
- Theme lifecycle, HTTP and cross-language renderer conformance are exercised in subsequent work orders.
