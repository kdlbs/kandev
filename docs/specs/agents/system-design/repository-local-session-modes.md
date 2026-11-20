---
status: current
system: agents
created: 2026-11-02
updated: 2026-11-18
requirements:
  - REQ-AGENTS-REPOSITORY-LOCAL-MODE-001
owners:
  - kandev
---
# Repository-local session modes system design

## Scope and authority

REQ-AGENTS-REPOSITORY-LOCAL-MODE-001 covers the profile editor, sessionless host
probe, and executor startup. The host utility in
[ADR 0002](../../../decisions/0002-host-utility-agentctl-for-sessionless-flows.md)
uses a scratch working directory. Its capability snapshot is advisory for a
task running in a different repository or executor.

| Acceptance criteria | Design section |
| --- | --- |
| AC-AGENTS-REPOSITORY-LOCAL-MODE-001.1, .5, .6, .7 | Settings and persistence |
| AC-AGENTS-REPOSITORY-LOCAL-MODE-001.2, .3, .4, .7 | Probe and session lifecycle |

## Settings and persistence

`ProfileCapabilitiesSection` passes the profile draft and discovered mode list
to `ModeCombobox`. It displays the provider's name and saves the exact ID from
the catalog. The picker accepts a typed ID and shows a saved unmatched ID.
It remains reachable for an empty ready catalog and stays disabled when the
existing discovery readiness guard requires it. Closing the popover clears the
local filter. The profile form owns the draft, and the existing profile update
stores the mode in `agent_profiles.mode`.

If a user-saved legacy ID falls outside the current catalog, it appears as a
custom entry. The user can reselect an advertised choice and save the profile.
ACP defines each vocabulary independently. A match in labels, ordering, or
current defaults is insufficient evidence of equivalent permissions.

Desktop and phone use the existing settings controls and localized labels. The
phone layout keeps the mode control in the settings scroll owner and preserves
touch sizing and horizontal containment.

## Probe and session lifecycle

`applySessionProbeFields` projects the first ACP `select` with category `mode`
into `ProbeResponse.Modes`. It uses exact option values as IDs and flattens
grouped choices in provider order. `CurrentModeID` uses the select's current
value if the same option advertises that value. This gives profile defaults and
settings the same catalog. A present empty select yields no advertised choices.
The legacy mode state supplies the catalog when a mode select is absent.
[ACP v1 session config options](https://agentclientprotocol.com/protocol/v1/session-config-options)
defines this precedence when both carriers are present.

`ACPInferenceExecutor.probeACPSessionWithContext` uses that precedence for
requests. It sends a matching config value with the option ID through
`session/set_config_option`. It uses the complete RPC response or a later
config update for this session to confirm the value and resolve model options.
If that RPC reports method not found, the exact same ID must be present in
legacy modes before the probe can use `session/set_mode`. Other RPC errors
fail the probe.

When no mode select exists, the probe applies a matching legacy ID through
`session/set_mode`. If both carriers are absent, it attempts the legacy RPC.
When the selected carrier is present and omits the requested value, the probe
skips the mode. The resulting snapshot describes the scratch session's context.

The task executor checks the selected mode against the active session catalog.
A rejection, missing confirmation, or mismatch blocks its first prompt.
The same exact config value can also reach a profile-backed utility prompt.
[Session startup and confirmation](agent-permission-control-integrity.md#initial-mode-delivery),
[utility permissions](utility-agent-profiles.md#permissions).

## Verification

ACP probe tests cover both mode carriers, distinct values, absent and empty
advertisements, grouped choices, and rejected application. Component tests
cover selection and filter reset. Profile E2E tests cover custom ID persistence
and the config value through the first prompt on desktop and phone.
Lifecycle tests cover mode confirmation before prompting. The
[fix plan](../../../plans/acp-profile-mode-discovery/plan.md) maps these checks
to the work order.
