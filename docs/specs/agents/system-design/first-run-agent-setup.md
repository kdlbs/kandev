---
status: current
system: agents
created: 2026-10-05
requirements:
  - REQ-AGENTS-FIRST-RUN-SETUP-001
  - REQ-AGENTS-FIRST-RUN-SETUP-002
  - REQ-AGENTS-FIRST-RUN-SETUP-003
---

# First-run agent setup system design

## Purpose and boundaries

Replace the tour's use of `ProfileFormFields` with a small dedicated form. Reuse
the existing authorized profile-discovery boundary without mounting the full
profile configuration or dependent-option resolver.

The [profile discovery design](profile-capability-discovery.md) remains
authoritative for launch context, authorization, cache identity, generations,
secrets, and provider failures. The [first-run availability design](../../ui/system-design/first-run-dialog-availability.md)
remains authoritative for the phone exclusion and completion marker.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-FIRST-RUN-SETUP-001 | Dedicated form; Responsive composition |
| REQ-AGENTS-FIRST-RUN-SETUP-002 | Shared baseline discovery; OpenCode launch compatibility; Failure handling |
| REQ-AGENTS-FIRST-RUN-SETUP-003 | Draft and mutation boundary; Responsive composition; Compatibility |

## Current defects and evidence

- `InstalledAgentRow` in `components/onboarding/step-agents.tsx` renders
  `ProfileFormFields` with only `variant="compact"`, `hideNameField`, and
  `hideCustomCLIFlags`. The component still renders permissions, modes,
  provider options, fallback policy, predefined CLI flags, and advanced options.
- That caller omits `capabilityProfileId` and `baselineProfile`.
  `isSavedLaunchSettingsCurrent` consequently returns false, and the dynamic
  model hook starts in `stale` rather than probing the saved profile. The
  picker is disabled until the user clicks Refresh. This explains the Codex
  screenshot even though its agent-wide discovery already reports installed.
- `RefreshCapabilitiesButton` uses `min-h-11` and `sm:min-h-9`, forcing 44px/36px
  minimum heights over its smaller button variant. It also occupies a labelled
  button column in the compact tour.
- Retained backend logs on 2026-10-05 report OpenCode's ACP initialization
  failure with `Invalid value for flag --log-level: "ERROR"`. An isolated
  parser reproduction using `/opt/homebrew/bin/opencode` 2.0.20 exits 1 with
  `acp --print-logs --log-level ERROR`; `acp --print-logs --log-level error` and
  `acp --print-logs` both exit 0 with closed stdin. No prompt was sent.
- The reviewed managed default remains `opencode-ai@1.18.32`. Its
  [upstream parser](https://github.com/anomalyco/opencode/blob/v1.18.32/packages/opencode/src/index.ts)
  accepts uppercase log-level choices. A universal lowercase replacement would
  break that runtime. The reproduction proves argument parsing, not a complete
  authenticated model-list handshake.

## OpenCode launch compatibility

Remove the optional `--log-level` argument and its value from OpenCode's trusted
`ManagedNPMRuntimeSpec.ACPArgs`. Keep `acp --print-logs`, ACP stdout framing,
runtime precedence, managed version selection, process cleanup, and bounded
stderr sanitization. Native and managed commands derive from that same spec,
including host-utility probes and normal execution.

Do not introduce runtime-version branching or retry an invalid command with a
different runtime. This removes a parser-specific optional default across both
dialects. Provider/default configuration controls verbosity; retained diagnostic
consumers must continue to drain and sanitize stderr independently of its volume.
Structured-error parser changes are outside this correction.

## Shared baseline discovery

Extract the existing baseline discovery state/effects from
`use-profile-model-capabilities.ts` into a domain hook,
`hooks/domains/settings/use-profile-capability-discovery.ts`. The proposed file
does not exist yet. Keep canonical launch settings, saved-context auto-probing,
request-sequence fencing, profile identity, refresh, and failure handling in
that one owner. Expose the accepted baseline, discovery state, error, and
refresh operation to both consumers.

`useProfileModelCapabilities` composes that hook with the existing
`useProfileModelOptions`; full settings keep their current behavior. The
onboarding consumer uses baseline discovery alone, so initial opening and model
selection do not invoke `resolveAgentModelConfig` or reconcile hidden
`config_options`.

Onboarding supplies the concrete saved `profileId` and its complete saved
launch context, including `envVars`, `cliFlags`, and `commandPrefix` from the
canonical `AgentProfile`. These fields are read-only inputs, not onboarding
draft fields. Automatic discovery calls the existing
`POST /api/v1/agent-models/:agentName/probe` with `{ profile_id }`. Explicit
refresh uses that same saved context with `refresh: true`; it must not send an
empty environment snapshot that clears saved overrides.

Static providers retain their declared catalog without a dynamic process.
Dynamic providers use only the accepted profile snapshot. They cannot use
`AvailableAgent.model_config` as an authoritative profile catalog. A read-only
status callback may project the expanded profile's result into its row header;
agent-wide polling must not overwrite that projection. Installation identity
and model-discovery outcome remain distinct.

## Dedicated form

Add `components/onboarding/agent-setup-fields.tsx` rather than expanding the
shared full-form component's hide flags. Its inputs are the saved discovery
context, two-field draft, provider metadata, and narrow change callbacks.

Reuse `ModelConfigSelector` with model entries and no non-model config options.
The model-only view closes on selection. Retain a saved but unadvertised model
label; mark it unavailable only after a successful matching catalog proves it
absent. A loading catalog must not paint a valid saved model as unavailable.

The refresh action uses `@kandev/ui/button` with its standard icon size and
`agents:refreshCapabilities` as its accessible name. The full profile page's
`RefreshCapabilitiesButton` also receives the standard compact icon treatment
under the [profile discovery design](profile-capability-discovery.md#responsive-behavior).
Both surfaces use the same shared icon-sizing primitive; onboarding retains its
own inline status and narrow discovery state.
Busy/error feedback is an inline localized status, not a tooltip-only error.

Reuse `@kandev/ui/switch` for supported passthrough, with a visible translated
label and accessible association. Help text explains that other settings live
in Settings > Agents. The existing read-only Auto Approve warning stays outside
the editable fields. No mode, dependent option, flags, fallback, permissions,
or advanced-setting component is mounted in this form or its picker.

## Draft and mutation boundary

Replace onboarding's dependency on `ProfileFormData` with an onboarding-owned
type containing only `model` and `cli_passthrough`. `AgentSetting` also retains
profile identity, a saved two-field baseline, the saved launch context, and dirty
state. No hidden execution or permission values are mutable draft inputs.

`buildAgentSettings` uses the existing selected saved profile and captures its
actual stored model without converting an empty stored value into a user edit.
Provider current/default model can supply a display hint separately. Reloads
preserve a dirty draft only for the same profile identity, as today.

`useOnboardingActions` derives a partial patch by comparing the two editable
fields with their saved baseline. An untouched field is omitted. The existing
`updateAgentProfileAction` and backend partial-update semantics retain all
other values. Remove `permissionsToProfilePatch`, `cli_flags`, and
`command_prefix` from the tour's write path. Preserve Skip, Next, Get Started,
save concurrency, settings-reload interlock, and failure handling.

Each successful write advances only its saved fields in the matching profile's
baseline. Recompute dirty state against the latest draft so edits made during a
save remain pending and returning to an earlier model produces a new patch.
Wait for every profile write before releasing the save interlock. On partial
failure, preserve successful baselines and retry only remaining dirty profiles.

Discovery alone never calls the draft updater. A successful refresh may change
the list and row status without changing the saved selection or dirty state.
No new database schema, profile, task, completion service, or cache is required.

## Responsive composition

Desktop entry is the first-run AI Agents step on the main page. The model
selector, adjacent refresh icon, then passthrough row form its hierarchy; Next
remains the primary navigation action. Reuse the executor step's bounded
`DialogContent` and internal scroll-body composition for the agents step.
Remove the nested `max-h-[320px]` agent-list scroller so the step body is the
single vertical scroll owner. Header, step dots, and navigation remain outside
that scroller. Use dynamic viewport bounds and the existing dialog primitives.

The nearest shipped availability exemplar is
`e2e/tests/office/mobile-onboarding-dialog.spec.ts`: below 768px the normal Home
surface appears, the tour is absent, and its completion marker is untouched.
`PageClient` retains the mounted `OnboardingDialog` with `open={!isMobile}`;
its draft stays above the visibility boundary. Returning to a larger viewport
restores the draft without saving it. This request does not enable a phone tour.
Phone users retain the existing full agent profile page for configuration.

At 768px and above with a coarse pointer, the tour uses the same focused form
and shared state with the primitives' 44px touch sizing. The standard icon
button's shared sizing helper supplies 28px fine-pointer and 44px coarse-pointer
dimensions. It must stay within its row and leave room for the model label.
Use inline error/status wrapping, not a new drawer for this small form. Existing
model-picker primitives own their bounded list scrolling and focus return.

## Failure handling

Loading shows one translated live status and disables unverified model choices;
it does not require a refresh click. Failure, authentication, unavailable
runtime, and empty catalog retain the model label and show an inline explanation.
Retry refreshes the saved concrete profile. Help can direct authentication or
installation recovery to Settings > Agents without adding advanced controls.
An unsupported/missing concrete profile is not probed as a new empty draft.
Next and Skip keep the existing ability to continue without changing the model.

Late responses are rejected by the shared identity/generation guards. Sanitized
status and errors may appear in the UI; raw provider stderr, credentials, and
launch values must not. No new metrics or raw-command logging is needed.

## Compatibility and delivery

The full profile editor continues to use dependent-model option resolution,
reconciliation, permissions, advanced configuration, and draft-context refresh.
The completed [profile discovery package](../../../plans/profile-capability-discovery/plan.md)
and [dynamic provider options package](../../../plans/dynamic-provider-options/plan.md)
remain historical records; their work orders are not reopened.

This is a local quick-setup composition and argument correction within existing
ownership and API contracts. Its rationale fits this design; no new ADR is
needed. Public onboarding guidance in `docs/public/use-kandev.md` changes with
implementation, not at this draft checkpoint.

- [Requirements](../requirements/first-run-agent-setup.md)
- [Implementation plan](../../../plans/first-run-agent-setup/plan.md)
