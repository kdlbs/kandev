---
created: 2026-09-08
status: completed
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-003
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
  - ../../specs/agents/system-design/no-silent-model-fallback-02.md
legacy_specs: []
---

# Implementation Plan: Profile selector model warnings

## Overview

Remove host model advisories from saved-profile selectors. Keep profile editing
diagnostics and actual executor model-selection warnings available where users
can act on them.

The agent system owns this change because it owns profile model intent and
executor model authority. Amend the existing
[requirements](../../specs/agents/requirements/no-silent-model-fallback.md);
do not create a separate UI specification.

One work order delivers the shared selector change with unit, desktop, mobile,
and runtime-warning regression coverage. The user authorized implementation,
PR creation, a 15-minute wait, and PR fixup through `@implement`.
Deployment remains outside this package.

## Evidence and classification

This is a change to intended product behavior. The former system design
explicitly required the amber selector icon; the implementation follows that
rule. Requirement 002.7 now locates the host advisory in the profile editor.
Requirement 003 defines selector behavior and preservation checks.

Read-only investigation on 2026-09-08 established:

- The live instance used backend port 38429 and database
  `/root/.kandev/data/kandev.db`.
- Profile `4fe051a9-f5cc-422b-bd0c-05048c22699e`, named Astra High, stored
  `gpt-6-astra` and `settings.config_options.reasoning_effort = high`.
- Session `cfac5c57-f9a8-4b89-bc4c-2db8a0d6e20f` persisted that model and
  effort in its settled runtime configuration.
- At 22:29:53 +01:00, logs reported that start-model policy applied Astra and
  published `current_model_id = gpt-6-astra`.
- A fresh `acpdbg probe --timeout 45s codex-acp` completed with exit code 0.
  Codex ACP 1.10.0 exposed 31 legacy model/effort IDs and six canonical model
  configuration values. Both representations included Astra.
- Both live capability and available-agent APIs contained the canonical Astra
  ID at inspection time.

The confirmed warning mechanism is the exact membership check in
`useAgentProfileOptions`: a nonempty browser host catalog that omits the
saved ID produces an amber advisory in both render paths. It never checks the
eventual executor outcome. A host catalog with several bracketed variants also
takes this path because no single variation matches.

The browser catalog at screenshot time was not captured. Staleness and legacy
ID representation are possible triggers, not established cache defects. The
design does not depend on choosing between those hypotheses.

The temporary probe capture was
`/tmp/kandev-model-warning-probe-OL5GGn/codex-acp-probe-20260908-213045.jsonl`.
It is optional local evidence, not a dependency for implementation. Do not
commit raw protocol frames or copy the live database into test fixtures.

## Scope

### In scope

- Remove host model advisory icons, help triggers, and selected-label indicators.
- Apply the result through the existing shared profile-option hook.
- Preserve capability health indicators, eligibility, recent-use ordering,
  profile labels, saved configuration, and passthrough indicators.
- Preserve profile-editor model diagnostics and persisted task warnings.
- Remove unused selector translations and explain warning placement in public
  agent-profile documentation when the implementation ships.
- Cover desktop keyboard selection, mobile row taps, successful model launch,
  actual fallback, and reload.

### Out of scope

- Host probe refresh, browser cache invalidation, or new polling.
- ACP normalization changes or provider-specific bracket parsing.
- Model/fallback policy, runtime warning metadata, database schema, or migrations.
- Runtime model-selector redesign, profile-editor redesign, Office routing,
  and unrelated capability-warning accessibility changes.
- New flags, telemetry, screenshots for public docs, or provider network calls.

## Technical approach

In `apps/web/components/task-create-dialog-options.tsx`, remove the
model-catalog comparison and the two model-warning renderers. Keep the
capability hook active, and use its snapshot only to refresh health fields on
profile options. Keep the existing option interface and shared label rendering
compatible with consumers.

The shared hook serves these existing callers:

- `components/task-create-dialog-computed.ts`
- `components/task/new-subtask-dialog.tsx`
- `components/task/new-session-dialog.tsx`
- `components/quick-chat/quick-chat-setup.tsx`
- `components/automations/config-section.tsx`
- `app/office/setup/agent-profile-setup-controls.tsx`

These callers require no separate presentation branches. In particular,
Office keeps its surrounding compatibility filtering.

Do not remove `getCapabilityWarning`, model fields from profile state, or the
editor's `findUniqueModelVariation` helper. Remove only the two unused
`settings` selector keys from every locale, including pseudo. Keep
`modelVariationAdvisory` and runtime-warning translations.

The profile editor already prefers structured model configuration options in
`components/settings/profile-model-fields.tsx`. Preserve that canonical source;
this task does not introduce another model-ID resolver.

Public documentation is owned by the same work order: add a short explanation
under “Host probes and executor model catalogs” in
`docs/public/agents-and-profiles.md`. This is an explanation section within
the existing agent-profile guide.

## Tests

The following regression scenarios define the acceptance coverage.

| Acceptance | Test file and scenario |
| --- | --- |
| 003.1 | `components/task-create-dialog-options.test.tsx`: “does not show host model advisories in option or selected labels”, parameterized over exact, missing, unique, multiple, bracketed, empty, pending, and changed host catalogs. |
| 003.2, 003.6 | Same file: preserve eligibility, ordering, profile label and saved values; retain existing disabled-profile and recent-use cases. |
| 003.3 | Same file: “preserves capability health warnings independently of model membership”, for auth_required, not_installed, failed, and healthy status. |
| 003.4, 002.7 | `components/settings/profile-form-fields.test.tsx` and `lib/model-variation.test.ts`: retain missing-model, unique-variation, and fallback-control coverage. |
| 003.5, 001.4 | `components/task/chat/messages/status-message.test.tsx`: retain structured model-selection warning rendering. |
| 001.5, 003.6 | Browser reload checks and saved-profile API comparison in the successful-launch scenario below. |

The first red test must render a healthy-capability profile with a missing host
model and assert no advisory in either label. It fails on the current
implementation because the dropdown renders a warning button and the selected
label renders a titled icon. Absence of the old test ID alone is insufficient.

## E2E tests

| Project / file under `apps/web/e2e/tests` | Outcome and acceptance |
| --- | --- |
| chromium / `settings/no-silent-model-fallback.spec.ts` | Replace tooltip assertions with warning-free missing/unique profile selection. Check option and selected label, keyboard selection, and no nested warning control. Covers 003.1–003.2. |
| mobile-chrome / `settings/mobile-no-silent-model-fallback.spec.ts` | Replace warning-drawer taps with row selection. Verify selection closes the picker, updates its label, and leaves the create flow usable without horizontal overflow. Covers 003.1–003.2. |
| Both settings files above | Add “launches a host-mismatched profile on its requested executor model without a warning”. Submit through the UI, verify effective model and zero model-selection warnings after completion and reload, then compare saved profile configuration. Covers 003.5–003.6 and 001.5. |
| chromium / `session/model-mismatch-warning.spec.ts` | Preserve actual fallback, unique-variation, ambiguous-catalog, single-warning, and reload coverage. Covers 001.4 and the preservation side of 003.5. |
| mobile-chrome / `session/mobile-model-mismatch-warning.spec.ts` | Preserve visible actual fallback warning and reload without horizontal overflow. Covers 001.4 and 003.5. |

For the successful-launch fixture, extend
`session/model-mismatch-warning-helpers.ts` with a disposable mock profile
whose exact model is `opus[270k]` and whose profile environment selects the
existing `MOCK_AGENT_MODEL_CATALOG=ambiguous` fixture. The default E2E host
catalog includes `opus[1m]`, so it cannot serve as the omitted model. Confirm that the host
catalog omits that ID while the executor advertises it. Assert those
preconditions so an ordinary exact-match launch cannot pass as the regression.
Use existing model options for valid saved configuration values; no live Codex
account is needed.

## Work orders

- [x] [Task 01: Remove host model advisories from profile selectors](task-01-remove-host-model-advisories.md) — done, sequential, no dependencies.

## Verification results

Planning checks on 2026-09-08:

- Specification-linter tests: 30 passed.
- Full specification lint: passed after reducing the amended design to its size limit.
- Whitespace check: passed.

Implementation checks on 2026-09-08:

- Red: 11 component assertions and two selector cases per browser project failed
  on the former model-warning controls.
- Green: 56 targeted unit tests, six desktop browser tests, and four mobile
  browser tests passed.
- Typecheck, targeted ESLint, localization checks, and the localization ratchet passed.
- Public-documentation validator: 61 tests passed and 46 pages validated.
- Desktop and mobile screenshots show selectable profiles without a model
  advisory or an empty warning control.

Task 01 records the commands, fixture correction, and mobile interaction detail.

## Risks

- Removing all warning icons would hide agent health errors; tests must distinguish
  those indicators from model advisories.
- Removing only the dropdown button leaves the selected-label warning intact.
- Capability polling is shared with the selector hook. Keep it independent from
  model-ID matching so health status remains current in selector-only surfaces.
- Old mobile tests deliberately open the warning drawer. Replace those steps
  with completed row selection rather than deleting the tests.
- The saved screenshot does not establish a stale-cache defect. Do not expand
  implementation into speculative discovery changes.
- Runtime unique-variation behavior is already covered by current code and
  requirements, although its ADR remains proposed. This package does not
  change that policy or its decision status.
