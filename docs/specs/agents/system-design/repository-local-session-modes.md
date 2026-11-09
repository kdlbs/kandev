---
status: current
system: agents
created: 2026-11-02
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

## Settings and persistence

`ProfileCapabilitiesSection` passes the profile draft and discovered mode list
to `ModeCombobox`. The picker accepts a typed ID and shows a saved unmatched ID.
It remains reachable for an empty ready catalog and stays disabled when the
existing discovery readiness guard requires it. Closing the popover clears the
local filter. The profile form owns the draft, and the existing profile update
stores the mode in `agent_profiles.mode`. No new API or database field is added.

Desktop and phone use the existing settings controls and localized labels. The
phone layout keeps the mode control in the settings scroll owner and preserves
touch sizing and horizontal containment.

## Probe and session lifecycle

`ACPInferenceExecutor.probeACPSessionWithContext` reads the legacy mode state
and the ACP mode-category select in `session/new`. A present advertisement is
authoritative for the scratch session, even when its list is empty. The probe
applies a requested ID in that list and skips one outside it. If neither carrier
is present, it retains the prior attempt-and-error behavior. Grouped option
choices are flattened using the existing ACP helper. A skipped mode leaves the
host session at its default; the response must not be interpreted as proof of
repository-mode availability.

The existing session lifecycle applies an explicit mode in the task executor
and checks its confirmed effective value before the first prompt. A rejection,
missing confirmation, or mismatch fails startup. No profile value is rewritten
by host discovery, and this design adds no permission-mode fallback.

## Verification

ACP probe tests cover both mode carriers, absent and empty advertisements,
grouped choices, and rejected application. Component tests cover selection and
filter reset. Profile E2E tests cover desktop persistence and the empty-catalog
phone flow. Existing lifecycle tests cover mode confirmation before prompting.
