---
created: 2026-11-05
status: in_progress
requirements:
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-001
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-002
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-003
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-004
  - REQ-UI-CONTEXT-COMPACTION-COUNT-001
  - REQ-AGENTS-ACP-SESSION-EVENTS-002
system_design:
  - ../../specs/agents/system-design/copilot-native-compaction.md
  - ../../specs/ui/system-design/context-compaction-count.md
  - ../../specs/agents/system-design/acp-session-events.md
legacy_specs: []
---

# Implementation Plan: Native Copilot compaction counting

## Overview

Restore useful Copilot compaction counts from successful parent native events while retaining the existing usage-drop exclusion.
First qualify the protocol and add narrow SDK dispatch support.
Then normalize attributed observations, implement durable count projection, and prove the visible result through real desktop/mobile flows.
Three of four work orders are implemented on this branch. Task 03 stays blocked until real PostgreSQL parity evidence runs; the package records that qualification limit explicitly.

Branch: `feature/copilot-native-compaction-count`.
Base: fetched `origin/main`, `ce9918b094855c38e5b3b4c79c1b565fe5c379cb`.
The base contains merged PR #5093 (`474c0b42ac4efe7e223bb52c00cde55888dd9755`).
Investigation: [issue #5089 follow-up](https://github.com/kdlbs/kandev/issues/5089#issuecomment-6672451872).

## Confirmed facts and assumptions

Confirmed user intent: create a fresh branch from main and prepare a new package for meaningful native-event counting.
Implementation was performed sequentially in the workflow-selected session without delegation.

Verified on this base:

- Copilot usage-drop inference is excluded in `lifecycle/events.go` and the stored eligibility guards.
- The current managed pin is 1.0.88. The reported live native capture used 1.0.103.
- The ACP SDK pin is `v0.13.6-0.20261101161427-70cc752b95c6`.
- Its `extensions.go` rejects unprefixed notifications before the existing Kandev callback.
- `handleExtensionNotification` currently routes Claude messages only.
- Native notifications need the same FIFO/barriers as `enqueueACPUpdate`.
- Canonical delivery projection commits before ordinary lifecycle callbacks. A callback-only counter can lose an increment during recovery.
- Harness generation records and native rebind history provide persisted native identity.

The comment captured zero compactions for two child calls and one successful applied manual compaction.
Its completion supplied an attempt ID but no required checkpoint number.
The stream is not historical backfill: native load did not replay those notifications.
The reported implementation limits payloads to 32 KiB and in-flight notifications to 256.
Those are investigation evidence, not guarantees of future versions.

Planning decisions:

- Require explicit applied success and attempt identity, including on older versions. Do not guess optional-field fallback behavior.
- Keep all Copilot usage-drop inference disabled, including root-attributed usage.
- Preserve historical totals. New increments use provider evidence, but the aggregate can contain old estimates.
- Count journaled completions in canonical SQL projection, including recovery with callbacks suppressed.
- Expose unavailable and known partial coverage without claiming that silence proves no compaction.

Qualification remains implementation work. Task 01 must resolve minimum-version support and automatic-compaction evidence before enabling subscriptions.
These are evidence gates with conservative fallbacks, not unresolved product choices.
No fresh model run, SDK build, product build, or product test is needed to review this design package.

## Ownership and requirements

The new provider contract belongs to Agents: native attribution and applied-outcome evidence determine which observations are authoritative.
Its vertical requirement/design pair includes task persistence and visible context-window behavior.
The existing UI pair remains the shared count/presentation contract and is amended without copying native acceptance rules.
The standard ACP timeline requirement is narrowed so its generic rows do not acquire counting authority.

Decision: [native compaction evidence](../../decisions/2026-11-05-copilot-native-compaction-evidence.md).
References: [provider requirements](../../specs/agents/requirements/copilot-native-compaction.md),
[provider design](../../specs/agents/system-design/copilot-native-compaction.md),
[shared count](../../specs/ui/requirements/context-compaction-count.md), and
[standard timeline](../../specs/agents/requirements/acp-session-events.md).

## Scope

### In scope

- Opt-in exact vendor notification dispatch and provider-scoped native subscriptions.
- Root completion validation, root context usage, and independent existing accounting.
- Successful manual and automatic compactions under one acceptance rule, with qualification evidence.
- Durable receipt identity, atomic count/cursor projection, replay, reset/rebind ownership, and both database dialects.
- Existing timeline normalization as a separate presentation consumer.
- Coverage metadata, localized explanation/status, and real desktop/mobile counter tests.
- Compatibility with older providers, backend/agentctl combinations, and retained historical counts.

### Out of scope

- Historical repair/backfill, private native-log readers, usage-drop fallback, and child counts in the parent total.
- Changing standard ACP counter semantics or other providers' inference behavior.
- Provider billing changes, new compaction controls, release toggles, or general RPC permissiveness.
- Broad local verification, publishing a new SDK version during planning, or reopening old completed work orders.

## Technical approach

### 1. Transport and version qualification

In the separate `kdlbs/acp-go-sdk` fork, introduce opt-in exact inbound notification dispatch.
Keep underscore-only defaults, request behavior, known standard routes, and outbound validators intact.
Merge/release the SDK change before importing an immutable replacement into this repository.
The work order names proposed tests; no unpublished SHA is invented in this package.

Qualify native subscription, parent/child identity, required outcome fields, data omission, and the automatic producer path for the selected CLI pin.
Use isolated `COPILOT_HOME`, a temporary work directory, exact executables, and synthetic prompts.
Store only sanitized wire fixtures and version/source evidence. Do not commit raw private captures.
If necessary, update only `@github/copilot` in `managed_npm_runtime_versions.json`.

### 2. Ordered normalization

Extend live `clientCapabilitiesForAgent` for Copilot when the backend/agentctl negotiated native support.
Subscribe only to compaction start/completion and attributed usage.
Add exact-method opt-in at SDK connection construction and a Copilot decoder beside `dialect_copilot.go`.
Queue raw vendor work through the current `notifWork` owner rather than processing it beside the FIFO.
Validate provider, method, native conversation, root attribution, typed fields, and applied success.

Emit a typed `context_compaction_observation` event for accepted completions or reporting changes.
Reuse the existing timeline converter for parent lifecycle display, without making tool rows countable.
Use root `session.usage_info` for gauge events. Keep ordinary usage/cost tracker updates before suppressing their competing gauge output.
Wire optional support through `CreateInstanceRequest`, instance/process/shared configuration, and `/identity` capability metadata.
The capability remains optional for retained-process adoption.

### 3. Canonical count projection

Create `task_session_compactions` in `session_compactions_schema.go` and initialize it after session/continuity schema.
Unique identity: Kandev session + provider + native session + attempt ID.
Retain receipts until task-session deletion. Record first accepted delivery provenance without making stream/process ID part of deduplication.

Extend canonical single/batch/combined projection with one transaction-owned helper in `session_compactions.go`.
Validate authenticated provider/native generation or rebind history, lock the session row, insert the receipt, and increment only on insertion.
Count, receipt, reporting metadata, generic effect, and projection cursor commit together.
Keep new observation payloads protected from generic receipt retirement until separately proven.
Recovery without callbacks still projects the count; raw stale observations cannot cross native replacement.

Publish the committed metadata through existing session state updates.
No tool callback or frontend arithmetic performs the authoritative increment.
An unavailable durable/native-capability combination retains totals and the usage exclusion.

### 4. Reporting and visible result

Add optional `context_compaction_reporting` with source, availability, observed/partial coverage, and an atomic revision.
Preserve it through the context parser, count-only patches, hydration, and session hook.
Never create a zero-token gauge to carry a count-only update.
Reject stale reporting revisions and merge lifetime totals monotonically so delayed gauge/HTTP updates cannot undo newer increments.
Known partial coverage is sticky for the session lifetime. A later valid event does not repair missing history.

Use the current count row/info control. Add a short Partial or Unavailable status only when applicable.
Supported help explains observed successful parent increments, retained historical estimates, and missed-event limits.
No label calls the whole historical aggregate a confirmed or complete total.
Update all seven languages, generated Traditional Chinese/pseudo, and the public session guide during implementation.

## Compatibility matrix

| Combination | Collection/count policy | Required evidence and fallback |
| --- | --- | --- |
| Qualified Copilot + new agentctl + new backend | Native root events, durable receipts, inference disabled | Manual/automatic qualified fixtures, real wire and SQL tests |
| CLI 1.0.88 | Unsupported: generated schema lacks attempt identity and outcome | Runtime qualification rejects it; native count is unavailable and historical totals remain |
| Current 1.0.103 pin | Qualified from version-pinned schema and sanitized fixtures | Manual applied completion and root usage observed; automatic/omitted/failure behavior qualified from the pinned schema |
| Missing outcome or attempt ID | No increment | Partial when a potentially lost root completion is recognized; no checkpoint fallback |
| Old retained agentctl or incompatible durable path | Native count unavailable, total retained | No compulsory process replacement or unsafe counting fallback |
| New agentctl with old backend | Subscription off when opt-in absent | Existing behavior preserved |
| Other ACP providers | Existing inference and standard timeline | No Copilot method subscription or new counter authority |
| Provider-omitted or dropped notifications | Count accepted evidence only | Sticky known partial status; always disclose potentially missed events |

## UI preview

**UI-01: Context-window count with native coverage.** Entry: the existing chat-input context ring.
The composition is shared across desktop and phone. The nearest phone exemplar is `mobile-context-window-source.spec.ts`.
The conversation preview uses the platform rich-output renderer.

| State | Count region | Explanation |
| --- | --- | --- |
| Before the native follow-up | Compactions: 0, info | Copilot usage drops excluded |
| Available, one applied parent compaction | Compactions: 1, info | New increments use observed parent outcomes; old estimates can remain |
| Known omitted evidence | Compactions: N, Partial, info | Some compactions can be missing; later events do not repair the gap |
| Unsupported runtime | Compactions: N, Unavailable, info | Historical count retained; zero does not prove no compaction |
| Existing historical total 2 plus one native event | Compactions: 3, info | Historical estimates remain; this is not three confirmed native events |

Token/source usage remains above the count row. There is no new route, overlay, scroll owner, or primary action.
Desktop opens the ring by hover and the explanation by hover/focus.
Phone taps the ring and info, then uses the existing toggle/outside dismissal.
The status and explanation must remain readable without horizontal overflow in every supported language.
The shared hierarchy and accessible status are requirements; wording and whitespace are illustrative.
Coverage: provider requirement 004 and `AC-UI-CONTEXT-COMPACTION-COUNT-001.1`, `.6`, `.7`, `.8`, `.13`.

## Tests and acceptance mapping

Proposed permanent tests are named here for a later TDD implementation.
Backend paths are relative to `apps/backend/`.

| Criteria | Test and boundary |
| --- | --- |
| Native 001.1, 004.7 | Kandev `internal/agentctl/server/acp` `TestClientAllowedExtensionNotifications` and adapter `TestCopilotNativeCapabilitySubscription`: exact raw method accepted only when opted in; defaults and other providers unchanged |
| Native 001.2-.6 | `internal/agentctl/server/adapter/transport/acp/copilot_native_compaction_test.go`: `TestCopilotNativeCompletionAcceptance`; root/child, completion without start, omitted/null/unknown fields, failure/non-applied outcomes |
| Native 001.4, 002.4 | Same package `TestCopilotNativeSessionBindingBuffersUntilBound`, `TestCopilotNativeBindingBuffersAcrossNewSessionReplacement`, and `TestCopilotNativeBindingBarrierDrainsBeforeNewerWork`: deterministic creation/load/reset queue barriers, stale events, shutdown, no rebinding to a successor |
| Native 003.1-.4 | `TestCopilotNativeRootUsage`: alternating parent/child data, raw usage after native usage, accounting and root-gauge independence |
| Native 002.1-.8 | `internal/task/repository/sqlite/session_compactions_test.go`: `TestSessionCompactionsAtomicProjection`; rollback, duplicate/cross-stream/native IDs, legacy total, concurrent context writes, deletion, reopen |
| Native 002.1-.8 | `session_compactions_postgres_test.go`: `TestPostgresSessionCompactionsAtomicProjection`; real multi-connection parity and lock order (blocked until a DSN runs it) |
| Native 002.3-.4 | `internal/agent/runtime/lifecycle/copilot_compaction_recovery_test.go`: `TestCopilotCompactionProjectionOnlyRecovery`; journal replay with callbacks suppressed, rebind history, metadata hydration |
| Native 004.1-.7, shared UI | Parser/WS/hook/component tests for reporting defaults, count-only updates, historical totals, partial/unavailable status, translated info |
| Native 004.6, standard ACP 002.8 | Existing standard compaction tests plus Copilot timeline fixtures; one row per identity, no generic timeline counter increment |

Preserve existing strict-drop and safe-Copilot-exclusion tests. They remain regression gates.
Data omission and notification loss cannot be converted into guessed increments or complete-history claims.

## Real E2E evidence

Add `copilot-native-compaction.spec.ts` and `mobile-copilot-native-compaction.spec.ts` under `apps/web/e2e/tests/chat/`.
Use an isolated mock provider registered as `copilot-acp` through the existing `KANDEV_MOCK_PROVIDERS` mechanism.
The raw peer must emit the exact unprefixed notification method and require the initialize subscription.
A fixture-only raw JSON-RPC mode in `cmd/mock-agent` can supply this without weakening SDK outbound extension rules.
Use one serialized writer for all fixture notifications and responses.

The scenario emits alternating parent/child usage, two child calls, one applied parent completion, and a duplicate completion.
Assert SQL/session metadata and the visible count remain zero for child calls, then become one and stay one for the duplicate.
Assert the gauge remains on parent usage. Refresh and restart the isolated backend against the same data, then confirm the retained count.
Also cover a retained historical total, completion before the first gauge sample, partial evidence, and unavailable collection.
Desktop and phone expose the same result; phone checks status/help containment and touch dismissal.
Do not use `__KANDEV_E2E_STORE__` count injection as proof of this feature's wiring.

## Work orders

- [x] [Task 01: Qualify the protocol and narrow SDK dispatch](task-01-qualify-native-transport.md)
- [x] [Task 02: Normalize native events and root usage](task-02-normalize-native-events.md)
- [ ] [Task 03: Persist deduplicated count projection](task-03-project-durable-counts.md) (blocked on PostgreSQL parity evidence)
- [x] [Task 04: Expose coverage and prove real desktop/mobile flows](task-04-report-coverage-and-e2e.md)

Dependency order was 01 -> 02 -> 03 -> 04. Work was sequential.
Task 01 published the SDK prerequisite and imported the immutable pseudo-version before the consumer work landed.
The work orders name exact checks; commits, pushes, and SDK publication remain a separate delivery decision.

## Verification results

Planning evidence: issue comment and native schema read; local SDK filtering, current pins, FIFO, canonical projector, and native rebind boundaries inspected.

Implementation completed on branch `feature/copilot-native-compaction-count`:

- SDK: `kdlbs/acp-go-sdk` commit `ec6d622b199996ceb8f44544129f7895d88e7f38` adds `WithAllowedExtensionNotifications`; Kandev pins `v0.13.6-0.20261106000654-ec6d622b1999`.
- Qualification: CLI 1.0.88's generated schema lacks `compactionAttemptId` and `outcome`; CLI 1.0.103's schema contains both, plus `CompactionTrigger` values `threshold`, `context_limit_retry`, `manual`, `memory_pressure`, and `model_switch`. The managed Copilot pin moved from 1.0.88 to 1.0.103. Sanitized fixtures are under `apps/backend/internal/agentctl/server/adapter/transport/acp/testdata/copilot-native/`.
- Backend: provider-scoped exact-method dispatch, ordered root/child normalization, root usage gauge with retained accounting, durable `task_session_compactions` receipts, atomic SQLite/PostgreSQL projection, sticky reporting metadata, and recovery without callbacks.
- Frontend: reporting-aware parser and hook, monotonic count merge, count-only patches, Partial/Unavailable status, seven-language copy, generated Traditional Chinese and pseudo catalogs, and the public session guide.
- Tests: focused Go packages, 124 component/unit tests, i18n check and ratchet, typecheck, focused Oxlint, and desktop/mobile Playwright suites pass. The PostgreSQL regression test is present and skipped without `KANDEV_TEST_POSTGRES_DSN`.
- E2E: desktop passed 6 tests (`copilot-native-compaction.spec.ts` 2, `context-window-source.spec.ts` 3, `context-compaction.spec.ts` 1). Phone passed 5 tests (`mobile-copilot-native-compaction.spec.ts` 1, `mobile-context-window-source.spec.ts` 3, `mobile-context-compaction.spec.ts` 1).

Review remediation is included: retained predecessor identity after rebind, native-gauge/accounting separation, explicit clear with retained totals, capability-gated launch opt-in, runtime-version qualification, sticky partial for unusable evidence and binding overflow, atomic binding/drain ordering, historical availability isolation with downgrade, monotonic session-metadata/hydration merges, atomic vendor-subscription state, executor-wide opt-in propagation, advisory-lock ordering before session metadata reads, native-identity current-availability checks, later compaction metadata in batch notifications, and authoritative session timestamps. Focused regressions cover each schedule.

Limitations: no live Copilot account call was available here. Automatic-compaction execution is qualified from the pinned 1.0.103 schema and the shared completion rule, not a live automatic capture. No PostgreSQL execution was available. Historical inflated totals remain unchanged.

Package validation commands, from the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans/copilot-native-compaction-count docs/plans/copilot-compaction-inference
git status --short --untracked-files=all -- docs/plans/copilot-native-compaction-count
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/copilot-native-compaction-count';
const files = fs.readdirSync(dir).filter(file => file.endsWith('.md')).map(file => `${dir}/${file}`);
const refs = [
  'docs/specs/agents/requirements/copilot-native-compaction.md',
  'docs/specs/agents/system-design/copilot-native-compaction.md',
  'docs/specs/ui/requirements/context-compaction-count.md',
  'docs/specs/ui/system-design/context-compaction-count.md',
  'docs/specs/agents/requirements/acp-session-events.md',
  'docs/specs/agents/system-design/acp-session-events.md',
];
const fileContents = Object.fromEntries([...files, ...refs].map(file => [file, fs.readFileSync(file, 'utf8')]));
const result = validateCoverage({ fileContents,
  changedFiles: ['apps/backend/internal/task/repository/sqlite/agent_delivery_projection.go', ...files] });
console.log(JSON.stringify(result));
if (!result.ok) process.exitCode = 1;
JS
```

The preflight uses a proposed production path because a docs-only exempt verdict does not prove work-order reference coverage.

After all work orders pass, update their exact results, promote the new/changed paired requirements and designs, and mark this plan implemented.
Do not mark qualification or PostgreSQL parity passed when unavailable or skipped.
Keep the completed prior package's statuses and verification history unchanged.

## Risks and open evidence

- CLI support floor is 1.0.103 (the pinned runtime); automatic producer behavior is qualified from the pinned schema only, with no live automatic capture. Record any live evidence when available.
- The SDK change is merged and pinned at `ec6d622b199996ceb8f44544129f7895d88e7f38` (`v0.13.6-0.20261106000654-ec6d622b1999`); confirm its published state before release.
- Optional fields cannot be assumed present on older/newer versions. Unknown forms fail closed with honest coverage.
- Provider drops occur before Kandev's journal, so durable downstream replay cannot restore them.
- Historical totals can already be inflated. The new ledger prevents future duplicate events but does not repair past estimates.
- Native rebinds can change the native ID within one generation. Current-token-only validation would reject valid accepted history or misbind stale events.
- Count-only updates can race gauge hydration. Parser/hook tests and real E2E must preserve both independently.
- New observations remain protected retention barriers. Ledger storage lasts until session deletion and scales with accepted compactions.
