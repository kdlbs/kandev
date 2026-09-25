---
status: active
system: agents
created: 2026-09-08
owners:
  - kandev
---

# Configured fallback summary requirements

## Overview

Agent profile rows on the Agents settings page expose the fallback policy saved
for each profile. The summary lets users distinguish executor-default behavior,
exact-model selection, automatic selection of the next model, and a configured
explicit fallback without opening the profile editor.

## Terminology

- **Exact-model state:** `require_exact_model` is true. Saved automatic or
  explicit fallback values are dormant while this policy is enabled.
- **Executor-default state:** `require_exact_model` is false, `auto_fallback` is
  false, and `fallback_model` is empty. The executor may use its default model.
- **Automatic fallback state:** `require_exact_model` is false and
  `auto_fallback` is true. The configured explicit fallback value is ignored by
  runtime precedence.
- **Explicit fallback state:** `require_exact_model` is false,
  `auto_fallback` is false, and `fallback_model` is non-empty.

## Requirements

### REQ-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001: Show the configured fallback policy

**Intent:** Make the saved fallback policy visible in the profile list.

**User story:** As a Kandev user, I want each agent profile row to show its
configured fallback policy, so that I can understand how an unavailable start
model will be handled without opening every profile.

#### Acceptance criteria

- **AC-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001.1:** When a profile row is shown
  on Settings > Agents, it shall render a fallback pill immediately after the
  model pill, with `fallback: exact` for the exact-model state, `fallback: none`
  for the executor-default state, `fallback: next` for the automatic fallback
  state, or `fallback: <configured model>` for the explicit fallback state.
- **AC-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001.2:** The fallback pill shall use
  the profile's saved `require_exact_model`, `auto_fallback`, and
  `fallback_model` values, and shall not change those values or infer runtime
  availability.
- **AC-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001.3:** The fallback summary shall
  remain readable and wrap with the existing profile metadata on phone-sized
  screens without introducing document-level horizontal overflow.
- **AC-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001.4:** The fallback summary shall
  be localized through the Agents translation namespace while preserving the
  configured model identifier as opaque product data.

## Out of scope

- Changing runtime fallback precedence or model selection.
- Editing profile fallback settings from the profile list.
- Showing executor availability, runtime effective models, or fallback warnings.
