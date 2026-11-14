---
id: "04-contract-admission"
title: "Admit plugins through observed runtime contracts"
status: done
wave: 3
depends_on:
  - "03-profile-recovery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-002
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-006
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-001.4
  - AC-AGENTS-COPILOT-PLUGINS-002.4
  - AC-AGENTS-COPILOT-PLUGINS-003.2
  - AC-AGENTS-COPILOT-PLUGINS-003.5
  - AC-AGENTS-COPILOT-PLUGINS-004.1
  - AC-AGENTS-COPILOT-PLUGINS-004.8
  - AC-AGENTS-COPILOT-PLUGINS-005.3
  - AC-AGENTS-COPILOT-PLUGINS-005.4
  - AC-AGENTS-COPILOT-PLUGINS-005.11
  - AC-AGENTS-COPILOT-PLUGINS-005.12
  - AC-AGENTS-COPILOT-PLUGINS-005.13
  - AC-AGENTS-COPILOT-PLUGINS-006.5
  - AC-AGENTS-COPILOT-PLUGINS-006.7
  - AC-AGENTS-COPILOT-PLUGINS-006.11
  - AC-AGENTS-COPILOT-PLUGINS-007.6
system_design:
  - ../../specs/agents/system-design/copilot-plugin-contract-compatibility.md
---

# Task 04: Admit plugins through observed runtime contracts

## Summary

Replace exact-version/platform admission maps with bounded native capability,
inventory, and installation checks. Make runtime preview and profile feedback
describe observed compatibility without claiming that unchecked targets failed.

## In scope

- Add the provider-owned capability helper and parser from the new design.
  Preserve the shared deadline, output bounds, cancellation, and final process identity.
- Remove every production dependency on version/platform support maps. Update
  discovery eligibility, new-process admission, controller DTOs, and affected tests together.
- Keep strict root/manifest/source validation and all-or-nothing selection preparation.
  Cover one valid plus one invalid selected plugin, and an invalid unselected control.
- Replace preview version certification with unchecked/compatible/incompatible
  state. Preserve read-only counts and candidate approval semantics.
- Extend native fixtures through production discovery/resolution, caller tests,
  localized desktop/phone feedback, and existing public Copilot guidance.
- Preserve historical native evidence and work-order results. Record actual
  new observations without rewriting earlier qualification claims as new proof.

## Out of scope

Runtime pins/defaults, generic plugin installation, remote executors, native
policy bypasses, new feature flags, automatic fallback, and enterprise certification.

## Acceptance

1. `TestCopilotPluginCompatibleUnknownRuntime` fails on the current map gate,
   then passes discovery and admission under the exact requested runtime/context.
2. `TestCopilotPluginContractFailures` rejects absent mount capability,
   malformed shapes, incompatible metadata, and mixed invalid selections with specific reasons.
3. Runtime preview stays unchecked without concrete evidence. Desktop/phone
   feedback preserves drafts, selections, native status, and existing control geometry.

## ASCII UI preview

UI-03, [full scope](plan.md#confirmed-direction-change-2026-11-05):

```text
Desktop dialog                  Phone bottom drawer
Target: <exact version>          Target: <exact version>
Plugin compatibility: unchecked Plugin compatibility: unchecked
2 profiles select plugins       2 profiles select plugins
Checked when profiles are used  Checked when profiles are used
              [Cancel] [Update]              [Cancel] [Update]

Profile Plugins [Refresh]
<Runtime>: plugin inventory format is incompatible.
Refresh or repair the plugin state. Selections are preserved.
[x] saved plugin    Native status: unknown
                                  [Reset] [Save]
```

Copy is illustrative and localized. Preserve existing scroll owners, safe-area
actions, 28px ordinary desktop controls, and at least 44px phone targets.
No finite qualified-version list or extra confirmation appears in current feedback.

## Verification

Run commands from the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/agent/copilotplugins ./internal/agent/settings/controller ./internal/agent/settings/handlers -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle ./internal/agent/hostutility -run '^TestCopilotPlugin' -count=1)
(cd apps/backend && KANDEV_COPILOT_NATIVE_VERSION=1.0.105 go test -trimpath -tags copilot_native ./internal/agent/copilotplugins -run '^TestCopilotPluginNativeAdapterAcceptance$' -count=1 -timeout=8m)
(cd apps/web && pnpm exec vitest run components/settings/agent-runtime-update-copilot-compatibility.test.tsx components/settings/copilot-plugin-profile-selection.test.tsx components/settings/copilot-plugin-selection.test.ts hooks/domains/settings/use-copilot-plugin-discovery.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-runtime-update-copilot.spec.ts tests/settings/copilot-profile-plugins.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-runtime-update-copilot.spec.ts tests/settings/mobile-copilot-profile-plugins.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Use the new provider test alone for causal Red, then record final results.
Run managed browser projects sequentially with fresh assets. Install workspace
dependencies once if missing. Run changed-Go lint with every changed Go path
and focused Oxlint with every changed web/test path; record the resolved commands.

## Files likely touched

- `apps/backend/internal/agent/copilotplugins/{context,inventory,arguments}.go`, new `compatibility.go` and `compatibility_test.go`.
- Existing provider/runtime/native tests; no platform skip can substitute for a contract assertion.
- `apps/backend/internal/agent/settings/controller/{copilot_plugin_discovery,runtime_copilot_compatibility}.go` and their tests.
- `apps/backend/internal/agent/settings/dto/{dto,copilot_plugin_discovery}.go` and handler contract tests.
- Lifecycle and hostutility Copilot caller tests; affected production callers of the retired predicate.
- `apps/web/components/settings/{copilot-runtime-compatibility,copilot-plugin-profile-selection}.tsx` and tests.
- `apps/web/lib/api/domains/agent-update-api.ts`, `apps/web/lib/types/copilot-plugin-discovery.ts`, and affected discovery tests.
- Existing four Copilot settings E2E specs/fixtures and all shipped `agents.json` locale catalogs.
- `docs/public/agents-and-profiles.md` and the sole compatibility evidence ledger.

## Dependencies

Task 03 is delivered in the current branch. Follow the new contract-admission
ADR and design, which supersede its exact-version policy.

## Risks

Help is interface evidence, not native-policy proof. Strictly parse option
tokens and keep actual native loading authoritative. A new metadata format
still requires an adapter change; never guess roots to make a release pass.

## Parallelism

`sequential`

## Inputs

The owning requirement criteria above, the new design, existing strict
inventory/installation adapters, and captured profile/process identity tests.

## Results

Implemented by the native `gpt-6-luna` worker with effort `max`, verified from
runtime metadata. The coordinator reconciled the final changed-file inventory.

Causal Red, before production edits:

```bash
(cd apps/backend && go test -trimpath ./internal/agent/copilotplugins -run '^TestCopilotPluginCompatibleUnknownRuntime$' -count=1)
(cd apps/web && pnpm exec vitest run components/settings/agent-runtime-update-copilot-compatibility.test.tsx)
```

The provider test failed because the original gate returned `unsupported` and
`runtime_unsupported` for compatible `9.9.9`. The UI test failed against the
old version-list response and qualification copy. Both became Green after the
production contract changed.

All commands in Verification passed on the final source. The provider,
controller, handler, lifecycle, and hostutility checks cover unknown-version
admission, bounded failures, all-or-nothing selection, cancellation, and
captured command identity. A coordinator finding added a regression using
the real managed-npm command shape: both help and inventory must prepare the
isolated project prefix before execution.

The four-file Vitest command passed 41 tests. The final inventory check also
ran the changed dialog-state suite:

```bash
(cd apps/web && pnpm exec vitest run components/settings/use-agent-update-dialog-state.test.ts)
```

It passed 14 tests, for 55 across five files. Typecheck and all four listed
i18n commands passed. Final fresh-asset desktop E2E passed 10/10; phone passed
6/6, sequentially with one worker each. An initial obsolete checkbox assertion
was corrected to expect removal of the deselected synthetic row while keeping
save/API/reload assertions. Both final E2E runs followed the last production
refactor, so earlier browser results are not substituted for final evidence.

The native command passed with real `@github/copilot@1.0.105` on Linux/amd64,
using production adapters and repeated mounts. No Darwin host was available;
Darwin native execution is not run. Admission contains no platform-certification
predicate or platform-simulation override. The
[sole evidence ledger](../copilot-profile-plugins/compatibility-evidence.md)
records the new observations separately from historical qualification evidence.

Public-doc validator tests passed 63/63 and content validation passed 55 pages.
Changed-Go lint passed all 26 paths and focused Oxlint passed all 13 changed
TS/TSX/E2E paths, including the dialog-state test. Their exact resolved commands
are recorded below. `git diff --check` passed. No commit, push, or workflow
completion signal was performed by the worker.

### Resolved lint commands

```bash
bash scripts/lint-go-changed \
  apps/backend/internal/agent/copilotplugins/arguments.go \
  apps/backend/internal/agent/copilotplugins/compatibility.go \
  apps/backend/internal/agent/copilotplugins/compatibility_test.go \
  apps/backend/internal/agent/copilotplugins/context.go \
  apps/backend/internal/agent/copilotplugins/copilotplugins_test.go \
  apps/backend/internal/agent/copilotplugins/inventory.go \
  apps/backend/internal/agent/copilotplugins/native_adapter_acceptance_contract_test.go \
  apps/backend/internal/agent/copilotplugins/native_adapter_acceptance_test.go \
  apps/backend/internal/agent/copilotplugins/runner.go \
  apps/backend/internal/agent/copilotplugins/runtime_compatibility_test.go \
  apps/backend/internal/agent/hostutility/copilot_plugins_test.go \
  apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_launch_test.go \
  apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_passthrough_initial_test.go \
  apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_passthrough_process_test.go \
  apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_process_test.go \
  apps/backend/internal/agent/settings/controller/copilot_plugin_discovery.go \
  apps/backend/internal/agent/settings/controller/copilot_plugin_discovery_cache_test.go \
  apps/backend/internal/agent/settings/controller/copilot_plugin_discovery_concurrency_test.go \
  apps/backend/internal/agent/settings/controller/copilot_plugin_discovery_review_test.go \
  apps/backend/internal/agent/settings/controller/copilot_plugin_discovery_test.go \
  apps/backend/internal/agent/settings/controller/runtime_copilot_compatibility.go \
  apps/backend/internal/agent/settings/controller/runtime_copilot_compatibility_test.go \
  apps/backend/internal/agent/settings/dto/copilot_plugin_discovery.go \
  apps/backend/internal/agent/settings/dto/dto.go \
  apps/backend/internal/agent/settings/handlers/copilot_plugin_discovery_test.go \
  apps/backend/internal/agent/settings/handlers/runtime_copilot_compatibility_test.go
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings \
  components/settings/agent-runtime-update-copilot-compatibility.test.tsx \
  components/settings/copilot-plugin-profile-selection.test.tsx \
  components/settings/copilot-plugin-profile-selection.tsx \
  components/settings/copilot-runtime-compatibility.tsx \
  components/settings/use-agent-update-dialog-state.test.ts \
  e2e/helpers/copilot-plugin-fixture.ts \
  e2e/tests/settings/agent-runtime-update-copilot.spec.ts \
  e2e/tests/settings/agent-runtime-update-helpers.ts \
  e2e/tests/settings/copilot-profile-plugins.spec.ts \
  e2e/tests/settings/mobile-agent-runtime-update-copilot.spec.ts \
  e2e/tests/settings/mobile-copilot-profile-plugins.spec.ts \
  lib/api/domains/agent-update-api.ts \
  lib/types/copilot-plugin-discovery.ts)
```
