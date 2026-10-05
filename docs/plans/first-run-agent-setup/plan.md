---
created: 2026-10-05
status: complete
requirements:
  - REQ-AGENTS-FIRST-RUN-SETUP-001
  - REQ-AGENTS-FIRST-RUN-SETUP-002
  - REQ-AGENTS-FIRST-RUN-SETUP-003
  - REQ-AGENTS-PROFILE-DISCOVERY-001
  - REQ-AGENTS-PROFILE-DISCOVERY-002
  - REQ-AGENTS-PROFILE-DISCOVERY-003
system_design:
  - ../../specs/agents/system-design/first-run-agent-setup.md
  - ../../specs/agents/system-design/profile-capability-discovery.md
legacy_specs: []
---

# Implementation plan: First-run agent setup

## Overview

Restore OpenCode startup compatibility, expose the existing saved-profile
baseline discovery as a reusable hook, then deliver the simplified tour form
and narrow saves plus compact refresh on the full agent profile page.
Work orders execute sequentially. Implementation was authorized after the
design review. Implementation and local rendered validation are complete.

Agents owns this package because its durable result is a saved agent profile
with correct provider discovery. The UI system retains first-run availability
and generic control sizing. Existing full-profile delivery packages remain
complete; this package adds a separate quick-setup consumer.

## Confirmed intent and evidence

The user requests only a model selector and CLI passthrough toggle in onboarding,
smaller refresh controls, hidden Auto Approve/fallback/advanced settings, and no
manual refresh prerequisite. The read-only warning stays because hiding a setting
does not change its default; it adds no editable configuration.

The follow-up screenshot confirms the same refresh sizing defect on the full
agent profile page. Its model-list refresh is also in scope, using the same
standard icon sizing while retaining full-profile configuration and discovery.

Root causes are source-confirmed: the full form is reused, saved profile identity
and baseline are omitted, and refresh has 36px/44px minimum-height overrides.
Retained logs and an isolated OpenCode 2.0.20 parser reproduction identify the
uppercase `--log-level ERROR` argument failure. Omitting that optional argument
works on the reproduced binary; changing it universally to lowercase conflicts
with the pinned 1.18.32 upstream parser. See the design for evidence limits.

## Scope

### In scope

- Shared OpenCode ACP argument compatibility across native and managed commands.
- Saved-context automatic baseline discovery with existing request guards.
- Dedicated model-only picker, compact accessible refresh, and supported passthrough.
- Hidden-setting preservation through two-field drafts and partial updates.
- Loading, failure, empty list, missing model, recovery, and profile-status continuity.
- Desktop/short-viewport and coarse-pointer sizing; existing phone suppression.
- Compact full-profile refresh aligned with model/mode selectors, with phone
  and coarse-pointer touch sizing.
- Localized help, existing warning, focused regression tests, and public tour guidance.
- User-requested PR skill guidance to start repository captures with isolated
  Playwright and classify permission failures against the capability that failed.

### Out of scope

- Permission defaults, runtime installation/version changes, parser-version dispatch,
  feature flags, new API/cache/schema contracts, and remote discovery.
- Full-profile settings redesign beyond its refresh control/row and later tour steps.
- Phone onboarding, a new completion marker, or broader UI control-size sweeps.
- Structured OpenCode stderr parser changes or raw diagnostic logging.

## Technical approach

### OpenCode commands

Update `OpenCodeACP.ManagedNPMRuntime().ACPArgs` in `opencode_acp.go` to retain
`acp --print-logs` without `--log-level`. Update command expectations in
`opencode_acp_test.go` and the built-in catalog entry in
`managed_npm_runtime_test.go`; leave synthetic generic-runtime fixtures alone.
Add strict CLI parser fixtures proving commands accepted by both log-level
dialects. Preserve all command selection and lifecycle semantics.

### Shared discovery

Extract `useProfileCapabilityDiscovery` and its tightly coupled baseline helpers
from `use-profile-model-capabilities.ts` into the proposed
`use-profile-capability-discovery.ts`. Full profile capabilities still compose
baseline discovery and `useProfileModelOptions`. Baseline-only callers must not
run the model-option resolver. Preserve saved/draft distinction, request guards,
canonical launch-context identity, statuses, and existing refresh behavior.

### Tour composition and writes

Replace `ProfileFormFields` in `InstalledAgentRow` with the proposed
`agent-setup-fields.tsx`. Use `ModelConfigSelector` with a model list and no
dependent options. Use a standard icon Button, existing Switch, and localized
inline status. Expanded profile status must supersede stale agent-wide probe
status without changing installation identity.

Define the narrow shared onboarding types in the proposed
`components/onboarding/agent-settings.ts`, and use them from
`onboarding-dialog.tsx`, `step-agents.tsx`, and `use-onboarding-actions.ts`.
Read saved launch fields, including env entries, for discovery; draft only model
and passthrough. Compare each field against its saved baseline before sending
a patch. Keep hidden fields out of writes and preserve current navigation/error
interlocks. Share the executor step's viewport-bounded scroll-body arrangement
for the agents step, without changing the remaining steps.

Update `common:onboardingAgentsHelp` across English, pt-pt, zh-cn, ja, ko and
generated zh-tw/zh-hk via `pnpm run i18n:zh-hant`; update pseudo-locale handling
as required by the existing generator/check. Reuse existing model, refresh,
passthrough, and status keys wherever accurate. Keep the warning's key/value.
Update the onboarding paragraph in `docs/public/use-kandev.md` with the shipped
two-field behavior. The page remains a getting-started tutorial with a
disclosed reference section; no new public page or screenshot is required.

### Full profile refresh

Update `RefreshCapabilitiesButton` in `profile-capability-status.tsx` to use
`Button size="icon"`, the existing translated refresh name as `aria-label`,
and no local height/min-height/full-width overrides. Preserve tooltip, spinner,
disabled state, callback, test ID, and error/recovery behavior. Adjust
`CapabilitiesRow` in `profile-capabilities-row.tsx` to align the action with
model/mode inputs; on phones place model and refresh together, then mode below.
Status/error content must not alter the action's dimensions or alignment.
The profile page retains its existing scroll owner and every other setting.
Task 04 owns this correction and its exact rendered-size regression evidence.

### Compatibility matrix

| Consumer/runtime | Context | Intended behavior | Evidence | Unsupported case |
| --- | --- | --- | --- | --- |
| Native OpenCode 2.0.20 | ACP; lowercase-only log-level parser | Omit optional level; retain stderr printing | Isolated parser reproduction and strict CLI fixture | Visible provider failure; no version switch |
| Managed OpenCode 1.18.32 | ACP; uppercase-only parser | Same common command arguments and existing version pin | Pinned upstream source and strict CLI fixture | Existing sanitized failure |
| Saved Codex/other dynamic agents | Authorized concrete profile | Auto-probe saved context; model list ready without Refresh | Hook/component tests and Chromium flow | Inline retry/Settings recovery |
| Static agents | Declared static catalog | Display available choices without dynamic probing | Component test | Empty catalog with preserved value |
| Full profile settings | Saved or draft launch context plus model | Keep option resolution and explicit draft refresh | Existing hook/component and profile E2E suites | Existing recovery behavior |
| Full profile refresh | Desktop/phone; modes present or absent | 28px fine-pointer icon; 44px touch target; aligned selectors | Updated profile discovery E2E and action component test | Existing recovery; no oversized button |
| Phone below 768px | Tour visibility suppressed | Normal page; no save/completion; draft restored on return | Mobile resize flow | No phone tour added |
| Coarse-pointer tablet at 768px+ | Same quick-setup form | Touch-sized model/refresh/toggle controls | Mobile project at 768px, rendered measurements | Existing Settings alternative |

## ASCII UI preview

UI-01: Current expanded Codex row, source and screenshot-confirmed.

```text
Codex                                      Installed
Start model                    [Refresh models (large)]
[gpt-6.1-sol                    v] (disabled)
Launch settings changed. Refresh models.
[Auto-approve all permissions                 switch]
[CLI Passthrough                              switch]
[Fallback settings                                  v]
[Advanced settings                                  v]
```

UI-02: Proposed expanded agent, ready on desktop (first-run AI Agents step).

```text
+-----------------------------------------------------+
| AI Agents                                           |
|                                                     |
| Codex                                  Installed  ^ |
| Start model                                         |
| [6.1 Sol                              v] [refresh]  |
| CLI Passthrough                          [off/on]   |
|                                                     |
| Other agent                                      v  |
| ...                                                 |
| More settings are available in Settings > Agents.   |
| Careful: default profiles can auto-approve.          |
|                                                     |
|                     step dots                       |
| [Skip]                                      [Next]  |
+-----------------------------------------------------+
```

The refresh label in the drawing names an icon's action; visible markup contains
the icon and a localized accessible name. Selector and refresh are 28px high
for fine pointers. Header/dots/footer are fixed within the dialog; the step body
owns vertical scrolling. Passthrough appears only when supported. This structure
is required; wording and whitespace are illustrative/localized.

UI-03: Proposed loading, failure, and empty-list regions (same desktop/tablet form).

```text
Start model
[6.1 Sol (retained)                 v] [busy]
Loading models...                         (live status)
```

```text
Start model
[6.1 Sol (retained)                 v] [refresh]
Could not load models. Retry or open Settings > Agents.
```

An empty successful catalog shows a localized empty explanation. Failed or
unverified choices are disabled; matching successful choices are enabled.
Neither refresh nor failure changes the model or dirty state. Missing selections
remain visible after successful discovery, without a silent replacement.

UI-04: Phone visit and return to a coarse-pointer larger screen.

```text
Phone (<768px)              Tablet (768px+, coarse pointer)
+----------------------+    +----------------------------------+
| Home / normal page    |    | AI Agents                        |
|                      |    | Codex                Installed ^ |
| Existing mobile      |    | Start model                      |
| navigation and       |    | [saved draft        v] [refresh] |
| task actions         |    | CLI Passthrough       [off/on]   |
|                      |    | ... internal scroll body ...     |
|             [+ task] |    | [Skip]                    [Next] |
+----------------------+    +----------------------------------+
No onboarding dialog.       Model/icon targets >=44px.
No save or completion.      Same draft restored; not saved yet.
```

The phone view follows the existing availability contract, not a new mobile
tour. Full profile settings remain the phone configuration entry point. UI-02
and UI-03 cover SETUP-001.1-.5 and SETUP-002.1-.5. UI-04 covers SETUP-003.4
with the existing UI availability criteria. Tests measure both icon dimensions,
row containment, viewport bounds, and scroll ownership.

UI-05: Full agent profile refresh (Settings > Agents > profile).

```text
Desktop:
Start model                  Start mode
[5.5 / High             v]   [Approve for me       v] [refresh]
Refresh: 28px square, aligned with selectors.

Phone / coarse pointer:
Start model
[5.5 / High                    v] [refresh]
Start mode
[Approve for me                         v]
Refresh: >=44px square. Status wraps below.
Other profile settings continue in the existing page scroller.
```

The refresh drawing represents an icon with a localized accessible name and
tooltip. Loading uses the same-sized disabled spinner; failures retain aligned
refresh and recovery. Model-only rows omit mode without changing icon geometry.
UI-05 maps to AC-AGENTS-PROFILE-DISCOVERY-003.2-.5. Desktop/phone checks cover
normal, busy, failed, mode-present and mode-absent states. Copy and spacing are
illustrative; action order, capability, alignment, and sizing are required.

## Tests

Planned names are new evidence to write during implementation, not completed tests.

| Acceptance | Unit/integration evidence |
| --- | --- |
| SETUP-002.2 | `opencode_acp_test.go`: `TestOpenCodeACPCommandsAcceptBothLogLevelDialects`; existing native, managed, host-utility command tests |
| PROFILE-DISCOVERY-001.1, 002.1/.3, 003.1-.3 | `use-profile-capability-discovery.test.tsx`: saved auto-probe, draft stale, refresh, failure, profile switch, late response |
| SETUP-002.1/.4-.6, SETUP-003.5 | Baseline-only hook makes no model-option calls; existing full-profile hook and form tests retain option resolution |
| SETUP-001.1-.4, SETUP-002.1/.3-.5 | `agent-setup-fields.test.tsx`: only required controls, matching model choices, loading/error/empty/gone states, status override, no false stale startup |
| SETUP-003.1-.4 | `onboarding-dialog.test.tsx` and new `use-onboarding-actions.test.tsx`: exact partial patch, hidden-field preservation, untouched/reverted draft, failures, interlocks, reopened same-profile draft |
| PROFILE-DISCOVERY-003.2-.5 | New `profile-capability-status.test.tsx`: accessible refresh, busy/disabled state and callback; existing form tests preserve options/recovery. Updated profile E2E proves size/alignment and touch behavior. |

## E2E tests

- New `tests/settings/onboarding-agent-setup.spec.ts` in Chromium: expand saved
  dynamic agent, await profile probe response, choose without Refresh, assert
  advanced controls and option sections absent, toggle passthrough, save exact
  partial patch, then verify hidden stored fields unchanged. Include failure and
  retry, empty/missing model, refresh without saving, and long-name/short-height
  geometry. Intercept provider response boundaries for deterministic error cases;
  real API profile reads/writes prove persistence. Map to SETUP-001, SETUP-002,
  SETUP-003.1-.3. Run existing `onboarding-executors.spec.ts` too.
- Extend `tests/office/mobile-onboarding-dialog.spec.ts` in mobile-chrome:
  phone visit, enlarge to 768px, edit draft, shrink to 393px, enlarge again,
  verify draft/marker/no save; touch model/refresh targets at least 44px and
  contained in their row. Add a narrow fine-pointer viewport case in desktop
  coverage to confirm tour absence below 768px. Map to SETUP-003.4.
- Run existing `tests/settings/profile-capability-discovery.spec.ts` and
  `mobile-profile-capability-discovery.spec.ts` plus
  `mobile-agent-profile-config-selector.spec.ts` for full-editor discovery,
  advanced options, and phone interaction continuity (SETUP-003.5).
- Extend desktop/mobile profile discovery specs with UI-05 measurements:
  28px within 1px for fine-pointer desktop, at least 44px in both dimensions
  for phone/coarse pointer, aligned model/mode bounds, no modes, busy/error
  states, keyboard and touch refresh. Cover the 768px boundary and absence of
  horizontal overflow (PROFILE-DISCOVERY-003.2-.5).
- Use causal HTTP/WS waits for probe/save/retry, not timed sleeps. Capture a
  focused screenshot of desktop ready/error and coarse-pointer tablet ready
  states, and compare it with UI-02 through UI-04.

## Work orders

- [x] [Task 01: Restore OpenCode ACP argument compatibility](task-01-opencode-arguments.md)
- [x] [Task 02: Share baseline profile discovery](task-02-baseline-discovery.md)
- [x] [Task 03: Deliver focused onboarding agent setup](task-03-onboarding-form.md)
- [x] [Task 04: Compact full-profile refresh](task-04-profile-refresh.md)

## Verification results

Implementation and review remediation on 2026-10-05:

- Seven focused Vitest files: 67 tests passed, including reproduced review
  regressions for saved baselines, empty dynamic catalogs, duplicate refresh
  buttons, and partial-save concurrency.
- Complete `internal/agent/agents` and `internal/agentctl/server/utility` Go
  package tests passed with an isolated writable build cache.
- All pre-commit checks passed in an isolated writable delivery checkout,
  including changed-package Go lint, formatting, web lint, and new-copy guards.
- Focused ESLint with zero warnings, web typecheck, all-language `i18n:check`,
  and `build:e2e` passed.
- Documentation catalog, all-spec lint, public-document validation, and
  `git diff --check` passed.
- Managed Playwright host runs passed 3 Chromium and 6 mobile-chrome tests:
  exact partial writes, Next/Back saves, single refresh geometry, saved-context
  discovery, full-profile options/recovery, and tablet draft continuity across
  phone suppression. Runs used one shard and one worker, sequentially.
- Isolated headless Playwright CLI captured desktop onboarding ready/error,
  short desktop onboarding, coarse-pointer tablet onboarding, and desktop/phone
  full profiles. Refresh measured 28px square on desktop and 44px on touch.
  The tour is structurally absent below 768px; phone coverage verifies that
  suppression without consuming the tour or saving its draft.
- Harness validation passed for the PR skill's capture-routing guidance. The
  user explicitly requested bundling that guidance into this PR.

All four work orders are complete. PR publication and subsequent remote CI/review
handling are delivery work and do not change these local verification results.

## Risks

- Dropping an explicit log level can increase stderr volume. Keep printing,
  sanitization, and bounded draining; do not weaken diagnostic protections.
- Extracting discovery can disturb full-editor dependent resolution if accepted
  contexts or callback identities change. Task 02 reruns its existing suites.
- Using agent-wide choices, dropping saved environment input, or saving hidden
  defaults would recreate the reported defects. Exact-context and patch tests
  are required.
- Model-only setup preserves hidden provider options. Existing runtime validation
  and full settings remain authoritative for incompatible saved options.
- Native parser success is not proof of authentication or a complete model
  handshake. Deterministic ACP/model evidence is required during implementation;
  live credentialed discovery remains optional and must not mutate user instances.
