---
created: 2026-10-14
updated: 2026-10-19
status: implemented
requirements:
  - REQ-AGENTS-WRAPPERS-001
  - REQ-AGENTS-WRAPPERS-002
  - REQ-AGENTS-WRAPPERS-003
  - REQ-AGENTS-WRAPPERS-004
  - REQ-AGENTS-WRAPPERS-005
  - REQ-AGENTS-PROFILE-DISCOVERY-001
  - REQ-AGENTS-PROFILE-DISCOVERY-002
system_design:
  - ../../specs/agents/system-design/generic-launch-wrappers.md
  - ../../specs/agents/system-design/profile-capability-discovery.md
legacy_specs: []
---

# Implementation plan: Generic launch wrappers

## Overview and current state

Apply operator-configured wrappers consistently across supported discovery, utility, session, and CLI passthrough paths.
Prepare runtime paths on the execution host. Refuse unsupported contexts without an unwrapped retry.
Basic wrapping remains in core. Sandbox-specific plugins remain optional and deferred.

The user authorized implementation on 2026-10-18. Tasks 02 through 06 are complete. Task 06 records the passing browser and live-provider acceptance evidence.
[Task 01](task-01-complete-design.md) records the design work.
The implementation order is shared preparation, utility execution, session lifecycle, passthrough, then acceptance and public documentation.

PR #4538 delivers this package. It supersedes the earlier wrapper investigation.
The broader sandbox-provider proposal in issue #3812 remains separate.
Session continuity instructions and test-instance ownership records remain in the Kandev task.

## Scope

### In scope

- Generic profile wrapper admission separate from trusted child-command admission.
- Whole-argument runtime references, execution-host resolution, and environment consistency.
- Supported ACP, continuation, recovery, and passthrough launches with versioned agentctl compatibility.
- Failure sanitization, process cleanup, and regression evidence using unrelated wrapper names.
- Fresh Pi/Greywall acceptance without the diagnostic overlay, plus operator documentation at implementation time.

### Out of scope

No provider-name exception, plugin implementation, executor-level policy setting, task override, approval transport, or policy learning.
No new UI controls or layout changes. Existing desktop and phone profile flows require regression coverage.
Native non-ACP utility wrapper contexts remain explicitly unsupported.
No changes to the deprecated task, the retained manual instance, or its authentication files.

## Inputs and decisions

Agents owns the durable contract because it owns profile launch settings and their execution semantics.
The [requirements](../../specs/agents/requirements/generic-launch-wrappers.md) and
[design](../../specs/agents/system-design/generic-launch-wrappers.md) are the authoritative pair.

The [reproduction](reproduction.md) records real Pi/Greywall success with a temporary overlay and explicit path grants.
It does not verify the generic implementation or every launch path.
The exact-name exception and its machine-specific policy are rejected implementation directions.

The design retains profile strings, adds two typed path references, and introduces a separate internal wrapper envelope.
There is no executor override hierarchy or profile migration.
The plugin evaluation concludes that core wrapping needs no plugin capability in this increment.
Future sandbox services require a separately reviewed capability and runtime evidence.

This package does not complete the Nono initiative in issue #3812.
The queued follow-up covers Nono acceptance, supervisor ownership, approval transport,
policy selection, denial reporting, and operator-approved policy learning.
It also covers sandboxed Kandev MCP access and enforcement checks.
Basic wrapping remains independent of that optional capability.

Current authentication differs from the historical launcher ADR.
Profile mutations and draft probes use `org.config.manage`. Disabled auth still uses a synthetic administrator.
This work preserves customization and existing authority. It does not claim universal sandbox enforcement.

## Technical approach

### Shared preparation

Add `internal/common/launchwrapper` with validated versioned templates and execution-host preparation.
Use existing tokenization. Preserve whole argv values and escaped literal reference tokens.
Extract a path-returning managed npm helper while retaining private-directory checks.
Prepare the child command before composing the wrapper. Never scan user wrapper arguments for trusted npm markers.

### Utility and discovery

Extend `InferenceConfigDTO` with the internal wrapper envelope.
Retain `resolveSpawnCommand` for the child and the old prefix allowlist for legacy requests.
Use shared preparation in ACP probe, prompt execution, and OpenCode secondary listing.
Resolve secret-backed utility environment entries rather than silently omitting them.
Preserve profile cache scope, runtime generations, and prompt-free discovery.

### Session and passthrough

Carry the wrapper separately through configure, process config, one-shot commands, and protected recovery state.
Resolve references in agentctl immediately before spawning.
Carry the same template through initial, fresh, and resumed `InteractiveStartRequest` paths.
Advertise `launch-wrapper.v1` only after every v1 receiving path is implemented.
The intermediate work orders use explicit test capability fixtures. Do not advertise partial support.

### Compatibility matrix

| Consumer | Shape and intended behavior | Planned evidence | Unsupported behavior |
| --- | --- | --- | --- |
| Pi, Codex ACP, other built-in ACP agents | Trusted child plus generic profile wrapper | Actual helper subprocess argv and protocol tests | Fail before prompt if wrapper cannot start |
| Registered custom ACP agent | Existing operator-defined child plus independent wrapper | Registered-command and wrapper-admission tests | No public arbitrary-command bypass |
| OpenCode secondary discovery | Same authorized context as ACP probe | Subprocess receipt and model snapshot assertions | Use same-context ACP snapshot or fail, never unwrapped listing |
| Native Codex and Claude utility transports | Wrapper context rejected in first increment | No-spawn refusal tests | Typed unsupported status |
| Pi and existing passthrough providers | Native argv and PTY with one outer profile wrapper | Initial/fresh/resume PTY receipts and cancellation | Refuse unsupported transport/platform |
| Local/worktree execution | Resolve wrapper and runtime paths locally | Process and browser integration | No silent fallback |
| Remote ACP execution | Require installed wrapper and `launch-wrapper.v1` | Target-host context and old-peer tests | Host probe does not establish remote support |
| SSH native terminal | Reject wrapper contexts before remote startup | Real controller refusal and unwrapped control | Remote terminal transport does not carry wrapper settings |
| Legacy retained process | Preserve authoritative already-composed literal argv | Adoption and replacement tests | Ambiguous template state blocks replacement |
| No prefix, agent-wide probe | Existing default launch contract | Compatibility regressions | No profile context imported |

## Tests and traceability

The table below records the original test plan. Work-order results and [acceptance.md](acceptance.md) record the implemented tests and commands.
Each implementation work order uses Red-Green-Refactor and records actual command results.
A test that only compares DTO fields is insufficient when the acceptance criterion concerns a running process.

| Acceptance criteria | Planned proof |
| --- | --- |
| WRAPPERS-001.1, 002.1–002.4, 004.3 | `launchwrapper/prepare_test.go`: `TestGenericWrapperPrepare`, table cases for arbitrary names, paths, spacing, escaping, references, and npm isolation |
| WRAPPERS-001.2–001.4, 002.5 | `settings/handlers/generic_wrapper_test.go`, `hostutility/generic_wrapper_test.go`, `lifecycle/generic_wrapper_utility_test.go`: authority and actual final child environment |
| WRAPPERS-001.3, 001.5 | `utility/generic_wrapper_test.go`: `TestGenericWrapperAdmission`, trusted child allowlist, forged provenance, and guard lookalikes |
| WRAPPERS-003.1, 003.2, 003.5 | `utility/generic_wrapper_test.go`: `TestGenericWrapperProbe`, `TestGenericWrapperPrompt`, `TestGenericWrapperSecondary`, real fixture receipt and no prompt during discovery |
| WRAPPERS-003.3, 004.4 | `process/generic_wrapper_test.go`, `lifecycle/generic_wrapper_test.go`, `runtime/agentctl/generic_wrapper_test.go`: continuation, replacement, reset, retries, old peers, and adoption |
| WRAPPERS-003.4 | `process/generic_wrapper_interactive_test.go`, `lifecycle/generic_wrapper_passthrough_test.go`: initial/fresh/resume, inner flags, and Pi native command |
| WRAPPERS-004.1, 004.2, 004.5 | Utility and process fixture tests: refused launch has no inner-process receipt, descendants terminate, captured logs contain no sentinel credentials |
| WRAPPERS-005.1–005.3 | Plugins-disabled process/browser fixture and fresh real-provider acceptance record |
| PROFILE-DISCOVERY-001.3–001.5, 002.4–002.5 | Existing profile suites plus wrapper tests: matching model/options context, authority, no task/session writes, no fallback, and sanitized failure |

Use unrelated fixture names generated in temporary directories, including a path with spaces.
Derive every runtime path from the test process. Do not depend on a developer home or a retained diagnostic path.
Include wrappers that fail before exec, retain pipes, spawn descendants, and forward an empty argument.
Include both direct and wrapped controls. A missing wrapper receipt is failure even if the agent replied successfully.

## E2E tests

Extend `tests/settings/profile-capability-discovery.spec.ts` and its mobile counterpart.
Use real profile APIs and an isolated subprocess-backed fixture, not intercepted success responses.
Prove draft refresh, dependent model options, saved selection, missing-wrapper error, and recovery after correcting the draft.
Map these to WRAPPERS-003.1, 003.5, 004.1 and the existing discovery criteria.

Add `tests/cli-mode/generic-launch-wrappers.spec.ts` and `mobile-generic-launch-wrappers.spec.ts`.
Prove initial and resumed passthrough commands produce wrapper receipts while preserving terminal input and cancellation.
Map these to WRAPPERS-003.4 and 004.2. Exercise the existing desktop and phone interaction patterns.
No new UI composition is proposed, so no UI preview is required.

Task 06 owns the fresh Pi/Greywall acceptance procedure.
Run it in a separate disposable instance with explicit operator policy and credentials supplied privately.
Record discovery, a prompt/tool round trip, ordinary ACP chat, and passthrough with positive wrapper evidence.
Do not infer confinement or macOS support from Linux success.
Missing tools, credentials, or platform support block that acceptance item. Record the blocker and keep the PR draft.

## Work orders

Completed: [Task 01: Complete design](task-01-complete-design.md).

- [x] [Task 02: Shared wrapper preparation](task-02-wrapper-preparation.md), wave 2.
- [x] [Task 03: Authorized discovery and utility execution](task-03-utility-execution.md), wave 3, depends on 02.
- [x] [Task 04: Session lifecycle and recovery](task-04-session-lifecycle.md), wave 4, depends on 03.
- [x] [Task 05: Passthrough and capability activation](task-05-passthrough.md), wave 5, depends on 04.
- [x] [Task 06: End-to-end acceptance and operator documentation](task-06-acceptance-and-docs.md), wave 6, depends on 05.

These work orders form one implementation package. Do not release intermediate partial capability support.
The existing [profile discovery plan](../profile-capability-discovery/plan.md) remains its completed historical delivery record.
This package adds wrapper coverage without rewriting its historical test counts or reopening completed work.

## Verification results

Design checks passed on 2026-10-16:

- `python3 scripts/list-docs.py validate`: 382 decisions and 1611 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` local `validateCoverage` preflight: all six work orders passed with zero reference errors.
- Local Markdown targets, plan design membership, dependency links, and `git diff --check`: passed.

The coverage preflight supplied a synthetic runtime-path entry to force artifact validation without changing runtime files.
Its local runner was `/tmp/check-wrapper-design.cjs`. It did not publish a GitHub status.
The English structural check passed with synonym warnings excluded because technical nouns retain distinct contract meanings.
Advisory passive-voice findings do not establish formal ASD dictionary compliance.

PR #4538 now uses the implementation-oriented title and scope. Its live body, draft state, and unchanged head were checked.
The design-only checkpoint ended before production changes. The later implementation turn is recorded in [acceptance.md](acceptance.md). Task 06 completed on 2026-10-19. The requirements are active and the design is current. All five live-provider scenarios and 12 harness safety tests pass. The PR remains draft and unmerged.

## Risks

- External wrapper policy can hide required directories even when argv is correct. Kandev must report failure without broadening policy.
- Remote or retained agentctl binaries can lack the new capability. Refuse new wrapped launches until compatible binaries are available.
- Some wrappers detach processes or alter PTY behavior. Such shapes require explicit unsupported results or bounded cleanup evidence.
- Profile environment values can affect the wrapper before confinement. Generic launch success does not establish an operator security boundary.
- The manual instance uses a rejected diagnostic overlay. Its success cannot satisfy permanent implementation acceptance.
