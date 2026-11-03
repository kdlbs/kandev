---
id: "02-actionable-policy-recovery"
title: "Explain blocked plugin loading and model discovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-002
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-006
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-002.4
  - AC-AGENTS-COPILOT-PLUGINS-004.3
  - AC-AGENTS-COPILOT-PLUGINS-005.3
  - AC-AGENTS-COPILOT-PLUGINS-005.7
  - AC-AGENTS-COPILOT-PLUGINS-006.2
  - AC-AGENTS-COPILOT-PLUGINS-006.3
  - AC-AGENTS-COPILOT-PLUGINS-006.5
  - AC-AGENTS-COPILOT-PLUGINS-006.6
  - AC-AGENTS-COPILOT-PLUGINS-006.7
  - AC-AGENTS-COPILOT-PLUGINS-006.8
  - AC-AGENTS-COPILOT-PLUGINS-007.4
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
  - ../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md
---

# Task 02: Actionable plugin and probe recovery

## Summary

Explain the actual plugin-policy blocker in both the plugin section and model
discovery, with a safe end-to-end recovery. Deliver this slice independently of
native qualification: keeping the gate closed must not require misleading
feedback or log inspection.

This completed slice describes the gate at delivery. The later native-authority
decision removes it in WO-04 while retaining bounded legacy-response recovery.

## In scope

- Separate `ReasonPolicyUnverified` repair guidance from runtime-schema support.
- Carry the allowlisted `policy_unverified` reason through both host-utility
  preflight branches and the profile capability/model-option response DTOs.
  Add optional `reason_code` without changing status or exposing raw errors.
- Keep that reason in capability/model-option hook state and render localized
  inline recovery ahead of the generic failure fallback.
- Expand the existing blocked-selection warning with removal/Save guidance.
  Do not blame native enabled status, authentication, or model availability.
- Preserve the selected model, config options, unrelated drafts, Save/Reset,
  request fencing, and recoverable error cleanup. Do not silently drop plugins
  to make the probe succeed.
- Cover failed capability discovery and failed model-option resolution,
  explicit empty-selection retry, missing/unknown reasons, other providers,
  and a delayed old rejection after successful recovery.
- Update the Copilot how-to with the recovery sequence and its limitation.
  Final native-authority support guidance remains WO-05.
- Reuse existing markup and locale keys where correct. Add required copy in all
  seven shipped languages, generate Traditional Chinese and pseudo, and run i18n gates.

## Out of scope

Allowlist entries, version fallback, login, raw provider diagnostics, new
global error infrastructure/status enums, automatic clearing/saving, refresh
without selected plugins, or settings layout redesign.

## Acceptance

1. A policy-unqualified selection gives actionable, localized recovery in the
   plugin section and profile capability/model-option feedback, without the
   misleading generic probe-failed instruction or raw native details.
2. At delivery, the gate still rejects selected-plugin preparation before native probing.
   The user can uncheck, Save, and refresh through the ordinary empty-selection
   path; no model/config-option draft is silently changed.
3. Desktop/phone recovery is inline, accessible, and request-fenced. Missing or
   unknown reasons and unrelated provider/auth/runtime failures preserve their
   existing behavior and privacy.

## ASCII UI preview

UI-02 in the [plan](plan.md#ui-02-actionable-policy-and-model-probe-recovery)
owns the full before/after view. The inline composition is shared on desktop
and phone; copy wraps on the phone and required guidance is not hover-only.

```text
Start model [fixture-model] [Refresh]
Model discovery is blocked by this plugin selection.
Uncheck plugins, Save, then refresh models.

Plugins [Refresh plugins]
[x] fixture-plugin
Selected plugins cannot load on this runtime/platform.
Uncheck them and Save to start without plugins.
                                  [Reset] [Save]
```

Copy is illustrative and must be localized. Do not add a new action control.
Reuse direct profile navigation, the existing settings scroller and shared
safe-area-aware Save/Reset; preserve 28px desktop and 44px phone/coarse targets.
Criteria: AC-AGENTS-COPILOT-PLUGINS-006.2/006.5/006.6/006.8 and 005.7.

## Verification

Use TDD from the worktree root. New regressions:

- `copilotplugins/arguments_error_test.go::TestPolicyUnverifiedRecovery`.
- `hostutility/copilot_plugins_test.go::TestCopilotPluginPolicyProbeFeedback`:
  both preflight branches, bounded reason, and no native probe dispatch.
- `settings/controller/profile_discovery_test.go::TestCopilotPluginProfileProbeFailureReason`:
  capability/model-option projection, privacy, and absent/unknown/foreign reasons.
- `settings/handlers/copilot_profile_probe_reason_test.go::TestCopilotPluginProfileProbeReasonContract`
  (new): both response envelopes and unchanged authorization.
- Existing discovery/model-capability/status component test files: add named
  cases for localized policy recovery, retained model/options, unknown-code
  fallback, and a delayed old blocked response after recovery.

Keep legacy-error fixtures deterministic when WO-04 removes the live policy
gate. An internal preparation dependency may inject the exact typed legacy
error, but cannot weaken production version/root/context checks or revive a
global policy map. Reject "no tests to run" as Red. The commands below record
this slice's delivery; WO-04 owns necessary admission-test updates.

```bash
(cd apps/backend && go test -trimpath -count=1 ./internal/agent/copilotplugins ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/handlers -run 'TestPolicyUnverifiedRecovery|TestPolicyUnverifiedBlocksLaunchSelection|TestWrappedLaunchContextFailsBeforeNativeReads|TestCopilotPlugin')
git ls-files --modified --others --exclude-standard -z -- 'apps/backend/**/*.go' | xargs -0 bash scripts/lint-go-changed
(cd apps/web && pnpm exec vitest run components/settings/copilot-plugin-profile-selection.test.tsx components/settings/profile-capability-status.test.tsx hooks/domains/settings/use-profile-capability-discovery.test.tsx hooks/domains/settings/use-profile-model-capabilities.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && git ls-files --modified --others --exclude-standard -z -- '*.ts' '*.tsx' | xargs -0 pnpm exec eslint --max-warnings 0)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/copilot-profile-plugins.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-copilot-profile-plugins.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Inspect the changed-file lint inputs first and record their expanded lists.
Keep them limited to the package's changes. Record actual Red/Green commands,
case names, counts, and exclusions.
If the first targeted web command fails because this fresh worktree has no
workspace dependencies, run `(cd apps && pnpm install --frozen-lockfile)` once
and retry. Do not reinstall an already-working dependency tree.

Extend the existing desktop/mobile E2E files with this regression: seed a
non-default model and saved plugin, return `status: unsupported` with
`reason_code: policy_unverified`, confirm actionable inline feedback, uncheck,
Save, reload, refresh, then return a successful no-plugin model response.
Assert the model/config values survive, generic policy-error copy is gone,
and the empty selection is actually persisted. Also cover model-option failure
using an otherwise-successful capability response.

The browser fixture mocks native responses; it proves UI recovery, not native
policy compatibility. A skipped Copilot fixture does not satisfy this work
order: use the existing disposable E2E Copilot identity, not real credentials.
Run both projects sequentially with fresh production assets.

## Files likely touched

- `apps/backend/internal/agent/copilotplugins/arguments.go` and new
  `arguments_error_test.go`.
- `apps/backend/internal/agent/hostutility/types.go`, `profile_probe.go`,
  `copilot_plugins.go`, and `copilot_plugins_test.go`.
- `apps/backend/internal/agent/settings/controller/profile_discovery.go`,
  `agent_config.go`, and focused tests.
- `apps/backend/internal/agent/settings/dto/dto.go`.
- `apps/backend/internal/agent/settings/handlers/copilot_profile_probe_reason_test.go`
  (new).
- `apps/web/lib/types/http-agents.ts`.
- `apps/web/hooks/domains/settings/use-profile-capability-discovery.ts`,
  `use-profile-model-options.ts`, `use-profile-model-capabilities.ts`, and
  existing hook tests.
- `apps/web/components/settings/profile-capability-status.tsx`,
  `profile-capabilities-row.tsx`, `copilot-plugin-profile-selection.tsx`,
  and focused component tests.
- `apps/web/src/locales/*/agents.json` and the existing generated catalogs.
- Existing Copilot plugin E2E helper and desktop/mobile spec files.
- `docs/public/agents-and-profiles.md` (how-to guide, recovery only).

## Dependencies

None. Use targeted provider tests that avoid the unrelated fixture failures
owned by WO-01. No native authentication or managed-policy fixture is required.

## Risks

The controller currently drops the safe reason, and model-option state can
drop new metadata again. Cover both paths. Do not classify by raw error text,
turn an unsupported probe into a ready state, or let a stale response restore
the warning after recovery.

## Parallelism

`parallel-safe` relative to WO-01 only: owned edits are disjoint, with no shared
schema, generated contract, lockfile, or package configuration changes.
Execution remains sequential. Do not overlap WO-04/05, which share provider and
frontend contracts, or another web E2E run. No native qualification attempt is
needed to verify this slice.

## Inputs

- [Actionable probe feedback contract](../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md#actionable-profile-probe-feedback).
- `Error`, `repairAction`, both `profile_probe.go` early returns, and settings
  DTO/controller sanitization.
- Existing `commitCapabilityResponse`, `storeResolvedModelOptions`,
  `StatusOrAuthMessage`, and plugin warning.
- Existing Copilot picker/phone settings exemplars, `/mobile-parity`, `/e2e`,
  `/tdd`, and `/docs-maintainer`.
- [Probe failure diagnostics](../../specs/agents/system-design/probe-failure-diagnostics.md):
  real child-error privacy/logging stays unchanged.

## Results

Completed in the follow-up worktree. Only typed Copilot plugin preflight can
produce the allowlisted `policy_unverified` response reason, and both settings
controllers independently restrict its projection to selected Copilot profiles
with unsupported status. The capability reason lives on
`ProfileCapabilityResult`, not shared agent-wide `AgentCapabilities`, to keep
the new metadata profile-scoped. Generic sanitized errors and authorization
remain unchanged.

Both discovery paths render localized inline recovery. Existing request fences
retain the new metadata without letting a delayed blocked response overwrite
recovery. Saved models/options remain intact; a blocked probe no longer styles
the saved model as unavailable. The plugin warning and public Copilot how-to
explain uncheck, Save, and refresh/retry. All seven shipped catalogs and the
generated pseudo catalog were updated.

TDD evidence:

- The worker's historical Go Red command and outcome were not retained across
  its handoff. No historical failing count is claimed. A separate retrospective
  replay used Go's overlay support with baseline production files from
  `fb4c2bc3ef98da2fc303fe05cb144bcd6c167d58`: `copilotplugins/arguments.go`,
  `hostutility/profile_probe.go`, and
  `settings/controller/{profile_discovery.go,agent_config.go}`. Current tests,
  internal field declarations, and DTOs stayed in place. No helper graft or
  worktree source edit was needed.

  ```bash
  (cd apps/backend && go test -json -trimpath -count=1 -overlay .wo02-red-replay-20261028/overlay.json ./internal/agent/copilotplugins ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/handlers -run '^(TestPolicyUnverifiedRecovery|TestCopilotPluginPolicyProbeFeedback|TestCopilotPluginProfileProbeFailureReason|TestCopilotPluginProfileProbeReasonContract)$')
  (cd apps/backend && go test -json -trimpath -count=1 ./internal/agent/copilotplugins ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/handlers -run '^(TestPolicyUnverifiedRecovery|TestCopilotPluginPolicyProbeFeedback|TestCopilotPluginProfileProbeFailureReason|TestCopilotPluginProfileProbeReasonContract)$')
  ```

  Replay Red exited 1 without compile errors or skipped tests: 15 terminal
  test events, comprising one passing/three failing top-level tests and ten
  passing/one failing subtests. Assertions caught the baseline runtime-switch
  guidance and absent capability reason in hostutility/controller responses.
  The handler wire-contract test passed because the new DTO declarations were
  deliberately retained. Normal Green, without overlay, exited 0 with all
  15 events passing (four top-level tests and 11 subtests). These are replay
  observations, not reconstructed historical TDD results. The temporary overlay
  map, baseline copies, and raw captures were removed afterward.
- The four-file Vitest command above initially ran 34 tests: 29 passed and five
  failed. Failures covered the blocked-selection warning, policy/fallback copy,
  retained capability state, retained model-option state, and delayed rejection
  after recovery.
- The additional saved-model regression, `does not mark a saved model
  unavailable while discovery is blocked`, also failed before its fix
  (nine passed, one failed in that selector).

Recorded Green and task checks:

- The exact Go selector above passed in all four packages, with 45 named
  test/subtest events, zero failures, and zero skips.
- The four-file Vitest selector above passed 34/34; expanded focused coverage
  including model-option status and saved-model rendering passed 47/47 across
  six files:

  ```bash
  (cd apps/web && pnpm exec vitest run components/settings/copilot-plugin-profile-selection.test.tsx components/settings/profile-capability-status.test.tsx hooks/domains/settings/use-profile-capability-discovery.test.tsx hooks/domains/settings/use-profile-model-capabilities.test.tsx components/settings/model-config-resolution-status.test.tsx components/settings/profile-model-fields.test.tsx)
  ```

- Changed-file Go lint reported zero issues. Expanded inputs were
  `copilotplugins/{arguments.go,arguments_error_test.go,copilotplugins_test.go}`,
  `hostutility/{copilot_plugins.go,copilot_plugins_test.go,profile_probe.go,types.go}`,
  `settings/controller/{agent_config.go,profile_discovery.go,profile_discovery_test.go}`,
  `settings/dto/dto.go`, and
  `settings/handlers/copilot_profile_probe_reason_test.go`, all relative to
  `apps/backend/internal/agent/`. The preserved WO-01 test file was included in
  lint but not edited by this worker.
- Typecheck passed with
  `(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)`.
  The first default-heap attempt exhausted Node's heap; the larger-heap retry
  passed without source workarounds.
- Both locale generators, `i18n:check`, and `i18n:ratchet` passed. The i18n
  checker reported 559 existing orphan-entry warnings; these did not fail the
  gates.
- Changed-file ESLint passed with zero warnings. Inputs relative to
  `apps/web/` were `components/settings/{copilot-plugin-profile-selection.test.tsx,model-config-resolution-status.test.tsx,model-config-resolution-status.tsx,profile-capabilities-row.tsx,profile-capability-status.test.tsx,profile-capability-status.tsx,profile-model-fields.test.tsx,profile-model-fields.tsx}`,
  `hooks/domains/settings/{use-profile-model-capabilities.test.tsx,use-profile-model-capabilities.ts,use-profile-model-options.ts}`,
  `lib/types/http-agents.ts`, `e2e/helpers/copilot-plugin-fixture.ts`, and
  `e2e/tests/settings/{copilot-profile-plugins.spec.ts,mobile-copilot-profile-plugins.spec.ts}`.
- Sequential managed E2E passed Chromium 6/6 and mobile Chrome 3/3, using the
  required disposable Copilot identity and fresh assets. Intercepted probe
  responses prove UI recovery only, not native compatibility. Post-run
  inspection found no remaining E2E processes or runtime containers. The
  runner publishes no host ports, and teardown removes its isolated
  database/HOME and fixture profiles.
- Public-doc tests passed 63/63; validation accepted 54 published pages.
  Updated page: `docs/public/agents-and-profiles.md` (how-to guide).
- `git diff --check` passed.

No native admission, eligibility, runtime pin, authentication, or live-instance
configuration was changed. No later work order was started, and no files were
staged or committed. Native qualification was blocked at that delivery point.
Later WO-03 completed diagnostics and the user selected native policy authority;
missing enterprise-policy evidence no longer blocks WO-04/05.
