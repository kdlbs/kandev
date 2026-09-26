---
created: 2026-09-24
status: in_progress
requirements:
  - REQ-AGENTS-CODEX-NATIVE-001
  - REQ-AGENTS-CODEX-NATIVE-002
  - REQ-AGENTS-CODEX-NATIVE-003
  - REQ-AGENTS-CODEX-NATIVE-004
  - REQ-AGENTS-CODEX-NATIVE-005
  - REQ-AGENTS-CODEX-NATIVE-006
  - REQ-COSTS-CONVERSATION-USAGE-001
  - REQ-COSTS-CONVERSATION-USAGE-002
  - REQ-COSTS-CONVERSATION-USAGE-003
  - REQ-COSTS-CONVERSATION-USAGE-004
system_design:
  - ../../specs/agents/system-design/codex-app-server.md
  - ../../specs/costs/system-design/conversation-usage.md
legacy_specs:
  - ../../specs/task-cost-ledger/spec.md
---

# Implementation plan: Native Codex app server

## Overview

Deliver a separate native Codex agent, shared protocol diagnostics, and built-in usage visibility.
Keep Codex ACP available for comparison. Work orders are implemented sequentially in the primary session.
No work order authorizes delegation.

## Inputs and ownership

- [Agent requirements](../../specs/agents/requirements/codex-app-server.md) and [design](../../specs/agents/system-design/codex-app-server.md).
- [Usage requirements](../../specs/costs/requirements/conversation-usage.md) and [design](../../specs/costs/system-design/conversation-usage.md).
- [Normalization decision](../../decisions/2026-09-24-native-codex-normalization.md).
- Existing [Codex ACP decision](../../decisions/0034-agentclientprotocol-codex-acp.md) remains in effect for `codex-acp`.

Agents owns the native provider and inspection workflow. Costs owns independent accounting and its chat projection.
Tasks supplies existing ownership, session persistence, and the ledger repository.
This package extends the existing ledger; it does not create another writer or billing engine.

## Scope

In scope: gated profiles, native structured chat, permissions, normalized messages, children/background activity, usage, shared-workspace forks, and `codexdbg` plus its skill.
The initial reviewed binary is 0.154.0, whose schema was inspected locally without a model turn.
Actual availability of internal-only response usage remains an implementation compatibility check.

Out of scope: automatic ACP migration, arbitrary live-process attachment, external history import, account billing management, and fork-created Git worktrees.
Public documentation changes land with implementation, not with this draft package.

## Technical approach

1. Build `pkg/codexappserver` and `codexdbg` against a versioned schema and a fake server. This establishes evidence before adapter integration.
2. Add `CodexAppServer`, managed native runtime metadata, runtime flag wiring, and an adapter in the existing process/executor path.
3. Persist native item/thread bindings and reconcile children independently from root turns.
4. Add attributed usage observations to the existing writer, shared pricing arithmetic, Office consumer, and authorized read projections.
5. Render built-in turn/session detail from committed ledger data, with no plugin dependency.
6. Add native forks through a completed turn with explicit shared-file semantics and idempotent session creation.
7. Record real protocol evidence, executor support, documentation, and scoped harness guidance before handoff.

The gate is `features.codexAppServer` / `KANDEV_FEATURES_CODEX_APP_SERVER`, off in prod, dev, and e2e.
It is restart-required. Existing session/profile data survives disablement.
The standalone developer debugger can inspect an explicitly selected binary without enabling the application feature.

## ASCII UI preview

Labels and amounts below are illustrative. Grouping, scope labels, visible actions, and phone presentation are required.
All final copy uses localization. Existing chat remains the primary scroll owner.

### UI-01: Profile creation

Entry: Settings > Agents, feature enabled. Reuse the existing profile form on desktop and phone.

```text
Agent type [Codex app server v]
Profile    [Native coding       ]
Model      [Provider models    v]
Permissions [Existing controls ]
                         [Save]
```

Disabled: hide the agent from new selections, but show saved native profiles as unavailable with the feature-toggle explanation.
Loading model catalogue: show a loading state and prevent an invalid save. Authentication errors expose the existing login action.
Phone uses the existing single-column form and touch-sized controls. No new desktop dialog is squeezed into a phone sheet.

### UI-02: Native activity

Entry: chat transcript and background summary after the parent replies.

```text
Desktop chat
  Codex reply ...
  [Usage: 1.2K tokens, estimated $0.02] [More v]
  Background work: 1 agent, 1 command  [View]
    Explore parser     Running         [Open]
    Test command       Running         [Output]

Phone chat
  Codex reply ...
  [Usage]  [More]
  [Background work: 2 >]
        tap child -> full-height child history
  [Back] Explore parser
  | child transcript (one scroll body) |
```

Parent completion leaves active rows visible. Disconnection shows unknown status until reconciliation.
Child history on phone uses a fixed back/header region and one dynamic-height scroll body with safe-area clearance.
Do not depend on hover to inspect a child or command.

### UI-03: Usage detail

Entry: a turn footer or the session Usage control. Relevant to all providers with recorded ledger data.

```text
Desktop turn popover              Phone inset drawer
This turn                        [Usage                 Close]
  1,200 tokens                   This turn: 1,200 tokens
  Estimated cost $0.02           Estimated cost: $0.02
Last response: 500 tokens        Last response: 500 tokens
Direct / children               Direct / children
Input 1,000                     Input 1,000
  Cached 600                      Cached 600
Output 200                      Output 200
  Reasoning 80                    Reasoning 80
Price: catalogue estimate        Price: catalogue estimate
Session recorded total          Session recorded total
                                (one scrolling body)
```

Optional thread provider estimate has its own scope label and observed time.
Pending: `Usage pending`. Missing price: `Cost unavailable`. Partial data: `Partial usage`.
These states do not replace measured zero or a known prior value.
The phone drawer follows `MobilePickerSheet` geometry and `useTouchDrawer` pointer selection.
It has a fixed heading, 44px touch controls, one scroll body, and bottom safe-area padding.
Desktop controls remain 28px. A long response list stays inside the disclosure, without horizontal page overflow.

### UI-04: Fork through completed turn

Entry: completed-turn More menu on desktop or phone.

```text
[Fork conversation]
Create another session through this turn.
Files stay shared in this workspace.
                       [Cancel] [Fork]
```

Phone uses the existing responsive menu and confirmation surface, with one primary action and focus return.
Active source or child work disables Fork with a reason. Failure retains the source and reports the failed/uncertain operation.
Success selects the new session. Copied history displays without new recorded cost.

## Tests

Evidence below reflects the implemented test files. Native adapter tests use a fake app-server; shared web E2E tests use the deterministic mock agent where noted.
Each work order lists its targeted commands and the acceptance IDs it owns.

| Evidence location and test | Acceptance coverage |
| --- | --- |
| `pkg/codexappserver/client_test.go`, `client_fork_test.go`, and `schema_test.go` | AGENTS 002.3, 002.4, 005.3 |
| `internal/agent/codexdbg/*_test.go` and `cmd/codexdbg/main_test.go` | AGENTS 005.1-005.5 |
| `internal/agent/agents/codex_app_server_test.go`, registry, profile, and runtime-flag tests | AGENTS 001.1-001.4 |
| `transport/codexappserver/adapter_test.go` | AGENTS 002.1-002.4, 003.1-003.4 |
| `transport/codexappserver/usage_test.go`, `internal/task/usage/*_test.go`, and `internal/task/dto/usage_turn_test.go` | COSTS 001.1-001.5, 002, 004 |
| SQLite usage-event repository and handler tests | COSTS 001-004 |
| `transport/codexappserver/fork_test.go` and `internal/orchestrator/conversation_fork_test.go` | AGENTS 004.1-004.4 |

Prefixes abbreviated in this table mean `AC-AGENTS-CODEX-NATIVE-*` and `AC-COSTS-CONVERSATION-USAGE-*`.
Repository integration tests cover SQLite and PostgreSQL migration, mixed ACP/native rows, totals, and duplicate observations.
Office tests prove that response observations cannot double-charge a terminal summary.

## E2E tests

Use new deterministic app-server fake scenarios through the real native adapter, not only synthetic frontend events.
Keep the production feature defaults off; fixtures explicitly enable native support.

| Files under `apps/web/e2e/tests/` | Projects | Acceptance coverage |
| --- | --- | --- |
| `settings/codex-app-server-profile.spec.ts`, `settings/mobile-codex-app-server-profile.spec.ts` | chromium, mobile-chrome | AGENTS 001, 006 |
| `chat/codex-app-server.spec.ts`, `chat/mobile-codex-app-server.spec.ts` | chromium, mobile-chrome | AGENTS 004, 006; shared fork UI |
| `chat/subagent.spec.ts`, `chat/mobile-subagent.spec.ts` | chromium, mobile-chrome | Shared child activity UI; these use the mock agent, not the native transport |
| `conversation-usage.spec.ts`, `mobile-conversation-usage.spec.ts` | chromium, mobile-chrome | COSTS 001-004 |

Native child/background protocol behavior is covered by transport fake-server tests. A native app-server-to-browser child E2E scenario is not implemented; real Codex compatibility remains a Task 07 gate.

Usage scenarios explicitly disable Office and install no session-cost plugin.
Cover two responses in one turn, reload, replay, late child usage, missing prices, and unavailable response detail.
Phone scenarios assert drawer/history composition, touch targets, focus return, and viewport containment.
Run managed E2E commands from each work order to build fresh assets and limit workers.

## Work orders

- [x] [01: Shared native client and debugger](task-01-client-and-debugger.md)
- [x] [02: Gated native profiles and conversations](task-02-native-conversations.md)
- [x] [03: Child and background lifecycle](task-03-background-lifecycle.md)
- [x] [04: Attributed usage and cost accounting](task-04-usage-accounting.md)
- [x] [05: Built-in chat usage](task-05-usage-display.md)
- [x] [06: Native conversation forks](task-06-conversation-forks.md)
- [ ] [07: Compatibility evidence and documentation](task-07-compatibility-and-docs.md)

## Verification results

Design validation: catalog validation, all 36 specification-linter tests, and full specification lint passed.
Local link, acceptance-reference, and whitespace checks passed for all 13 design artifacts.
Tasks 02-06: scoped backend, frontend, unit, race, and desktop/mobile E2E checks passed. PostgreSQL usage/migration coverage passed against a temporary PostgreSQL 16 container. Web typecheck, lint, i18n check, and ratchet passed.
Tasks 01-06 are complete. Task 07 remains pending. The authenticated Codex 0.154.0 run covered initialization, prompt, resume, fork, shared-workspace behavior, and background completion. It did not observe an approval request or exact response usage, and it did not establish child-to-collaboration-call correlation. A later follow-up added fake-server and desktop/mobile coverage for native `item/tool/requestUserInput` clarification routing; direct live question behavior remains unverified.
The Docker, SSH, and Kind matrix passed with a fake app-server. These tests cover executor launch and profile gating, not live Codex compatibility inside each executor. PostgreSQL usage and migration coverage passed on PostgreSQL 16.

## Risks and bounded choices

- Internal-only response events can disappear. Fall back to labeled turn measurements without claiming response precision.
- `usageMetadata.amount` lacks proven currency. Preserve it only in diagnostic evidence until its meaning is established.
- The optional thread USD estimate depends on upstream billing routes. Absence does not block token accounting or calculated cost.
- Initial forks share the existing workspace. New-worktree forks require a separate task/workspace design.
- Additive observation handling must reach both task and Office consumers before native usage publication.
- Resume can replay historical token snapshots. Baselines and durable observation IDs prevent duplicate charges.
- Live tests require an authenticated disposable runtime. Record a blocker rather than claiming fake-server results prove native availability.

## Final PR delivery (2026-09-25)

The user-authorized protocol follow-up is implemented in PR #3916. Commit `4795ed8244184e421d3a7d2a6b8379e1f15628f9` merged the current `main`, resolved the LSP release-order conflict, and included the final plan/work-order reconciliation. The WebSocket race suite, full i18n check, focused E2E lint, and commit hooks passed. Exact-head `scripts/pr-await 3916` reported 60 passed, 0 failed, and 0 pending; review threads are empty and GitHub reports mergeable/clean. The PR remains open.

Task 07 remains pending. The authenticated Codex 0.154.0 run did not produce an approval or direct-question request, exact response usage, child-to-collaboration-call correlation, or provider thread estimate. Docker, SSH, and Kind coverage used a fake app-server and does not establish live Codex compatibility in those executors.
