---
created: 2026-11-03
status: in-progress
requirements:
  - REQ-AGENTS-DEVIN-PERMISSIONS-001
system_design:
  - ../../specs/agents/system-design/devin-profile-permissions.md
legacy_specs: []
---

# Implementation Plan: Devin profile permissions

## Overview

Add predefined Devin permission controls with Smart as the new-profile default.
Reuse the ACP Start mode selector and add a sandbox override beside the extra settings.
Keep the first-draft discovery work orders as prerequisites within the combined delivery package.

This package implements the user's permission-settings request, not another cause of issue #5023.
The [combined Devin plan](../first-draft-profile-discovery/plan.md) now also owns issue #5026's grouped/composed selector work.
Complete its Tasks 01/02 before this companion's implementation and follow its documented cross-package order.
Keep modes and sandbox controls separate from Fusion model dimensions and provider Thinking/Speed options.
Implementation was explicitly requested after the design review; the results below record what was delivered and what remains open.

## Scope

### In scope

- Smart defaults for new UI, API, and first-provisioned profiles.
- Supported Smart and Bypass selections through existing ACP mode authority.
- Enabled, disabled, and inherited sandbox overrides, with Autonomous coupling.
- Saved-profile compatibility, localized UI, public docs, and desktop/phone acceptance.

### Out of scope

- Retroactive changes to existing profiles or sessions.
- New independent boolean Smart/Bypass CLI flags.
- An automatic prerequisite installer, new provider protocol, or new persistence field.
- A claim that saved ACP mode configures terminal permission behavior.
- Devin's own permission rules, network policy, account rollout, or organization policy.

## Technical approach

### Defaults and authority

Add a narrow backend default/validation helper in `settings/profileconfig/devin.go`.
Call it from new-profile creation and first default provisioning.
Mirror new-draft cases in `apps/web/lib/devin-profile-settings.ts`.
Keep default seeding out of PATCH, duplication, and existing-profile synchronization.

For new create requests, omitted/null/empty mode means Smart unless an explicit enabled sandbox requires Autonomous.
Preserve explicit non-empty modes and explicit sandbox values.
Add a false sandbox override only when new creation inputs omit one.
The existing false auto-approval default remains authoritative.

Do not append permission flags to `devin acp`.
The official `3000.11.234` binary rejects that argument position.
Global flags before `acp` parsed but did not select the current ACP mode in the investigation.
Use generic ACP mode application and its provider confirmation instead.

### Sandbox and editor

Bind a new `DevinSandboxPreference` to `env_vars.DEVIN_SANDBOX`.
The selector exposes Use CLI settings, Disabled, and Enabled (Autonomous).
Changing it updates only the reserved entry and its coupled mode.
An enabled setting selects Autonomous, marks discovery stale, and requires matching refreshed capabilities.
Invalid or unsupported combinations stay visible and never retry without sandbox.

When Smart is not advertised, retain the requested value and show an actionable unsupported state.
Do not insert a fake selectable Smart capability or silently choose Code/Accept Edits.
Preserve the provider's Code, Ask, and Plan choices where advertised.

Use the same fields for discovery, model-option resolution, and task launch.
Keep Kandev auto-approval independent and explain its effect on remaining approval prompts.
The terminal view identifies the ACP mode control's scope and preserves native CLI permission behavior.

### Compatibility matrix

| Surface or capability | Authority | Intended behavior | Evidence and unsupported case |
| --- | --- | --- | --- |
| ACP advertises Smart | Generic session config/mode API | New profile prefers Smart and confirms it before inference | Fake ACP confirmation test and desktop/phone selectors |
| ACP omits Smart | Returned capability snapshot | Visible unavailable state and explicit alternative | Real CLI rejected an unavailable Smart selection with `-32602` |
| ACP Bypass | Advertised `bypass` mode | Explicit selection, no independent CLI switch | Real no-prompt config-option selection confirmed Bypass |
| Sandbox on Linux | Profile env and provider sandbox initialization | Requires provider prerequisites and Autonomous support | Real `DEVIN_SANDBOX=true` failed for missing `socat`, without an unsandboxed retry |
| Sandbox on macOS | Same env, provider Seatbelt implementation | Conditional support, same failure contract | Official docs. No macOS runtime proof in this session |
| Sandbox on native Windows | Same stored intent | Visible unsupported failure | Official docs. No silent fallback |
| CLI passthrough | Native CLI plus profile environment | Sandbox override applies. ACP Start mode has no terminal guarantee | Retain existing behavior and explicit scope help |
| Existing custom profile | Saved fields | No normalization on open, reload, duplication, or unrelated edits | Backend and frontend compatibility tables |

## ASCII UI preview

UI-DEVIN-01: Existing Devin profile form, new profile with Smart available.
The labels illustrate localized copy, not final catalog strings.
The existing settings page owns scrolling and Save.

```text
Desktop
Start model [ selected model v ]  Start mode [ Smart v ] [Refresh]

Extra settings
Devin sandbox        [ Disabled v ]
Enable to use Autonomous mode with Devin's sandbox.
Auto-approve all permissions                       [off]
Kandev can otherwise answer Devin's remaining approval requests.
```

UI-DEVIN-02: Phone composition, same new profile.

```text
Start model
[ selected model                 v ]
Start mode
[ Smart                          v ]

Extra settings
Devin sandbox
[ Disabled                       v ]
Enable to use Autonomous mode
with Devin's sandbox.

Auto-approve all permissions [off]
```

UI-DEVIN-03: Sandbox enabled or unavailable mode.

```text
Sandbox enabled:
Start mode       [ Autonomous      ]
Devin sandbox    [ Enabled        v ]
[Refresh models]

Smart unavailable:
Start mode       [ Smart (unavailable) v ]
Smart is unavailable. Choose an advertised mode or refresh.

Sandbox initialization failed:
Devin sandbox    [ Enabled        v ]
Sandbox could not start. Review the required prerequisites.
[Retry]  [Edit settings]
```

Structural requirements: one mode authority, a distinct sandbox override, preserved error state, and reachable actions.
Phone controls have at least 44px targets. Desktop controls retain 28px density.
UI-DEVIN-01/02 cover AC-AGENTS-DEVIN-PERMISSIONS-001.1, 001.2, 001.7, and 001.8.
UI-DEVIN-03 covers 001.3, 001.4, and 001.9.

## Tests

| Acceptance criteria | Planned evidence |
| --- | --- |
| 001.2, 001.6 | `TestDevinProfileDefaults`: UI/API/bootstrap, omitted/null/empty creation, explicit values, existing synchronization, PATCH clear, and exact duplication |
| 001.1, 001.3 | `devin-profile-settings.test.ts` and mode-selector component tests, with Smart present and absent |
| 001.4, 001.5 | `TestDevinSandboxProfileContext` and `TestDevinPermissionModeLaunch`: actual child env, confirmed requested mode, unsupported and missing-prerequisite failures |
| 001.6, 001.9 | Helper and component tests for unrelated entries, custom values, round trips, failure retention, and explicit clear |
| 001.7 | Profile auto-approval tests and contextual help. A fake provider request still reaches the existing approval path when auto-approval is false |
| 001.8, 001.9 | Desktop and phone E2E save/reload, keyboard/touch, geometry, and no horizontal overflow |

## E2E tests

Add `tests/settings/devin-profile-permissions.spec.ts` and `mobile-devin-profile-permissions.spec.ts`.
Use an isolated disposable mock ACP descriptor with Devin's settings identity.
Extend the established E2E registry pattern only under `KANDEV_E2E_MOCK=true`.
Do not add Devin to the Office routing-provider list merely to seed settings tests.
The fake peer must expose Smart-present, Smart-absent, sandbox-Autonomous, and sandbox-failure snapshots.
Do not require provider credentials or a live Devin installation in CI.

Assert Smart defaults, Bypass save/reload, Autonomous coupling, missing-prerequisite failure retention, and explicit unsupported-mode handling.
Keep a legacy empty-mode/inherited-sandbox case and an independent auto-approval case.
Use real profile save APIs. Fixture capability responses must not replace backend default and environment-boundary tests.

## Work orders

- [ ] [Task 01: Default and execute Devin permission intent](task-01-permission-intent.md) (lifecycle acceptance open)
- [ ] [Task 02: Expose and prove Devin settings](task-02-permission-controls.md), after Task 01 (controls done; browser proof open)

Execution is sequential in the primary session.
Task 01's controller defaults/validation are implemented, but its lifecycle mode-confirmation acceptance remains open; Task 02 controls are implemented with desktop/phone flows, and its conditional capability scenarios remain partially open.
The combined plan's final acceptance follows both packages and does not introduce a dependency cycle.

## Investigation and verification results

Investigation used a checksum-verified official Linux CLI `3000.11.234` in disposable storage.
`--help` parsing, ACP initialize/session-new, mode selection, and the missing-socat failure ran without prompts or inference.
Modes and Smart availability differed across fresh contexts.
No inference or successful sandbox workload ran, and no macOS/Windows executable ran.
Temporary runtime data and raw frames are not repository artifacts.

Implementation completed 2026-11-03: backend defaults/validation, the sandbox control, localization, unit/component tests, and desktop/phone browser flows. The mock peer still lacks deterministic Smart-absent and sandbox-failure scenarios, so those branches are covered only by unit/controller tests.

Design-package validation on 2026-11-03:

- `python3 scripts/list-docs.py validate`: passed, 428 decisions and 1895 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: a simulated backend/frontend diff passed as covered with all four companion/repair work orders.
  This checks requirement, acceptance, plan, and design references. It does not validate production implementation.
- `git diff --check -- docs/specs/agents docs/plans/first-draft-profile-discovery docs/plans/devin-profile-permissions`: passed.
- Production, permanent tests, and live user configuration remain unchanged.

All design artifacts remain unstaged and uncommitted.
The later #5026 integration update passed catalog/specification checks and the combined nine-work-order reference preflight.
Its current source of delivery order and completion gates is the combined plan.

## Documentation impact

Update `docs/public/agents-and-profiles.md` as a how-to/reference section during Task 02.
Include the official `curl -fsSL https://cli.devin.ai/install.sh | bash` command and the ACP mode/terminal distinction.
Document Smart availability, sandbox prerequisites, and independent Kandev auto-approval.
Retain the existing checked installer implementation.

## Risks

- A CLI flag can parse without configuring the ACP session. Require provider confirmation of the selected mode.
- Smart availability can vary within one CLI version. Use observed capabilities rather than a version allowlist.
- An absent sandbox override can inherit CLI policy. Do not render it as explicit Disabled.
- Kandev auto-approval can answer remaining provider prompts. Preserve its independent explicit authority.
- A host probe cannot establish remote executor sandbox readiness.
