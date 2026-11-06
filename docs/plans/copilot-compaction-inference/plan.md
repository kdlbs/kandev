---
created: 2026-11-05
status: implemented
requirements:
  - REQ-UI-CONTEXT-COMPACTION-COUNT-001
system_design:
  - ../../specs/ui/system-design/context-compaction-count.md
legacy_specs: []
---

# Implementation Plan: Copilot compaction inference

## Overview

Prevent ambiguous Copilot usage drops from increasing the session compaction count.
First deliver the backend eligibility path and regression coverage.
Then update the explanation, translations, public documentation, and desktop/mobile checks.

Issue: [kdlbs/kandev#5089](https://github.com/kdlbs/kandev/issues/5089).
The issue is assigned to `carlosflorencio` after confirmation of the repair path.
This package is implemented; permanent regression coverage and verification results are recorded below.

## Confirmed root cause and evidence

Copilot reports parent and child occupancy under one ACP session identifier without context-owner metadata.
`Adapter.convertUsageUpdate` forwards each reading as the latest context window.
The previous `updateSessionContextWindowQuery` counted every strict decrease in that session's stored usage.
The persisted count reaches `ContextCompactionCount` in the chat-input surface.

A temporary Go overlay replayed the reported observations through real SQLite storage on this checkout:

| Observation | Used tokens | Stored count before correction | Prototype result |
| --- | ---: | ---: | ---: |
| Parent | 13,267 | 0 | 0 |
| First child | 2,877 | 1 | 0 |
| Parent | 13,390 | 1 | 0 |
| Second child | 2,877 | 2 | 0 |
| Parent | 13,485 | 2 | 0 |

`TestInvestigation5089CopilotUsageReplay` failed its zero-count assertion with the expected stored total of two.
The separate eligibility prototype passed this sequence, normal eligible decreases, and transitions between excluded and eligible samples.
The existing `TestUpdateSessionContextWindowSQLiteCountsStrictUsageDrops` also passed with the prototype overlay.
These experiments proved the repository mechanism; the implemented publisher-to-storage regression now covers event propagation.
The issue supplies live Copilot capture evidence. This investigation made no new live provider call.
Temporary sources remain outside the repository and are removed after package validation.

The [ACP usage documentation](https://github.com/agentclientprotocol/agent-client-protocol/blob/main/docs/protocol/v1/prompt-turn.mdx#session-usage-updates)
describes context occupancy and cumulative cost separately from compaction events.
The absence of attribution in this Copilot sequence comes from the issue's captured frames.

## Requirement conformance and ownership

The previous active `AC-UI-CONTEXT-COMPACTION-COUNT-001.2` intentionally counted every valid decrease.
This defect exposes an incomplete requirement rather than a violation of that old criterion.
The package narrows eligible comparisons and adds exclusion, accounting, recovery, and historical-count criteria.
The revised requirement and new paired design remain `draft` until implementation passes.

The existing canonical requirement is amended in place. No parallel agent or UI repair requirement is created.
The original completed package remains historical evidence and links to this follow-up.
The separate ACP timeline package does not increment this count and needs no implementation changes here.

## Scope

### In scope

- Exclude current Copilot ACP samples from usage-drop inference.
- Preserve latest usage, accounting, lifetime counts, reset guards, and ordered publication.
- Guard both incoming and previous eligibility in atomic SQLite and PostgreSQL updates.
- Persist the exclusion and support observations from retained older agentctl processes.
- Explain the policy through translated help and the public session guide.
- Cover real backend propagation and both rendered input modes.

### Out of scope

- Exact Copilot compaction counting, a provider-side fix, or historical total repair.
- Reconstructing context identity from token sizes, timing, tool lifetimes, or native files.
- Schema migrations, new settings, release flags, UI layout changes, or accounting changes.
- Counting explicit compaction timeline events.

## Technical approach

### Provider classification and propagation

In `apps/backend/internal/agent/runtime/lifecycle/events.go`, classify eligibility inside `EventPublisher.PublishContextWindow`.
Use `AgentExecution.AgentID` and `agents.CopilotACPAgentID`; the payload's current `agent_id` is an execution identifier.
Add `CompactionInferenceDisabled` to `ContextWindowEventPayload` in `event_types.go` and `watcher.ContextWindowData` in `watcher.go`.
Both use the optional JSON key `compaction_inference_disabled`.
Keep the publisher signature and raw agentctl stream unchanged.

`Service.handleContextWindowUpdated` in `event_handlers_git.go` copies the boolean into the stored context-window sample.
Synchronous persistence, the generation guard, and combined sample/count publication remain the single write path.

### Atomic inference

Update `Repository.UpdateSessionContextWindow` and both SQL dialect branches in `session.go`.
Bind incoming eligibility and inspect the previous persisted marker within the same statement.
Excluded samples and transitions from excluded baselines cannot increment.
Always store the latest usable sample. A missing marker retains legacy eligibility.

### Compatibility matrix

| Provider/transport | Evidence | Policy | Verification and fallback |
| --- | --- | --- | --- |
| Copilot ACP | Trusted provider type, unscoped usage | Disable current inference | Real publisher-to-storage replay. Preserve usage/history, accepting missed genuine compactions |
| Copilot via retained older agentctl | Restored provider type, old context event | Same backend exclusion | Publisher test without a raw exclusion field. No agentctl upgrade dependency |
| Other ACP providers, including Grok metadata usage | Existing normalized context path | Preserve current inference | Ordinary-drop, usage, and source-fallback tests. Remains approximate |
| Other/legacy context streams | No exclusion marker | Preserve legacy eligibility | Missing/false marker tests. No new attribution guarantee |
| Explicit ACP compaction events | Separate timeline identity | Preserve separate path | No new counter increment path |

### Presentation and documentation

Update `contextCompactionCountHelp` locale values and focused rendered assertions.
The existing component and parser need no provider checks or new state fields.
Add zero-count explanation coverage and retain non-zero lifetime-count coverage.
Update the context-window paragraph of `docs/public/sessions-and-review.md` during implementation.

## UI preview

**UI-01: Context-window count and expanded accuracy help.** Entry: the chat-input context ring.
Source evidence shows the stored value reaches the count row. The corrected fresh-session sequence shows `0` instead of `2`.

| Region | Proposed content or behavior |
| --- | --- |
| Header and usage | Existing title, progress, and token/source reading |
| Count row | Compactions: 0, followed by the existing info control |
| Expanded info | Inferred usage drops, the Copilot ACP exclusion, and approximate accuracy |
| Desktop access | Hover the ring, then hover or focus info |
| Phone access | Tap the ring, then tap info inside the pinned surface |
| Dismissal | Existing toggle, outside interaction, and keyboard behavior |

Desktop and phone share this composition. The closest shipped phone exemplar is `mobile-context-window-source.spec.ts`.
This content correction introduces no new overlay, scroll owner, action, or breakpoint behavior.
The token/source row followed by the count row and accessible info are structural requirements.
Wording and whitespace are illustrative. The conversation preview uses the platform's rich-output renderer.
Coverage: `AC-UI-CONTEXT-COMPACTION-COUNT-001.1`, `.6`, `.7`, `.9`, and `.13`.

## Tests

Regression tests are implemented. Backend paths are relative to `apps/backend/`.

| Acceptance criteria | Test path and name | Evidence |
| --- | --- | --- |
| `.2`, `.9`, `.10`, `.12` | `internal/orchestrator/context_window_compaction_test.go`: `TestContextWindowCopilotUsageDoesNotInferCompaction` | Real publisher, watcher decode, handler, SQLite, and session patches for all five samples |
| `.9`, `.10`, `.12` | `internal/agent/runtime/lifecycle/events_context_window_test.go`: `TestPublishContextWindowCompactionPolicy` | Copilot/other providers, restored identity, old normalized event compatibility |
| `.9`, `.12` | `internal/orchestrator/watcher/context_window_test.go`: `TestContextWindowCompactionPolicyRoundTrip` | Marker survives actual watcher JSON conversion |
| `.2`-`.5`, `.8`, `.11`-`.13` | `internal/task/repository/sqlite/session_context_window_test.go`: `TestUpdateSessionContextWindowCompactionEligibility` | Mixed eligible/excluded sequence, duplicates, history/keys, baseline after database reopen |
| `.2`, `.9`, `.11`-`.13` | `internal/task/repository/sqlite/postgres_context_window_test.go`: `TestPostgresUpdateSessionContextWindowCompactionEligibility` | Equivalent real PostgreSQL behavior, environment-gated |
| `.5`, `.10`, `.12` | Existing context/reset/orchestrator and ACP usage tests | Reset, ordered writes, source fallback, and accounting |
| `.1`, `.6`, `.7`, `.13` | `apps/web/components/task/chat/token-usage-display.test.tsx` | Zero/non-zero counts and revised accessible help |

New Go test files avoid growing the existing large `session_test.go` and `event_handlers_git_test.go` files.

## E2E tests

Extend `apps/web/e2e/tests/chat/context-window-source.spec.ts` in project `chromium`.
Extend `apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts` in project `mobile-chrome`.
Use the established shared fixture with zero and assert the revised explanation.
Retain non-zero historical-count coverage. Parameterize the shared count assertion only if needed.
Desktop exercises pointer access. Phone exercises touch toggling and document containment.
These checks cover `.1`, `.6`, `.7`, `.9`'s visible zero outcome, and `.13`'s retained history.
The real backend test supplies counting evidence; a browser store fixture cannot prove the repair.

## Work orders

- [x] [Task 01: Exclude ambiguous Copilot samples](task-01-exclude-copilot-samples.md) — done
- [x] [Task 02: Explain the count and verify desktop/mobile](task-02-explain-count-and-cover-surfaces.md) — done

Execution is sequential. Task 02 depends on Task 01's metadata behavior.
No delegation is authorized by this package.

## Verification results

### Design-time investigation

- Investigation: real SQLite replay failed the intended zero-count assertion, as expected before correction.
- Temporary prototype: the eligibility sequence and existing strict-drop test passed, two tests total.
- Package validation: catalog validation passed (432 decisions, 1,922 specifications), specification-linter tests passed (36), and full specification lint passed.
- Documentation-coverage preflight passed for both work orders against the prospective backend change. Requirement, criterion, and design references are covered.
- Specification/plan whitespace checks passed. Workspace status confirmed the new design and both untracked work orders were present.

### Implementation

- Task 01 RED: with the provider classification and stored eligibility guard reverted, `TestContextWindowCopilotUsageDoesNotInferCompaction` failed before reaching its zero-count assertion. The design-time overlay recorded the pre-fix stored counts `0,1,1,2,2`.
- Task 01 GREEN: `go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestContextWindow' -count=1` passed, including the five-sample Copilot replay with five zero counts, retained usage values, and matching metadata patches.
- Repository eligibility matrix and reopen baseline passed in `./internal/task/repository/sqlite`; the existing strict-drop regression also passed.
- Broader packages passed: `./internal/agent/runtime/lifecycle`, `./internal/orchestrator`, `./internal/orchestrator/watcher`, and `./internal/task/repository/sqlite` (full package runs).
- Go lint: `bash scripts/lint-go-changed` reported 0 issues for all changed backend files.
- PostgreSQL: `TestPostgresUpdateSessionContextWindowCompactionEligibility` passed against a disposable PostgreSQL 16 container, together with the existing `TestPostgresUpdateSessionContextWindowCountsStrictUsageDrops`. The targeted repository command passed all 8 tests.
- Task 02: `pnpm exec vitest run components/task/chat/token-usage-display.test.tsx` passed 16 tests, including zero-count and revised-help assertions.
- Task 02: `pnpm run i18n:check` passed (all seven locales complete, pseudo in sync), `pnpm run i18n:ratchet` passed, focused Oxlint passed, and `pnpm run typecheck` passed.
- E2E: managed Chromium desktop `context-window-source.spec.ts` passed 3 tests; mobile-chrome `mobile-context-window-source.spec.ts` passed 3 tests.
- Public documentation: `node --test scripts/validate-public-docs.test.mjs` passed 63 tests and `node scripts/validate-public-docs.mjs` validated 55 pages.
- Specification lifecycle: `python3 scripts/list-docs.py validate` passed (432 decisions / 1,922 specifications) and `python3 scripts/lint-spec-files.py --all` passed after promotion.
- `git diff --check` passed.
- Limit: no live Copilot call was available. Generated Traditional Chinese catalogs produced
  unrelated `automations.json` drift, which was reverted to keep this change scoped.

Package validation commands, from the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/copilot-compaction-inference docs/plans/context-compaction-count
git status --short -- docs/plans/copilot-compaction-inference
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/copilot-compaction-inference';
const files = fs.readdirSync(dir).map(file => `${dir}/${file}`);
const refs = ['docs/specs/ui/requirements/context-compaction-count.md',
  'docs/specs/ui/system-design/context-compaction-count.md'];
const fileContents = Object.fromEntries([...files, ...refs]
  .map(file => [file, fs.readFileSync(file, 'utf8')]));
const result = validateCoverage({ fileContents,
  changedFiles: ['apps/backend/internal/task/repository/sqlite/session.go', ...files] });
console.log(JSON.stringify(result));
if (!result.ok) process.exitCode = 1;
JS
```

Exact implementation commands are recorded in the work orders. A PostgreSQL skip is never parity evidence.
The requirement is now `active`, the design is `current`, and this plan is `implemented`.
Keep original package results intact. New results are recorded here and in the relevant work order.

## Risks and alternatives

- Exclusion undercounts genuine Copilot compactions until usable context attribution exists.
- Existing false-positive totals remain visible because retained totals cannot identify incorrect historical increments.
- A dropped event field restores the bug. The regression must exercise serialization rather than direct map input alone.
- Thresholds and open-card gating remain ambiguous with concurrent or delayed samples, so this package rejects those heuristics.
- Explicit-event counting needs provider support and replay/dedup semantics beyond this repair.
- High-water usage or discarded child-like samples distort current usage and can affect accounting.
- No fresh live-provider run occurred here; the runtime propagation is covered by the real publisher-to-storage regression instead. PostgreSQL parity was verified against a disposable PostgreSQL 16 container.

## Native-event follow-up

The [native Copilot counting package](../copilot-native-compaction-count/plan.md) adds attributed, successful completion evidence.
It retains this package's usage-drop exclusion and completed verification history.
Its separate ledger, qualification gates, reporting states, and new checks belong to that follow-up.
