---
created: 2026-09-14
status: complete
requirements:
  - REQ-PLUGINS-DIRECT-PROFILE-INVOCATION-001
system_design:
  - ../../specs/plugins/system-design/plugin-direct-profile-invocation.md
legacy_specs: []
---

# Implementation Plan: Plugin Direct Agent Profile Invocation

> Historical delivery record. [The replacement plan](../plugin-explicit-utility-invocation/plan.md) supersedes implicit host selection.
> Previous completed checks do not verify the replacement API.


## Overview

Plugins that delegate a one-shot LLM step currently select execution identity
through the legacy `utility_agent` config field, which models the choice as a
custom utility prompt and hides the agent profile that actually executes.
This plan adds a direct `agent_profile` selection to the Host invocation
contract, wires the host to execute the selected profile via
`ExecuteProfilePrompt`, and keeps declaration-gated legacy behavior
untouched. The companion [kandev-plugin-notes PR 7](https://github.com/yattdev/kandev-plugin-notes/pull/7)
consumes the new `format: agent-profile` field at exact Notes head
`f8548f9d7a1eaabf19d27bb0b0af8f75d8eaeec4`.

The smallest reproduction of the addressed defect is:

1. Open Settings > Plugins > Notes. The Utility Agent field lists rows from
   the utility/custom-prompt catalog, not the platform's agent profiles.
2. Selecting a row persists a utility-prompt ID that fails at invocation with
   a generic error rather than executing the configured agent profile.

## Scope

### In scope

- The `agent-profile` config renderer format, shared-profile-picker reuse,
  and eligibility filtering in the web app.
- Direct profile routing, typed `FailedPrecondition` translation, and legacy
  fallback bounding in the backend plugins host.
- Hydration merging in the plugin config form and store profile-list
  hydration revision checks.
- Accessibility and display-label reload for the picker.
- The companion Notes manifest, webhook error translation, tests, docs, and
  packaging evidence for the Notes half of the delivery.

### Out of scope

- Changing the public `Host.InvokeUtilityAgent` signature or `agent_invoke`
  gate semantics.
- Changing utility-agent resolution for plugins that declare only
  `format: utility-agent`.
- New database or config migration for the dual selector formats.

## Technical approach

### Settings rendering

Extend `config-schema.ts` with the `agent-profile` format renderer and route
it through `utility-agent-profile-picker.tsx` in `agentRegistration` mode.
Eligibility requires enabled, non-deleted, global, non-CLI-passthrough,
inference-capable profiles; unavailable saved IDs render replaceably.
`plugin-config-form.tsx` merges hydration results so concurrent profile
snapshot and config updates do not overwrite each other, and
`use-settings-data.ts`/`hydrator.ts` guard store revisions against stale
profile snapshots with WebSocket-triggered retry.

### Host execution

`host_utility.go` gates on `agent_invoke` first, validates the declared
format, resolves the profile once at call start, and executes with
`ExecuteProfilePrompt(profileID, prompt)`. Missing/ineligible selections
return typed `FailedPrecondition` before runner start; non-not-found store
errors pass through unchanged in `services.go`. Legacy `utility_agent`
routing runs only when no direct selection exists and the active manifest
declares that selector, per
[ADR 0048](../../decisions/0048-plugin-host-utility-agent-invoke.md).

### Notes companion

The plugin uses the daemonized target server, refreshes
`Host.InvokeUtilityAgent` enhancement to send the selected profile ID,
translates `FailedPrecondition` to an actionable HTTP 412 setup message,
updates its manifest contract test, README, packaging, and test-tooling
pins, and carries final visual QA evidence for the picker, mobile settings,
and enhancement preview.

## Tests

- Backend focused Go tests for direct routing, eligibility/error classes,
  capability gating, and legacy fallback preservation.
- Frontend unit tests for config-schema formats, picker eligibility and
  required fields, hydration merge, store snapshot revision, and WS retry.
- Notes Go tests for the manifest contract, webhook error mapping, and
  enhancement payload, plus `node --test ui/bundle.test.mjs` wiring smoke.

## Work orders

- [x] [Task 01: Direct profile invocation and settings rendering](task-01-direct-profile-invocation.md)
- [x] [Task 02: Notes companion manifest, errors, and evidence](task-02-notes-companion-delivery.md)

## Verification results

- Backend: focused `internal/plugins` and `internal/backendapp` Go tests
  passed, including the direct-profile routing, eligibility, cancellation,
  and store-failure cases; static checks passed.
- Frontend: config-schema and picker tests, plugin-config-form hydration
  tests, settings hydration and WS agent-handler tests, web typecheck, and
  lint all passed at CI for the delivery head.
- Notes: Go test suite, `node --test ui/bundle.test.mjs`, package smoke, doc
  validation, and packaging metadata checks passed.
- Notes PR evidence and committed screenshots confirmed the picker, mobile
  layout, and enhancement preview behavior.

## Risks

- Concurrent profile-list hydration and plugin-config hydration can clobber
  state; merge and revision guards prevent lossy overwrites.
- Legacy stored `utility_agent` values must remain executable for trusted
  pre-transition manifests, so fallback gating is explicitly bounded in ADR
  0048 and regression-tested.
- Config failures must not be misclassified as operational failures, or web
  hooks will mislead operators with wrong HTTP status messages.
