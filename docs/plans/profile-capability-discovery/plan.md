---
created: 2026-09-29
status: implemented
requirements:
  - REQ-AGENTS-PROFILE-DISCOVERY-001
  - REQ-AGENTS-PROFILE-DISCOVERY-002
  - REQ-AGENTS-PROFILE-DISCOVERY-003
system_design:
  - ../../specs/agents/system-design/profile-capability-discovery.md
legacy_specs: []
---

# Implementation plan: Profile capability discovery

## Overview

Make profile discovery use the launch settings shown in the profile editor.
First deliver a validated backend discovery contract with isolated caches.
Then connect the editor, add deterministic provider evidence, and verify desktop and phone flows.
Implementation was authorized by the user on 2026-09-29. Both work orders are complete.

## Scope

### In scope

- Saved and unsaved concrete-profile environment entries, secret references, CLI flags, and command prefix.
- Both baseline model discovery and dependent model-option resolution.
- Current managed runtime selection, refresh generations, bounded caches, and sanitized failures.
- Existing profile settings on desktop and phone, including new drafts.
- Public guidance for profile refresh and host-only discovery.

### Out of scope

- Updating a bridge's bundled Codex CLI or changing runtime update version labels.
- Automatic binary selection, package installation, or runtime activation from profile refresh.
- Remote executor discovery, workspace/repository overlays, live session mutations, and routing policy.
- New provider gateway behavior, dynamic-profile expansion, and workflow-only profile inference.

## Confirmed intent and assumptions

The user requested a plan after confirming that profile overrides never reach discovery.
The intended result is provider-neutral: discovery receives the same relevant launch settings as a host profile launch.
The design uses explicit refresh for draft launch changes and automatic discovery when a saved profile opens.
That avoids executing a partially typed command prefix while retaining immediate saved-profile behavior.
No product question blocks this package.

Agents owns the contract because it owns profile identity and provider capabilities.
The existing [dynamic-provider-options package](../dynamic-provider-options/plan.md) is complete and remains historical delivery evidence.
Its model-resolution semantics remain valid. This package adds launch-context identity without reopening its completed work orders.
The [runtime-update requirements](../../specs/agents/requirements/runtime-updates.md) retain package activation ownership.

## Technical approach

### Backend contract

Add the typed profile probe route beside `FetchDynamicModels` and extend `ResolveAgentModelConfig` with the same optional context.
Resolve authorized saved or draft inputs in the settings controller before calling host utility.
Reuse existing profile validation, secret authority, environment precedence, CLI tokenization, and managed command resolution.
Pass the resolved environment, flags, and prefix through `InferenceConfigDTO` on both paths.

Add a separate bounded profile cache. Its identity includes the effective command and resolved launch context.
Keep agent-wide snapshots separate and preserve generation fencing across refresh and managed runtime activation.
Audit the actual child process, secondary provider commands, and command logs.
The lower probe already supports environment and CLI fields; the test must prove process delivery, not only DTO construction.

### Editor integration

Extend `ProfileFormData` and the caller boundary where needed so the discovery hooks receive the current environment draft.
Extend `settings-api.ts` and `http-agents.ts` with the typed profile request.
Use `useProfileModelCapabilities` for both baseline and model-option context.
Preserve the context-free hook contract for workflow and agent-wide callers.

A launch edit marks the current discovery stale. Refresh captures the complete draft once.
Only a matching snapshot makes new choices selectable.
Late requests cannot replace a newer launch context, model, or profile.
Keep the existing picker open during dependent-option loading and retain existing option-reconciliation behavior.

### Compatibility matrix

| Provider or consumer | Transport/context | Behavior | Verification | Unsupported result |
| --- | --- | --- | --- | --- |
| Codex ACP | ACP; `CODEX_PATH` env | Forward profile context, preserve bridge pin | Fake child fixture plus optional local acpdbg reproduction | Visible failure, no bundled-CLI retry |
| Other ACP agents | Generic env, ordered flags, prefix | Same contract, no provider-name map | Utility subprocess and controller tests | Typed unsupported/failure |
| OpenCode secondary model listing | ACP plus helper command | Same applicable context or use ACP snapshot | Helper invocation assertions | Never merge default-context choices |
| Native Codex when enabled | Existing native utility probe | Forward supported context without changing protocol selection | Native process regression suite | Explicit unsupported if context cannot be honored |
| Gateway profiles | Existing provider-specific bypass | Preserve current skip/auth behavior | Profile component regression | Existing gateway status |
| Agent-wide and workflow-only callers | No concrete profile context | Keep existing default probe/resolver | Existing hook and hostutility suites | Existing status conventions |
| Remote executors | Separate runtime | Host results remain hints | Requirement and docs review | No remote parity claim |

## ASCII UI preview

UI-01: Settings > Agents > concrete profile. Existing selector and Refresh action remain in place.
Names below are illustrative. All new copy uses locale keys.

Desktop, after a launch-setting edit:

```text
Start model                                      [Refresh]
[6 Sol / High                                      v]
Launch settings changed. Refresh models.

Environment variables
[CODEX_PATH] [Value] [/path/to/new/codex]
```

Phone, same state and shared data:

```text
< Agents     6 Sol High
Start model
[6 Sol / High             v]
Launch settings changed.
[Refresh models]

Environment variables
[CODEX_PATH]
[Value]
[/path/to/new/codex]
```

UI-02: Matching refresh outcome, inside the existing model picker:

```text
[Filter models...]
  6.1 Sol
  6 Astra
  6 Sol (selected)
```

UI-03: Progress and failure replace the stale message:

```text
Refreshing models...             [busy]
```

```text
Could not refresh models.        [Retry]
[6 Sol / High - retained selection]
```

Structural requirements: stale, loading, and failed contexts do not offer unverified choices.
The current selection stays visible. A successful refresh does not select the newly advertised model automatically.
The status wraps below the selector on phones. Refresh and Retry have at least 44px touch targets.
The page keeps its current scroll owner; the existing picker keeps its own bounded list scrolling and focus return.
No new overlay or navigation surface is required. The existing mobile profile-selector test is the nearest shipped exemplar.
UI-01 through UI-03 map to AC-AGENTS-PROFILE-DISCOVERY-003.1 through 003.4.

## Tests

Names marked **new** are planned test files or methods, not existing evidence.
Each work order follows Red-Green-Refactor for its changed logic.

| AC suffixes | Test evidence |
| --- | --- |
| 001.1, 001.2, 001.3 | `controller/profile_discovery_test.go`: saved versus full draft, explicit clears, secret authority, actual request construction |
| 001.3, 002.4 | **New** `utility/profile_probe_context_test.go`: fake subprocess advertises different models from env/flags; prefix wrapper records actual invocation |
| 001.4, 002.5 | Controller/handler tests: wrong agent, foreign profile/secret, invalid args, secret precedence, sanitized logs and responses |
| 001.5, 001.6 | Hostutility and controller tests: no prompt/session/profile writes; agent-wide cache unchanged |
| 002.1, 002.2, 002.3 | **New** `hostutility/profile_capability_cache_test.go`: two profiles, canonical env, ordered flags, TTL/cap, secret rotation, runtime activation, late completion |
| 002.4, 003.2, 003.3 | Hook/component tests: failed refresh, missing model, retry, preserved values, no environment-only save deadlock |
| 003.1, 003.2, 003.3 | `use-dynamic-models.test.ts`, `profile-form-fields.test.tsx`, **new** `use-profile-model-capabilities.test.tsx`: dirty launch context, clear, refresh, stale response, model-option context |
| 001.2, 001.3, 003.4 | Desktop and mobile E2E against a deterministic subprocess-backed mock |

The subprocess test checks both baseline and post-model snapshots under the same context.
A DTO-only test cannot prove that `CODEX_PATH`, flags, or a wrapper reaches the child.
Mock fixtures must not access real credentials or use a commercial model request.

## E2E tests

Task 02 adds `tests/settings/profile-capability-discovery.spec.ts` under Chromium and
`tests/settings/mobile-profile-capability-discovery.spec.ts` under mobile-chrome.
Use the real HTTP routes and isolated backend with mock-provider catalog variants controlled by a test environment entry and CLI flag.
Do not fulfill the model-list route with a canned response in the core regression.

Scenarios:

1. Saved profile A receives its configured catalog; profile B retains the default catalog (001.1, 001.6, 002.1).
2. Edit environment without saving, refresh, select the newly advertised model, save, reload (001.2, 001.3, 003.1, 003.2).
3. Change a CLI flag and supported prefix; verify fixture catalog/invocation evidence (001.3).
4. Repeat the draft refresh and dependent-option flow on phone. Verify touch-sized refresh controls, the latest launch context, and no horizontal overflow (003.4).

Unit and component tests cover stale-response rejection after launch edits, failed model-option discovery and Retry, and selection preservation.

Extend existing profile ACP, CLI flag, mobile selector, and workflow tests for compatibility where request shapes change.
Run the exact commands in Task 02 sequentially through the managed runner, which rebuilds the affected application.

## Work orders

- [x] [Task 01: Profile discovery API and probe isolation](task-01-profile-probe-contract.md), wave 1.
- [x] [Task 02: Profile editor refresh and end-to-end delivery](task-02-profile-editor-discovery.md), wave 2, depends on Task 01.

The API/cache slice can be verified independently before UI adoption.
Task 02 owns the complete user flow, fixture changes, translations, and public documentation.
No separate generic QA or review work order is required.

## Verification results

Implementation and documentation checks passed. The commands and detailed counts are recorded in the two work orders.

- Backend controller, handler, hostutility, utility, CLI-flag, lifecycle, race, and mock-agent suites passed.
- Follow-up review regressions passed for profile-scoped refresh, activation during a blocked probe, and activation during command resolution; the hostutility race suite and focused backend packages passed again.
- Focused frontend Vitest passed, with typecheck, scoped ESLint, i18n checks, and production Vite build.
- Desktop E2E passed 27 tests; mobile E2E passed 10 tests.
- Public documentation validation passed. The catalog contains 330 decisions and 1248 specifications; specification lint tests passed, 36 tests.
- PR documentation coverage and `git diff --check` passed.

The requirement is active and the system design is current. Implementation is committed and under review in PR #4070.

## Remaining boundary

Host discovery is an editing aid and cannot predict remote executor capabilities. Runtime package selection and activation remain owned by the runtime updater.
