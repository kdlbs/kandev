---
created: 2026-10-04
status: complete
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-003
system_design:
  - ../../specs/agents/system-design/runtime-model-discovery.md
legacy_specs: []
---

# Implementation plan: Model discovery after runtime activation

## Overview

Keep runtime evidence tied to profile discovery for [issue #4205](https://github.com/kdlbs/kandev/issues/4205).
Refresh an open profile after successful runtime activation, using its current complete draft.
Runtime versions and management remain in Settings > Agents > Agent runtime updates.
Profile pages contain model controls and discovery feedback without a runtime panel.

The user removed the profile runtime presentation requirement on 2026-10-05.
Task 03 completes that revision. Tasks 01 and 02 record the original delivery.
The current requirement and system design take precedence over the historical Task 02 presentation scope.

## Evidence and root cause

Investigation used main commit `5094a41082c` on 2026-10-04.
The issue reports missing models with old bridges and different results after runtime changes for the same account.
Its Windows and account-specific results are reported evidence, not locally reproduced provider behavior.
The issue has no attachments or comments. Related issue #2419 is closed.

Current code already supports profile launch settings, including `CODEX_PATH`.
The completed [profile discovery package](../profile-capability-discovery/plan.md) addressed that independent mismatch.
`PublishCapabilities` also invalidates backend profile and option caches after managed activation.
Those completed work orders remain historical evidence and do not need reopening.
The [runtime notifications](../agent-runtime-notifications/plan.md) and [compact runtime settings](../agent-runtime-settings-compact/plan.md) packages supply existing status and recovery controls.
This package reuses their final contracts and adds no notification or policy redesign.

The current discovery projection drops `AgentCapabilities.AgentVersion`.
`DynamicModelsResponse` contains models, status, and context revision, but no version or source information.
`ProfileCapabilitiesSection` renders model controls and status without a runtime summary or management entry.
`useProfileModelCapabilities` retains a local snapshot and does not observe successful update jobs.
The `agent.update.finished` handler only writes job state, so backend invalidation alone does not refresh an open profile.

A temporary test called the real `FetchProfileDynamicModels` controller with an existing repository fixture.
Its host utility returned status `ok`, bridge version `1.11.0`, and one model.
`TestIssue4205RuntimeVersionSurvivesProfileDiscovery` failed because serialized discovery omitted that version.
The command was:

```bash
(cd apps/backend && go test ./internal/agent/settings/controller -run '^TestIssue4205RuntimeVersionSurvivesProfileDiscovery$' -count=1)
```

The failure included `status: ok`, `context_revision: revision-1`, and the model, without `agent_version`.
The temporary test was removed after diagnosis. Implementation must create permanent coverage for the approved `runtime_info` contract.

Upstream [Codex ACP documentation](https://github.com/agentclientprotocol/codex-acp) identifies bundled Codex and the `CODEX_PATH` override.
Its [spawn implementation](https://github.com/agentclientprotocol/codex-acp/blob/main/src/CodexJsonRpcConnection.ts) selects the external path or package-local Codex.
The [Claude ACP manifest](https://github.com/agentclientprotocol/claude-agent-acp/blob/main/package.json) identifies its Claude Agent SDK dependency.
These sources establish component relationships. Their current versions are not defaults or compatibility rules for this package.

## Requirement conformance and settled scope

`REQ-AGENTS-RUNTIME-UPDATES-003` owns runtime evidence and profile refresh after activation.
Its revised acceptance criteria exclude runtime details and update actions from profile model settings.
The [runtime model discovery design](../../specs/agents/system-design/runtime-model-discovery.md) defines the current boundary.
No new updater, API, persistence boundary, or ownership decision is required.

## Scope

### In scope

- Preserve bounded, read-only runtime evidence in profile discovery responses.
- Preserve accurate managed, bundled, external, and unknown component attribution.
- Refresh an open profile once after a new successful managed activation.
- Preserve selected models, unsaved draft settings, and stale-response guards.
- Remove the profile runtime panel, its unused component tests, and its locale strings.
- Retain runtime information and update controls in existing Agents runtime settings.
- Update desktop and phone regressions and public guidance.

### Out of scope

- Replacement disclosures, tooltips, or update actions in the profile model region.
- New runtime settings, automatic consent, model-access guarantees, or remote inspection.
- Changes to backend runtime collection or installation behavior.

## Technical approach

### Observation boundary

Add trusted observation descriptors in `internal/agent/agents/runtime_observation.go`.
Declare the relevant components in `codex_acp.go` and `claude_acp.go`.
Use the existing internal probe request to transport those descriptors from registration into agentctl.
Keep descriptor types at an existing shared agent/DTO boundary to avoid a settings-controller dependency in agentctl.

Add a focused `server/utility/runtime_observation.go` collector beside `ACPInferenceExecutor.Probe`.
Reuse sanitized environment, npm project isolation, trusted exact cache identity, and owned process cleanup.
Read actual installed manifests. Inspect an explicit Codex executable only with the fixed `--version` recipe.
Keep strict size and time bounds, including Windows paths and command shims.
Preserve observation through `hostutility/types.go`, `capabilitiesFromProbe`, and `settings/controller/profile_discovery.go`.
Add optional DTO fields without changing legacy response meanings.

### Editor and recovery boundary

`ProfileCapabilitiesSection` renders the existing model controls and discovery feedback.
It does not render runtime metadata or subscribe a runtime panel to release status.
Remove `ProfileRuntimeInfo`, its component tests, and unused `profileRuntime*` locale keys.
Backend metadata and the hook's correlated snapshot remain compatible.

Observe the matching agent's terminal update job in the profile hook.
On a newly succeeded job, invalidate old requests and refresh the current complete draft once.
Failed, historical, duplicate, and unrelated jobs remain inert.
Resolve dependent options from the matching refreshed snapshot.

## ASCII UI preview

Desktop model region:

```text
Start model [Selected model v]  Mode [v]  [Refresh models]
Models match the current launch settings.
```

Phone model region, using the existing page scroller:

```text
Start model [Selected model v]
Mode [v]
[           Refresh models           ]
Models match the current launch settings.
```

These previews preserve the current model controls without runtime rows or actions.
The existing phone refresh control retains its 44px touch target.
Runtime selection stays in Settings > Agents with its existing desktop dialog and phone drawer.

## Tests

Retain backend observation, host-context isolation, and activation-generation coverage from Task 01.
Retain hook coverage for successful activation, current drafts, duplicate completion, historical jobs, failures, and late responses.
Keep existing profile form and model-option coverage.
Remove tests whose subject is the deleted runtime panel.

## E2E tests

Update the existing desktop and phone profile-discovery specs.
Return runtime metadata from the fixture and assert that no runtime panel appears after discovery or activation.
Cover managed, external, and unknown metadata without rendering runtime details.
Keep a profile open while another tab uses the existing update surface.
Assert a refresh with current draft values, new model choices, unchanged selection, and unchanged saved data.
A failed candidate preserves the prior catalog.
Retain phone refresh dimensions, narrow fine-pointer coverage, and document overflow checks.
Existing runtime settings and their update surfaces remain unchanged.

## Work orders

- [x] [Task 01: Preserve and collect runtime observations](task-01-runtime-observations.md)
- [x] [Task 02: Original runtime presentation and refresh](task-02-model-runtime-recovery.md)
- [x] [Task 03: Remove runtime details from profile settings](task-03-remove-profile-runtime-panel.md)

Tasks 01 and 02 record the original implementation and verification. Task 03 records the user-directed UX revision.

## Original delivery verification results

- The focused Go runtime-observation packages, the six-package regression set, and hostutility race tests passed. The managed E2E runner built the backend.
- Windows-native test execution was unavailable on Linux. Windows-targeted tests compiled for `agentctl/server/utility` and `agent/hostutility`.
- The initial focused frontend suite passed 21 tests. Typecheck and targeted ESLint passed.
- `i18n:zh-hant`, `i18n:check`, and `i18n:ratchet` passed.
- The managed desktop E2E suite passed 18 tests. The initial managed phone suite passed 8 tests.
- Public documentation validation and tests, specification catalog validation, spec lint, and `git diff --check` passed.
- The web E2E production build passed. Vite reported existing advisory chunk-size and ineffective-dynamic-import warnings.

## Original delivery code-review results

- Native OpenCode is classified from the captured launch command. The exact native command is external, the exact managed npm fallback is Kandev-managed, and custom wrappers remain unknown. Native OpenCode receives its trusted CLI guidance URL and no managed bridge action or managed release-status wording.
- Relative PATH entries and executable paths resolve against the captured probe work directory. Empty PATH entries continue to resolve to that directory.
- Backend runtime-source, command-prefix ambiguity, and relative-path regressions pass, including the full six-package test set, hostutility race tests, backend command builds, and Windows-targeted test compilation for hostutility and agentctl utility.
- The focused profile/status hook suite passed 11 tests; web typecheck, targeted ESLint, locale checks, and the new-code i18n ratchet passed. Desktop E2E passed 18 tests and mobile E2E passed 9 tests, including second-tab status transitions and the external-runtime touch action.
- Public documentation tests and validation, specification catalog validation, spec lint, and final `git diff --check` passed.

## User-directed UX revision results (2026-10-05)

Task 03 removes runtime presentation from profile pages while preserving activation refresh and backend observation metadata.
Runtime management remains in the existing Agents runtime settings.

- Desktop RED found the old runtime panel before removal.
- Focused hook, form, and model-option coverage passed 36 tests after the final code change.
- Desktop profile-discovery E2E passed 3 tests. Phone profile-discovery E2E passed 5 tests.
- Phone checks cover managed, native, and unknown metadata without runtime presentation, current-draft activation refresh, and authentication recovery.
- A narrow fine-pointer regression found the existing Refresh control shrinking to 36px at 767px. Its touch target now remains at least 44px below 768px and on coarse pointers.
- Typecheck, targeted ESLint, locale completeness checks, and the new-code i18n ratchet passed.
- Public documentation tests passed 62 tests. Public documentation, specification catalog, and spec lint validation passed.

## Risks

- The original report's Windows/account-specific model omission cannot be reproduced here without its external environment and credentials.
- Exact bridge pins do not lock dependency ranges. Installed manifest evidence must describe the selected execution tree.
- A wrapper can change environment or executable identity. Unknown evidence is safer than a false host observation.
- Windows shims and paths with spaces need fixture coverage and a native Windows run before claiming platform execution proof.
- A successful candidate can still have different models under a profile override. Reprobe the current profile after activation.
- Extra version inspection has a two-second total budget and must never break a successful catalog.
