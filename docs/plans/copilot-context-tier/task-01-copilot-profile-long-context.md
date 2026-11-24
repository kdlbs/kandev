---
id: "01-copilot-profile-long-context"
title: "Add the Copilot long context profile preset"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-CONTEXT-TIER-001
acceptance_criteria:
  - AC-AGENTS-COPILOT-CONTEXT-TIER-001.1
  - AC-AGENTS-COPILOT-CONTEXT-TIER-001.2
  - AC-AGENTS-COPILOT-CONTEXT-TIER-001.3
system_design:
  - ../../specs/agents/system-design/copilot-context-tier.md
---

# Task 01: Add the Copilot long context profile preset

## Summary

Expose an optional launch flag in the existing profile editor. Keep
provider model discovery and session lifecycle on the base implementation.

## In scope

Add the disabled preset to the Copilot agent catalogue. Cover new profile
defaults in Go and validate toggling, saving, and command preview on desktop
and phone.

## Out of scope

Context tier overrides in Task Chat and workflows. Model eligibility and
context size reporting remain with Copilot.

## Acceptance

A user can enable the flag in a Copilot profile and see it in Command
Preview. The saved setting persists after reload. Copilot launch uses the
existing enabled CLI flag list.

## ASCII UI preview

UI-01 from the [plan](plan.md#ascii-ui-preview) appears in the existing
profile settings scroll region on desktop and phone. It covers
AC-AGENTS-COPILOT-CONTEXT-TIER-001.1 and AC-AGENTS-COPILOT-CONTEXT-TIER-001.2.

```text
CLI Flags
Long context                               [off]
--context long_context
```

## Verification

- From `apps/backend`, run `go test -trimpath ./internal/agent/agents ./internal/agent/settings/controller ./internal/agent/registry -count=1`.
- From `apps/web`, run `pnpm e2e:run --host e2e/tests/settings/copilot-profile-long-context.spec.ts -- --retries=0`.
- From `apps/web`, run `pnpm e2e:run --host --project mobile-chrome e2e/tests/settings/mobile-copilot-profile-long-context.spec.ts -- --retries=0`.

## Files likely touched

The Copilot agent catalogue, mock Copilot catalogue, profile controller
tests, browser profile tests, and public profile documentation.

## Dependencies

The existing profile CLI flag catalogue, storage, command builder, and
command preview.

## Results

The focused Go packages passed. Desktop Chromium and mobile Chrome each
passed one profile flow with retries disabled. Both used the freshly built
artifacts with `--no-build` and verified that the enabled flag appears in
Command Preview and survives a save and reload. The CLI Flags component
tests, web type check, locale checks, and documentation validators passed.
