---
created: 2026-11-18
status: completed
requirements:
  - REQ-AGENTS-REPOSITORY-LOCAL-MODE-001
  - REQ-AGENTS-UTILITY-AGENT-PROFILES-001
system_design:
  - ../../specs/agents/system-design/repository-local-session-modes.md
  - ../../specs/agents/system-design/utility-agent-profiles.md
legacy_specs: []
---

# Fix plan for ACP profile mode discovery

## Overview

Make the **Start mode** picker offer the values the ACP session control accepts.
Carry the selected value through model-option discovery and into the first task
or utility prompt. The Agents system owns the contract because it interprets
provider capabilities and applies saved profile modes.
[GitHub issue #5364](https://github.com/kdlbs/kandev/issues/5364).

## Root cause and reproduction

`applySessionProbeFields` takes legacy `modes` whenever the field exists.
The host capability cache and the saved-profile probe hand those IDs to the
settings editor. `ModeCombobox` correctly saves the ID it receives.
`selectSessionModeOption` checks that ID against the live mode config select
when the agent advertises one. A mismatch fails before the task sends a prompt.

The model-option probe recognizes values from both advertisements. It sends
either value through legacy `session/set_mode`. The one-shot executor also uses
that RPC for all explicit modes. Selecting the config value alone would leave
those callers with the wrong RPC.

The current picker already accepts a typed ID. A user can enter a known config
value, but the model-option probe can still fail through the legacy RPC.

The smallest reproduction supplies both fields in `session/new` over an ACP
pipe. The legacy list contains `dont_ask` and the mode config select
`edit_approval_policy` contains `session`. Temporary Go overlays exercised the
real probe, utility executor, and live adapter at `40e18c4b577`.

| Check | Observed behavior |
| --- | --- |
| `TestIssue5364ProbePublishesConfigModeValues` | Expected failure. The probe returned `default`, `accept_edits`, and `dont_ask`. |
| `TestIssue5364ProbeSendsExactConfigModeValue` | Expected failure. The probe sent `session` to `session/set_mode`. |
| `TestIssue5364UtilityUsesTypedProfileMode` | Expected failure. A config-only fixture rejected the utility's legacy RPC. |
| `TestIssue5364LaunchRequiresExactConfigModeValue` | Passed. The live adapter rejected `dont_ask` and confirmed `session` through the config RPC. |

## Scope

- Project mode choices from the ACP host probe for agent-wide and profile
  capability responses.
- Apply the same mode selection rules in scratch probes and utility execution.
- Verify profile selection, persistence, and task startup on desktop and phone.
- Document how to recover saved profile IDs outside the advertised catalog.

## Technical approach

Use one rule in the existing utility package. If the provider offers a mode
config select, publish its choice names and values. Otherwise, publish the
legacy mode list. Keep the existing response fields and profile column.
Publish the config default only if the provider lists it as a choice.

Route the two existing utility callers through the same ACP mode request.
Reuse the existing config setter and bounded snapshot wait for a typed mode.
Each caller keeps its mode and model order. A scratch probe can skip an
unadvertised selection. A utility prompt requires confirmation for a successful
typed request. The live task adapter already sends and confirms the exact
config value. [Session startup design](../../specs/agents/system-design/agent-permission-control-integrity.md#initial-mode-delivery).

### ACP compatibility

| Advertisement | Profile editor and scratch probe | Task and utility execution | Evidence |
| --- | --- | --- | --- |
| Mode select plus a disjoint legacy list | Show the config choices. Apply an exact config value. Keep an old ID visible as a custom entry. | Send the selected config value. Reject an unmatched explicit mode. | Dual-carrier fixture and desktop and phone launch. |
| Config-only mode select | Use the advertised config choices and the config RPC. | Use the config RPC. | Grouped choice and one-shot wire tests. |
| Legacy list only, including with unrelated config options | Show legacy names and IDs. Apply matching IDs through the legacy RPC. | Use the existing legacy path. | Legacy probe and lifecycle tests. |
| Present mode select with zero choices | Offer custom entry in the ready editor. Skip explicit modes in the scratch probe. | Reject an explicit value absent from the active select. | Empty select combined with a nonempty legacy list. |
| Neither mode carrier | Offer custom entry. Attempt the legacy RPC if the probe receives a requested mode. | Apply the existing behavior of each caller. | Absent-advertisement probe test. |
| Config RPC reports method not found | Use the legacy RPC only if legacy modes advertise the same exact ID. | Preserve exact-ID fallback and each caller's error behavior. | Same-ID and disjoint-ID wire tests. |

The two ACP fields can use different vocabularies. A user-saved `dont_ask` is
retained as written until the user saves a different mode. The existing
custom-entry flow supports repository-local values and lets the user recover
by choosing the new advertised row.
[ACP v1 session config options](https://agentclientprotocol.com/protocol/v1/session-config-options).

## ASCII UI preview

**UI-01. Profile settings with discovery ready**

Settings > Agents > agent > profile > Start mode. This example uses the
provider's **Ask** and **Don't Ask** labels.

```text
Desktop
Start model [Fixture Model v]   Start mode [Ask v]   [Refresh icon]
                                          [Search modes...]
                                          Ask (Default)
                                          Accept Edits
                                          Don't Ask

Phone
Start model [Fixture Model v] [Refresh icon]
Start mode  [Ask                        v]
            [Search modes...             ]
            Ask (Default)
            Accept Edits
            Don't Ask
```

The same anchored picker opens on touch and keyboard. The phone mode field sits
under the model and refresh row. The settings page and the open list keep their
existing scroll regions. The trigger fits the viewport and retains its 44px
phone target. Provider labels can vary. The saved value for **Don't Ask** in
this example is `session`. When opening a profile with `dont_ask` still saved,
the trigger displays `dont_ask` and the list also contains its custom row.
See AC-AGENTS-REPOSITORY-LOCAL-MODE-001.1, .5, .6, and .7.

## Tests

- AC-AGENTS-REPOSITORY-LOCAL-MODE-001.2, .3, and .7.
  `acp_profile_mode_test.go` reproduces the wrong catalog and RPC at the ACP
  boundary. Keep legacy, empty, grouped, and missing carrier cases in
  `acp_probe_capabilities_test.go`.
- AC-AGENTS-REPOSITORY-LOCAL-MODE-001.4.
  `TestLegacyConfirmedModeLifecycle` and
  `TestInitializeAndPromptWithLayers_AppliesOnlyWinningModeBeforePrompt`
  exercise task startup. Existing `TestSetMode*` tests cover refusal and
  confirmation through the live adapter.
- AC-AGENTS-REPOSITORY-LOCAL-MODE-001.1, .5, and .6.
  `mode-combobox.test.tsx` and the existing custom-mode E2E flows cover
  the picker fallback and search reset. Keep the existing test scenarios.
- AC-AGENTS-UTILITY-AGENT-PROFILES-001.7.
  `TestIssue5364UtilityUsesTypedProfileMode` proves that the selected mode
  reaches a first utility prompt. Add a clamp or missing-confirmation case
  that proves the prompt stays blocked.

## E2E tests

- `chromium`, `tests/settings/agent-profile-acp-mode.spec.ts`.
  Choose an advertised `Don't Ask` row by keyboard, save, reopen, and launch the
  task from that profile. Check the persisted `session` value and the fixture's
  mode on receipt of the first prompt. Covers AC-AGENTS-REPOSITORY-LOCAL-MODE-001.4,
  .5, .6, and .7.
- `mobile-chrome`, `tests/settings/mobile-agent-profile-acp-mode.spec.ts`.
  Start with `dont_ask` saved, confirm it appears as a custom ID, then replace it
  by touch. Check the saved value, first prompt, trigger size, and horizontal
  containment. Covers AC-AGENTS-REPOSITORY-LOCAL-MODE-001.1, .4, .6, and .7.

Use a local custom ACP fixture with disjoint values. The tests use the real
profile APIs, settings UI, host probe, and task executor. The closest phone
precedent is `mobile-profile-custom-mode.spec.ts`.

## Work orders

- [x] [Task 01. Align profile mode discovery and application](task-01-align-profile-mode.md)

## Verification results

The new ACP regressions failed before the implementation and pass after it.
The complete utility package, focused adapter and lifecycle checks, and a race
run of the new regressions pass. Desktop and mobile profile E2E tests pass, and
their managed runner build succeeded. Go lint, public documentation validation,
specification validation and lint, and `git diff --check` pass.

The static review follow-up scopes config notifications to the active ACP
session, applies catalog snapshots returned after model and config mutations,
and lets utility execution continue after a successful legacy mode ACK. The
updated desktop and mobile E2E tests pass with argv-safe fixture paths and
successful resolve-response assertions. The full utility package, focused race
check, and changed-file Go lint pass.

## Risks

- A previously saved legacy ID can require manual reselection before a task or
  utility call starts. The profile guide will document that recovery.
- A different repository, runtime version, model, or remote executor can
  advertise different choices. The active session stays authoritative.

## Public documentation

During implementation, update the reference in **Create and configure a profile**
with config choice selection and saved-mode recovery. Use the existing
**Start mode** label. [Agents and profiles](../../public/agents-and-profiles.md#create-and-configure-a-profile).
