---
id: "03-notes-companion"
title: "Notes companion migration"
status: done
wave: 3
depends_on: ['02-caller-integration']
plan: "plan.md"
requirements:
  - REQ-PLUGINS-EXPLICIT-UTILITY-001
acceptance_criteria:
  - AC-PLUGINS-EXPLICIT-UTILITY-001.2
  - AC-PLUGINS-EXPLICIT-UTILITY-001.4
  - AC-PLUGINS-EXPLICIT-UTILITY-001.7
  - AC-PLUGINS-EXPLICIT-UTILITY-001.9
  - AC-PLUGINS-EXPLICIT-UTILITY-001.10
  - AC-PLUGINS-EXPLICIT-UTILITY-001.11
system_design:
  - ../../specs/plugins/system-design/plugin-explicit-utility-invocation.md
---

# Task 03: Notes companion migration

## Summary

Migrate the Notes plugin in yattdev/kandev-plugin-notes to load its saved selection and send the override explicitly.
Keep the host PR and companion delivery records separate.

## In scope

- Inspect current Notes source and repository instructions; reference files were verified at f8548f9d7a1eaabf19d27bb0b0af8f75d8eaeec4.
- Use the implemented host SDK through the existing sibling-checkout replace in go.mod. Record the host revision and refresh checksums only if needed.
- Read agent_profile through GetConfig, pass the selection explicitly, and omit it when empty. Stop on configuration read/type errors.
- Keep the existing optional agent_profile field and control; the inspected manifest has no required list.
- Update host fakes, manifest tests, README, and existing enhancement tests for default, explicit, invalid selection, and unsupported host.
- Package and check the companion against the revised host. Record the exact SDK pin and companion revision.
- Recheck the live companion PR before delivery; do not assume historical PR #7 is still open or the latest delivery target.

## Out of scope

- General agent execution, cwd, session APIs, profile eligibility changes, and unrelated UI redesign.
- Work owned by other tasks. Do not revert their changes or the prior hydration and Windows CI fixes.

## Acceptance

- Notes sends a persisted override with its enhancement prompt, while empty preference uses the platform default.
- Invalid selection and configuration errors never silently switch profiles.
- Companion tests and package smoke pass against the recorded host revision; deployment dependencies are explicit.

## Verification

Use TDD for changed logic. Commands below run from the host repository root unless stated otherwise.

```bash
# Run from the Notes repository root, not the Kandev monorepo.
go test ./...
node --test ui/bundle.test.mjs
make vet
make package-host
git diff --check
```

The verified Notes README expects a sibling checkout named kandev for its local SDK replace.
Use a separate clean checkout at the implemented host revision for packaging if the current task path cannot satisfy that layout.
For the package smoke, install the resulting host-platform tarball into an isolated revised host.
Exercise Enhance with AI using default A, explicit B, and deleted B. Record host/plugin revisions and outcomes; never use real notes or paid agents.

## Files likely touched

- `yattdev/kandev-plugin-notes: server/plugin.go`
- `yattdev/kandev-plugin-notes: server/plugin_test.go`
- `yattdev/kandev-plugin-notes: server/manifest_test.go`
- `yattdev/kandev-plugin-notes: manifest.yaml`
- `yattdev/kandev-plugin-notes: go.mod`
- `yattdev/kandev-plugin-notes: go.sum`
- `yattdev/kandev-plugin-notes: README.md`

## Dependencies

02-caller-integration.

## Risks

- This task requires a separate repository and a buildable SDK pin. It must not be represented as completed by host CI.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/plugins/requirements/plugin-explicit-utility-invocation.md) and [design](../../specs/plugins/system-design/plugin-explicit-utility-invocation.md).
- [Plan](plan.md), including compatibility rules, evidence matrix, and delivery instructions.
- Existing SDK wire tests, host utility tests, and plugin-fixture patterns. Read changed source and local AGENTS.md before editing.

## Results

Implemented in the separate `yattdev/kandev-plugin-notes` checkout. The Notes
Go suite passed, all 96 UI tests passed, `make vet` passed, `make package-host`
passed, and `git diff --check` passed. The companion commit and PR head are
recorded after its separate push.
