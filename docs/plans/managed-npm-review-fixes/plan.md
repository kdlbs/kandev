---
created: 2026-11-09
status: done
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-007
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
  - ../../specs/agents/system-design/managed-npm-cache-recovery.md
  - ../../specs/agents/system-design/managed-npm-repair-reuse.md
legacy_specs: []
---

# Fix plan: Managed npm launch and repair reuse

## Overview

Correct PR #5208's Start ownership bug and managed-launch recognition with profile CLI arguments.
Unify single-user repair owners and remove blocking toolchain discovery from manager state locks.
Finish with integrated recovery proof and explicit maintenance guidance.

Agents owns this correction because it owns managed-runtime recognition, repair identity, and startup recovery.
Auth retains authorization policy. Executors retain compute admission.
This package amends existing requirements/designs and adds an ADR plus six sequential work orders.
All six work orders were implemented and verified on 2026-11-09, including Docker and SSH retained-storage evidence.
Implementation ran locally without live-instance maintenance, commit, push, or PR mutation.

## Verified baseline

On 2026-11-09, PR [#5208](https://github.com/kdlbs/kandev/pull/5208) is OPEN, unmerged, and reported mergeable.

- Local and PR head: `74d3b2fae82e5e0bb78bda3df1ce8e481ad70d05`.
- GitHub PR-reported base and local merge-base: `58cf68ddd2ea411df98e012e5a2e406393e78cbf`.
- Remote main, independently read through GitHub ref and `git ls-remote`: `95424c2b1a672bf17831df8ce64fd7ee3585e538`.
- Required checks pass for that PR head. No unresolved review threads were present.
- The working tree was clean before this planning turn.

Remote main has advanced. Existing PR checks do not prove the future integrated patch.
Refresh both heads before implementation and delivery.
Inspect upstream changes, including `43a41b11d32` in the process manager, before reconciling the branch.
No merge or branch rewrite occurs during planning.

## Findings and root causes

| Reported item | Verified source behavior | Work order |
| --- | --- | --- |
| 1. Duplicate Start drops publication | `startWithGeneration` detaches before admission/running checks. Rejection can remove the candidate handle and stop its expiry timer. | 01 |
| 2. Background owner differs | Orchestrator prefers caller identity. Lifecycle consumes caller-derived workspace information. Unowned background work can resolve empty, while boot/UI discovery uses system/default scopes. | 04 |
| 3. npm version blocks state locks | Context completion runs `npm --version` with a background timeout under start/initialize state-lock regions. | 05 |
| 4. Cleanup guidance lacks location/procedure | Public docs mention explicit removal without the exact store path and all surviving consumers. | 06 |
| 5. Duplicate-Start regression missing | Fold into finding 1. The existing normal-miss test expects detachment even after admission closes. | 01 |
| CLI-argument incident | Full-argv equality rejects a registered package with valid profile flags. Discovery has independent equality and flag-exclusion gates. | 02, 03, 06 |

### Incident evidence

The reported session used `@agentclientprotocol/claude-agent-acp@0.100.0` and `--dangerously-skip-permissions`.
Its profile was Opus 5.5 Medium.
Task: `f2045b0a-bbb5-4d62-b9fb-d82e01a32374`.
Session: `2444d3b7-3567-45ee-8c65-ff7c6b470834`.

The user supplied installed-package inventory, a missing execution manifest, and npm exit 254.
The missing file was `/root/.npm/_npx/6b9485ba72ad1e50/package.json`.
The later ACP closed pipe is a downstream symptom.
The user also supplied two temporary failing regressions, with passing controls without the flag.
Those tests and their detached checkout were removed.
This turn verifies the rejecting predicates by source inspection and does not claim a new reproduction.

Persisted npm `UNKNOWN` is a sanitizer marker.
Do not infer a raw npm code from it.
Retain complete executor-local evidence and the independently verified manifest condition.

The ownership gap also has source support without reading the live database.
`CreateWorkspace` permits empty owners in disabled-authentication mode.
`SyntheticIdentity` uses `userstore.DefaultUserID`.
The existing helpers produce different repair owners for that valid default configuration.
`startupRecoveryOwnerContext` returns the original context in disabled mode, so it does not supply a missing synthetic owner.

## Requirements and decisions

Use the existing [shared recovery contract](../../specs/agents/requirements/managed-npm-runtime-recovery.md),
[cache requirements](../../specs/agents/requirements/managed-npm-cache-recovery.md),
and [reuse requirements](../../specs/agents/requirements/managed-npm-repair-reuse.md).

- 006.11/.12 clarify valid profile arguments and the trusted launcher boundary.
- 007.17-.20 cover rejected Start continuity, default-user ownership, cancellable metadata lookup, and maintenance guidance.
- Existing 004.3/.4, 006.4/.7/.9, and 007.4/.6/.8/.10/.13 retain permission, evidence, isolation, and retry constraints.

The [launch-identity ADR](../../decisions/2026-11-09-managed-npm-launch-identity.md) records command and owner boundaries.
The amendments were implemented and verified on 2026-11-09.

## Scope

### In scope

- Preserve repair ownership and timers when Start is rejected.
- Recover and reuse registered runtimes with valid profile CLI arguments.
- Cover task start/resume and agent-wide/profile/model-option discovery.
- Unify disabled-mode owner resolution across interactive and background callers.
- Make context completion cancellable and reuse stable version facts outside state locks.
- Prove desktop/phone/executor outcomes and document safe store removal.

### Out of scope

- Changes to the live instance, profiles, caches, running agents, or persisted ownership.
- Removing permission flags, changing models, normal npm preferences, or retry limits.
- New launcher forms, arbitrary prefixes, wrappers, operator commands, or direct-native repair.
- Requirement 005's dependency repair, new executors, or new compute lifetimes.
- Automatic cleanup, record migration, SQL changes, feature flags, new settings, UI changes, or storage-size advisory changes.
- Paid model requests, public npm test downloads, implementation, and delivery mutations in this turn.

## Technical approach

### Start admission

Run admission, running/passthrough, and revision guards before any handle detachment.
A rejected request changes no current handle, retained-owner list, timer, generation, or physical-cache evidence.
Keep accepted stopped-process context-miss cleanup as a separate regression.
Use a real owned candidate and an independent lock waiter to prove continued publication.

### Launcher and agent arguments

A proposed structured parser under `agent/managedruntime` validates the npm region and exact package position.
A proposed `ManagedNPMRuntimeSpec.MatchRuntimeCommand` applies registered package and required protocol metadata.
These names describe planned additions.

Accept the existing direct canonical npx shape and freshness preference.
Required runtime arguments follow the exact package.
Preserve the remaining authorized agent arguments and their boundaries.
Never search an arbitrary prefix or the agent tail for installation authority.
Owned-cache rewriting uses validated positions and the selected handle.
An agent value that equals the package or resembles an npm option remains opaque.

Align `exactManagedRuntimePackage`, `managedRuntimeSpecForArgs`, `onlineManagedRuntimeArgs`,
`managedRuntimeProbeRetry`, `managedRuntimeProbePackageForAgent`, and `eligibleManagedRuntimeProbeConfig`.
Audit native Codex predicates and `PreparedRuntime.RewriteNpxCommand` as separate consumers.
Replace whole-argv equality and blanket flag rejection only in the validated managed path.
Compose `InferenceConfigDTO.CLIFlags` once through the existing launch builder.
Keep full-argv attempt fingerprints, profile supersession, and separate result publication.

### Canonical owner

One pure policy receives trusted actor, workspace owner, and permitted default-user inputs.
Resolve a real actor first, then an explicit workspace owner, then the disabled-mode fallback.
Backend composition reads the live auth service.
Unknown/setup/enabled mode never supplies a default-user fallback.

Hydrate real workspace ownership at its producer and use the same policy for fresh/resumed/background launches.
Boot and UI agent-wide discovery share the default owner only in disabled mode.
Preserve authorized profile scopes and enabled-mode system discovery.
Existing signed generations retain their original keys and files.

### Context and version facts

Require operation context first in `RepairStore.Context` and the probe helper, then propagate it through npm version lookup.
Capture immutable inputs under `startMu`, compute facts outside it, then revalidate ticket, generation, revision, owner, and admission.
Cancellation propagates as cancellation, rather than a miss that starts a provider.
Initialized agents survive publication failure.

Memoize successful version facts in a bounded process-local cache.
Include resolved npm/node executable identity, `NODE_OPTIONS`, and `NODE_PATH` in its key.
Task/provider configuration and npm registry values do not fragment this version cache.
Their installation-context checks remain fresh.
Invalidate changed executable/input facts. Do not cache failed or cancelled lookups.
Keep mutable npmrc/environment and publication checks fresh.
Do not hold state or memo mutexes during subprocess work.
Concurrent cold misses can resolve independently without a shared-flight owner.

### Maintenance

Document `<KANDEV_HOME_DIR>/managed-runtime-repairs/v1/` on each execution host.
Without an override, the path is `~/.kandev/managed-runtime-repairs/v1/` under that host's operating-system user.
Require stopped provider, installer, utility, and agentctl consumers and prevent automatic relaunch.
Backend shutdown alone is insufficient for surviving or remote agents.
Remove only the verified store, preserving `.npm`, provider state, and other Kandev files.
If consumer ownership is uncertain, retain the store.
The next eligible launch uses ordinary preparation and recovery.
Preserve normal npm freshness behavior. Do not promise that every reuse performs zero metadata requests.

## Compatibility matrix

| Shape | Required behavior | Proof / fallback |
| --- | --- | --- |
| Claude ACP 0.100.0 plus incident flag | Recovery and later reuse with unchanged argv/conversation | Task/process and real-npm fixture |
| Other registered ACP, OpenCode v1/v2 | Preserve exact selected distribution and required arguments | Descriptor table and existing family controls |
| Managed native Codex app-server plus valid flags | Preserve required subcommand and arguments | Native task and utility cases |
| Startup/profile/model-option discovery | Shared installation boundary, separate result owners | Path-specific hostutility/probe tests |
| Disabled auth, unowned workspace | Interactive/background and boot/UI parity | Default-mode owner and wiring tests |
| Enabled/setup/unknown auth | Preserve distinct user/profile/system scopes | Negative owner/mode tests |
| Local PC/Docker/SSH retained storage | Reuse execution-host generation | Real-npm and existing executor fixtures |
| New container/lost storage | Ordinary miss | Existing new-storage control |
| Old peer/unavailable store | Existing eligible private preparation or bounded visible failure | Compatibility controls, including old helpers that reject flagged commands |
| Windows current build | Existing private recovery while reusable-store ACL support is disabled | No new Windows-reuse claim |
| Wrapper/custom/native/invalid npm shape | Existing visible unsupported/ineligible outcome | Negative recognizer/compatibility cases |

## Presentation and mobile contract

No markup, copy, control, geometry, navigation, or interaction change is planned.
The session transcript remains the scroll owner on desktop and phone.
Settings retains existing responsive profile refresh controls and touch entry points.
The closest exemplars are the existing managed-npm session/settings helpers.
First recovery retains attempt-2 presentation. Later first-attempt reuse shows ordinary startup/resume.
No UI preview is required because the rendered composition is unchanged.
If a UI change becomes necessary, update the design and mobile contract first.

## Tests

All named additions below are planned.
Write and observe the failing regression before its production correction.

| Acceptance / finding | Planned evidence |
| --- | --- |
| 007.7/.8/.13/.17; review 1, 5 | `TestManagedRuntimeRepairRejectedStartPreservesTicket`, admitted-miss and selected-cache controls |
| 006.11/.12; incident | `TestManagedNPMInvocation`, `TestManagedRuntimeCacheEvidenceWithCLIArgs`, task start/resume CLI-argument tests |
| 007.2/.4/.10; incident | `TestManagedRuntimeRepairReuseWithCLIArgs`, opaque-value and exact-rewrite tests |
| 001.6-.8, 006.2/.4 | `TestManagedRuntimeProbeRepairWithCLIArgs` for each discovery entry point and managed protocol |
| 007.6/.18; review 2 | `TestManagedRuntimeOwnerPolicy`, `TestManagedRuntimeBackgroundOwnerReuse`, probe owner parity and backend mode wiring |
| 006.7, 007.9/.12/.13/.19; review 3 | npm version memoization, context cancellation, Stop availability, stale publication-context tests |
| 006.4/.8/.11, 007.2/.5/.15 | Real-npm incident fixture and desktop/phone/executor cases |
| 007.20; review 4 | Exact cleanup path/procedure and both public-doc validators |

Use barriers for rejection, cancellation, and stale-callback schedules.
Use local archives and protocol fixtures without public npm or paid inference.
Do not alter incident profiles or caches to make tests pass.

## E2E tests

Add `managed npm review fixes` cases to the shared session/settings helpers and their desktop/mobile specs.
Cover the flag, first recovery, cold resume/restart, unchanged native token, one provider start on reuse, and one follow-up.
Settings covers profile refresh and model options without saved/draft changes or cross-catalogue publication.

Extend Docker/SSH retained-storage fixtures with valid flags and a new owned process.
Kill only the fixture-owned process, retain the store, and preserve shared sentinels.
Count installer calls and archive downloads separately from incidental metadata requests.
A new container remains a miss.
Retain original startup, family, policy, cancellation, old-peer, and private-recovery controls.

## Work orders

- [x] [Task 01: Preserve repair ownership on rejected starts](task-01-preserve-start-ownership.md)
- [x] [Task 02: Recognize managed task launchers with profile arguments](task-02-managed-launcher-arguments.md)
- [x] [Task 03: Keep recovery and reuse for profile discovery arguments](task-03-discovery-arguments.md)
- [x] [Task 04: Unify repair owners across interactive and background work](task-04-canonical-owner-scope.md)
- [x] [Task 05: Resolve toolchain context outside manager state locks](task-05-cancellable-toolchain-context.md)
- [x] [Task 06: Prove integrated recovery and document safe maintenance](task-06-proof-and-maintenance-guidance.md)

Execute sequentially. These dependencies do not authorize subagents.

## Companion records and delivery

The original [reuse](../managed-npm-repair-reuse/plan.md), [cache-recovery](../managed-npm-cache-recovery/plan.md),
and [startup-resilience](../managed-npm-startup-resilience/plan.md) packages retain their completed historical records.
This package owns the newly reported gaps and final evidence.
Requirement 005 remains pending in its separate dependency-recovery package.

Run every work order's targeted commands during later implementation and record actual results.
Task 06 owns final real-npm, browser/executor, public-doc, spec, and package-coverage checks.
Run `scripts/lint-go-changed` on the actual changed Go files and record the invocation.
Update commands if the final implementation changes a path or test name.

Before delivery, re-resolve PR head, merge status, and current main.
If still open, the authorized fix can update the PR branch.
If merged or advanced, inspect current source and reconcile the patch before choosing its delivery branch.
Old CI and mergeability do not prove the integrated result.
Do not merge or publish without applicable user authorization.

## Verification results

Planning validation passed on 2026-11-09:

- `python3 scripts/list-docs.py validate`: 441 decisions and 1948 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `validateCoverage` from `.github/scripts/pr-docs.cjs`: six work orders, zero reference errors.
- The coverage preflight includes a projected `prepared_runtime.go` change to exercise the production-documentation gate. That file is unchanged.
- Tracked-document whitespace checks and separate checks for all eight new Markdown files passed.
- The new ADR is discoverable through the decision catalog.

No product build, product test, live-instance operation, or production-code edit occurred during planning. All six work orders were pending at that time; PR CI describes only the reviewed baseline.

## Implementation results

Implemented on 2026-11-09. Recorded evidence:

- Go package suites passed for `managedruntime`, `agents`, `process`, `utility`, `hostutility`, `lifecycle`, `orchestrator/executor`, `task/service`, and `backendapp`.
- Race runs passed for parser, owner, cancellation, publication, and CLI-argument cases.
- `bash scripts/lint-go-changed` reported zero issues for the changed Go files; its five-minute golangci-lint budget elapsed while analyzing the combined package set. A clean sequential `golangci-lint --timeout=15m` rerun over the same changed packages reported zero issues.
- Real npm passed `TestManagedRuntimeRepairReuseNPMWithCLIArgs` with `KANDEV_TEST_REAL_NPM=1`.
- Desktop Chromium and mobile Chrome passed the four `managed npm review fixes` session/settings cases.
- Public-doc validators, catalog validation, specification lint, and whitespace checks passed.
- All six Docker and SSH containers-project cases passed on 2026-11-09 after remediation. Retained-storage cases assert captured provider argv and installer calls, a different task-owned provider PID, required resume-token continuity, unchanged shared/sibling sentinels, bounded provider starts, one accepted follow-up, and the new-container miss.
- New POSIX shell fixtures skip on Windows. Windows cross-compilation of the affected Go test packages passed.

## Review follow-up

The user requested implementation of the accepted suggestions from the independent review.
Task 03 restores the existing one-shot Codex prompt boundary; profile flags remain available to discovery probes.
Task 05 requires context-first internal APIs and narrows version memoization to executable identity and Node execution options.
Its executable-change fixture uses an explicit timestamp instead of a sleep.

Retain caller cancellation for publication validation.
Discarding that cancellation would allow metadata work to outlive the recovery operation.
Retired publication keeps the initialized agent running, consistent with requirement 007.12.
Installation provenance, npmrc, and environment checks still run for every context.

Validation for this follow-up:

```bash
cd apps/backend
go test -trimpath -tags fts5 ./internal/agent/managedruntime ./internal/agentctl/server/utility -count=1
go test -trimpath -tags fts5 ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/hostutility -run 'RepairNPMVersion|RepairContext|ManagedRuntime.*Context|ManagedRuntimePublication|ManagedRuntimeRepairRejectedStart|ManagedRuntime.*CLIArgs|ManagedRuntimeProbe|ManagedRuntimePreparation|ManagedRuntimeCodex|ProfileProbe|ProfileModel|ModelConfig|Profile.*Context' -count=1
go test -trimpath -tags fts5 -race ./internal/agent/managedruntime ./internal/agentctl/server/process ./internal/agentctl/server/utility -run 'RepairNPMVersion|RepairContext|ManagedRuntime.*Context|ManagedRuntimePublication|ManagedRuntimeRepairRejectedStart|CodexAppServerExecuteRejectsProfileArguments|CodexAppServerManagedProbePreservesProfileArguments' -count=1
KANDEV_TEST_REAL_NPM=1 go test -trimpath -tags fts5 ./internal/agent/managedruntime ./internal/agentctl/server/api -run '^(TestManagedRuntimeRepairReuseNPMWithCLIArgs|TestHandleStart_.*|TestHandleWSInitializeCarriesExactProcessEvidence)$' -count=1
```

All four commands passed. The second command compiled API consumers without matching an API test; the fourth ran the focused API tests.
`scripts/lint-go-changed` passed with zero issues for the 14 Go files touched by this follow-up, covering managedruntime, process, and utility.
The recorded browser, container, and Windows checks above belong to the prior implementation; they were not rerun for this follow-up.
Public documentation needs no additional change for these internal corrections.

At follow-up start, PR #5208 remained OPEN at `74d3b2fae82e5e0bb78bda3df1ce8e481ad70d05`, with conflicts.
Remote main had advanced to `ad8348fd455182c3aee92d75bfd18d210ed914e2`.
Changes remain uncommitted. Refresh main, reconcile the branch, and validate the integrated patch before a later push.

## Risks

- Loose recognition can turn agent values or wrapper prefixes into installation authority.
- Default-owner fallback can collapse authenticated users if mode is not explicit.
- Unlocked context completion requires immutable tickets and apply-time identity checks.
- Cached mutable configuration can falsely reuse a repair after registry or credential changes.
- Deleting a live store can break a surviving consumer.
- Current main has advanced and requires later reconciliation.
