---
id: "03-refresh-stable-pins"
title: "Validate and refresh stable CLI pins"
status: completed
wave: 3
depends_on:
  - "01-decouple-runtime-tests"
  - "02-complete-weekly-pr"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-001.5
  - AC-AGENTS-RUNTIME-UPDATES-001.7
  - AC-AGENTS-RUNTIME-UPDATES-001.9
  - AC-AGENTS-COPILOT-PLUGINS-001.1
  - AC-AGENTS-COPILOT-PLUGINS-003.2
  - AC-AGENTS-COPILOT-PLUGINS-003.3
  - AC-AGENTS-COPILOT-PLUGINS-004.1
  - AC-AGENTS-COPILOT-PLUGINS-005.8
  - AC-AGENTS-COPILOT-PLUGINS-007.6
system_design:
  - ../../specs/agents/system-design/runtime-updates-01.md
  - ../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md
---

# Task 03: Validate and refresh stable CLI pins

## Summary

Refresh trusted defaults from current stable npm metadata. Establish Copilot
adapter compatibility before admitting the new exact version and document the
actual scope of the evidence.

## In scope

- Resolve all nine packages again and compare them with the plan's snapshot.
- Add a bounded `copilot_native` inventory compatibility test for an exact
  candidate. Keep it independent of the macOS-only authenticated policy harness.
- Use disposable homes, installation metadata, manifests, and two plugin roots.
  Include a path with spaces. Observe inventory and repeated-mount behavior.
- Use `/acp-debug` for any native ACP investigation. Keep captures private and
  record only sanitized evidence in the existing compatibility ledger.
- Record adapter evidence for the exact Copilot version. Preserve the current
  capability and inventory checks instead of adding a version allowlist.
- Run the existing pin updater. Reconcile the supported-adapter documentation
  and generated delivery records with the actual diff and validation results.

## Out of scope

Global CLI installation, copying authentication stores, login, enterprise-policy
changes, feature flags, weakening adapter admission, or changing native authority.

## Acceptance

1. The catalogue matches stable metadata for every proposed update and retains
   OpenCode's supported package majors. Unchanged entries remain unchanged.
2. The Copilot candidate has exact native inventory, installation-metadata,
   manifest, and repeated-mount evidence. Incompatible contracts fail visibly.
3. The six-package suite and generated-record coverage pass with the refreshed
   catalogue. Public adapter documentation describes only observed support.

## Verification

Run from the repository root. The new inventory test uses isolated fixtures
and does not require authenticated model calls or an enterprise-policy fixture.

```bash
set -euo pipefail
(cd apps/backend && KANDEV_COPILOT_NATIVE_VERSION=1.0.105 go test -trimpath -tags 'copilot_native fts5' -count=1 -timeout 5m ./internal/agent/copilotplugins -run '^TestCopilotPluginNativeInventoryCompatibility$' -v)
node scripts/update-agent-runtime-pins.mjs
node --test scripts/update-agent-runtime-pins.test.mjs .github/scripts/record-agent-runtime-pin-refresh.test.mjs
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/agents ./internal/agent/managedruntime ./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/agent/copilotplugins ./internal/agentctl/server/utility ./internal/agentctl/server/adapter/transport/acp)
python3 .github/scripts/update-agent-runtime-pins-workflow-contract_test.py
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If the registry candidate changes, update the exact test version and Results
before the native run. Record actual commands and sanitized observations.
Run `bash scripts/lint-go-changed` with the exact changed Go paths.
Record native incompatibility or unavailable evidence accurately. Do not replace
an error with a support claim to finish the pin refresh.

## Files likely touched

- `apps/backend/internal/agent/agents/managed_npm_runtime_versions.json`
- `apps/backend/internal/agent/copilotplugins/context.go`
- `apps/backend/internal/agent/copilotplugins/copilotplugins_test.go`
- `apps/backend/internal/agent/copilotplugins/native_inventory_compatibility_test.go` (new)
- `docs/plans/copilot-profile-plugins/compatibility-evidence.md`
- `docs/specs/agents/system-design/copilot-profile-plugins-native-qualification.md`
- `docs/public/agents-and-profiles.md`, supported-adapter reference paragraph.
- Generated maintenance records from Task 02.

## Dependencies

Tasks 01 and 02 restore the validation and delivery path for the refreshed pins.

## Risks

Stable release metadata does not prove native compatibility. Missing credentials
limit authenticated diagnostics but do not independently block adapter evidence.
Actual inventory or metadata incompatibility must remain visible.

Public docs impact: update the existing agent guide's adapter reference after
support changes. No new page or screenshot is required.

## Parallelism

`sequential`

## Inputs

- The plan's version snapshot and CI evidence.
- Runtime design: central catalogue, exact commands, and scheduled workflow.
- Copilot design: qualification/admission, evidence, and native authority.
- Existing compatibility ledger and disposable native fixtures.

## Results

Completed on 2026-11-05.

- Registry lookup resolved the plan's candidate snapshot exactly. The updater
  refreshed eight of nine packages; `pi-acp` stayed at `0.0.34`. Both OpenCode
  packages retained their supported majors.
- Added `TestCopilotPluginNativeInventoryCompatibility` behind the existing
  `copilot_native` tag. It installs the exact package in disposable homes,
  checks package and binary identity, observes two repeated `--plugin-dir`
  mounts (including a path with spaces and shell-like characters), installs a
  manifest-backed plugin from a local marketplace, verifies the native
  installation metadata, and resolves the plugin through the adapter's parser
  and root validation.
- The inventory compatibility helper `runNativeInventoryCommand` uses the
  production owned-process `runCommand` helper, including process-tree cleanup
  and `WaitDelay` pipe draining. The acceptance harness `runNativeCommand` keeps
  its own `Start`/`Wait` flow with context-based process cancellation and does
  not use `runCommand`. A focused helper-process regression proves cancellation
  returns promptly and reaps a descendant that retains an output descriptor.
- The isolated native test passed for `1.0.105` on Linux x64 in 6.1 seconds.
  It did not authenticate or exercise managed policy.
- The branch originally added `1.0.105` to the adapter set after that run. Main
  subsequently replaced version-based admission with capability checks. The
  compatibility ledger describes the observation without claiming authenticated
  or enterprise certification.
- Inspected the `@agentclientprotocol/claude-agent-acp@0.100.0` package contract
  and added it to the exact verified prompt-suggestion fallback versions without
  broadening the range.
- The generated delivery record lists the actual eight changed packages and the
  validation that preceded it.
- The six-package suite plus `copilotplugins`, `internal/agentctl/server/utility`,
  and `internal/agentctl/server/adapter/transport/acp` passed with the refreshed
  catalogue and the `fts5` build tag. `node --test
  scripts/update-agent-runtime-pins.test.mjs
  .github/scripts/record-agent-runtime-pin-refresh.test.mjs` passed 18/18.
- `node --test scripts/validate-public-docs.test.mjs` passed 63/63 and
  `node scripts/validate-public-docs.mjs` validated 55 pages.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`,
  `bash scripts/lint-go-changed`, and `git diff --check` passed.
