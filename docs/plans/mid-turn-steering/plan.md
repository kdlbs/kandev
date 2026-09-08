---
spec: docs/specs/platform/requirements/mid-turn-steering.md
created: 2026-08-04
status: complete
---

# Implementation Plan: Mid-turn steering

## Overview

Deliver operator input into a turn that is still generating, for agents that
negotiate the capability, by extending the prompt-handoff machinery that already
exists for the foreground-idle case rather than adding a new concurrency path.

The design rests on three observed protocol facts, each reproducible with the
probes committed alongside this plan:

1. **The Claude CLI folds a queued stream-json message into a running turn.** On
   2.1.220, a message pushed mid-turn is acknowledged `queued` immediately, then
   drains into the *running* turn at the next tool boundary (`started` 27.3s in,
   before the single `result` at 37.5s). One `result` answered two prompts and
   the model obeyed the second — it skipped the tool the first prompt had asked
   for. Reproduce: `scripts/probe-claude-midturn-fold`.
2. **The ACP bridge accepts the concurrent `session/prompt` and hands the turn
   off.** The predecessor's request resolves `end_turn` **3 ms after the tool
   completes and 1.9 s before the steered answer exists**, with zeroed usage;
   the answer and the real usage land on the successor. Reproduce:
   `scripts/probe-acp-midturn-fold`.
3. **Background work survives the handoff boundary.** A `run_in_background`
   shell outlived both prompt resolutions and, 64 s after the second resolved,
   woke a cost-bearing model cycle with no prompt in flight. Its launch tool card
   had reported `completed` 92 s before the workload actually finished, and the
   workload's completion produced **no** `tool_call_update` — only assistant
   text. Reproduce: the `--prompt-a` variant documented in that probe's header.

Fact 2 is why this is primarily an *attribution* change, not a concurrency one.
Kandev already tolerates two overlapping `session/prompt` RPCs with one logical
owner: `promptGate` token transfer, `claimPromptTurnCompletion`, and
`protectActiveBackgroundWorkForHandoff` were built for exactly this in ADR-0049.
What is missing is a trigger that fires while the foreground is *generating* —
today `markPromptHandoff` only fires on `EventTypeForegroundIdle`.

## Capability negotiation correction

ADR-0049 rejects "a central agent-name whitelist" because "an agent identity
does not prove that the installed adapter/provider version emits the lifecycle
frames Kandev expects". Two shipped call sites are exactly that whitelist:

- `adapter_prompt.go:supportsPromptHandoff` — `agentID == claudeAgentID || mockAgentID`
- `turn_activity.go:claudeBackgroundPromptHandoffEnabledForSession` — `agent_name == "claude-acp" || "mock-agent"`

The ACP `initialize` response carries the correct negotiated signal, and the
adapter already retains it: `agentCapabilities._meta.claudeCode.promptQueueing:
true`, reachable as `a.capabilities.Meta` (the pinned `acp-go-sdk` exposes
`AgentCapabilities.Meta map[string]any`). Verified present on bridge 0.49.0.

Note what this signal does and does not assert. It asserts the bridge accepts a
prompt while one is in flight — the necessary precondition. It does **not**
assert mid-turn folding, which is version-dependent, undocumented, marked
`@internal`, and **not visible over ACP at all** (`msg_lifecycle_v1` appears
zero times in both bridge logs; it is consumed inside the bridge). Therefore
delivery is opportunistic and both outcomes must be correct, per the spec.

Replacing the whitelist is a prerequisite for this feature and simultaneously
brings the shipped foreground-idle experiment into line with its own ADR. It
narrows eligibility correctly rather than widening it: a bridge too old to
advertise the capability becomes ineligible instead of being trusted by name.

### Why there is no compatibility branch

The capability is advertised by the component that does not decide the
behavior, and the deciding component's version is invisible:

| Component | Controlled by | Decides | Version visible to Kandev |
|---|---|---|---|
| Bridge `@agentclientprotocol/claude-agent-acp` | Kandev (`npx --yes --prefer-offline`, agent-runtime-update) | advertises `promptQueueing` | yes — ACP `agentInfo.version` (observed `0.49.0`) |
| CLI `claude` | the user's own PATH install (`Detect(WithCommand("claude"))`) | whether it folds mid-turn | **no** — `claude_code_version` appears zero times in both bridge transcripts |

So a current bridge over a 2.1.175 CLI reports itself steer-capable and defers
every steer, and Kandev cannot detect that. The conclusion is not "add a
compatibility path" but "make the deferred outcome a first-class success", which
is what the spec requires. Old and new installations run the same code; only the
observed outcome differs. Two consequences bind the implementation:

- **No version gate, floor, or warning.** Gating on the CLI is impossible and
  gating on the bridge gates the wrong component.
- **The composer must promise delivery, not folding.** Any copy claiming the
  message will steer the running turn is false on an old CLI. Task 07 owns this.

If an installation's CLI version is ever wanted for diagnostics, that belongs in
agent discovery or `/doctor` as informational only, and is out of scope here.

## Backend

### Capability plumbing

- Read `_meta.claudeCode.promptQueueing` in a focused helper beside
  `supportsPromptHandoff` in
  `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`;
  keep the mock provider eligible by having `apps/backend/cmd/mock-agent`
  advertise the same `_meta`, so E2E exercises the negotiated path rather than a
  bypass.
- Extend the existing `streams.EventTypeAgentCapabilities` event
  (`apps/backend/internal/agentctl/types/streams/agent.go`, emitted at
  `adapter.go:459`) with the negotiated steer capability, so the orchestrator
  learns it over the existing channel instead of a new one.
- Record it on the in-memory session activity record in
  `apps/backend/internal/orchestrator/turn_activity.go` and use it in place of
  the `agent_name` comparison.

### Steer admission

- Add a steer intent to the prompt path
  (`adapter_prompt.go` / `adapter_prompt_cancel.go`). When the connected agent
  is steer-capable, a human prompt with a matching generation may transfer the
  existing gate token while the predecessor RPC is still open — the same
  transfer `tryTransferPromptTurn` performs today, triggered by the operator's
  send instead of by a `ForegroundIdle` event. Reuse
  `protectActiveBackgroundWorkForHandoff` unchanged: fact 3 makes it required,
  not merely useful.
- Keep synthetic wakeups excluded. They already cannot observe `handoffCh`; the
  steer path must not weaken that.
- Preserve `claimPromptTurnCompletion` semantics so the predecessor's early
  `end_turn` cannot emit completion, sweep live tool ownership, consume the
  successor's usage, or clear the successor's trace.

### Orchestrator admission

- Admit a steer while `RUNNING` in `checkSessionPromptable` / `PromptTask`
  (`apps/backend/internal/orchestrator/`), keeping ADR-0049's check-and-claim
  atomicity: a steer must not take the ordinary foreground claim, because the
  predecessor still owns it.
- Enforce the order rule: steer only when the session has no queued messages;
  otherwise queue. At most one unacknowledged steer per session.

### Runtime toggle

- `features.claudeMidTurnSteering` /
  `KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING` through
  `apps/backend/internal/runtimeflags/registry.go`,
  `apps/backend/internal/common/config/config.go`, and
  `apps/backend/internal/profiles/profiles.yaml` (`"false"` in prod, dev, e2e),
  following `/runtime-feature-flags`. Registry completeness tests cover the
  registry/profile/frontend contract.

## Frontend

- Surface `supports_steering` through the session DTO, boot payload, and the WS
  session events; add it to the store slice and to
  `apps/web/lib/state/slices/features/types.ts`.
- Composer distinguishes steer from queue before the operator sends. All new
  copy goes through `t()` per the root i18n rule; add keys rather than reusing
  an approximate existing key.

## Fixtures and E2E

- Commit the three probe transcripts as protocol fixtures and add an adapter
  test that replays the fold-handoff sequence (early `end_turn` with zeroed
  usage and no answer text, successor carrying the answer and the usage).
  ADR-0049 admits handoff only for bridges that are *fixture-tested*; the probes
  are the evidence but are not yet fixtures.
- Extend `apps/backend/cmd/mock-agent` to replay that sequence so E2E can drive
  steer-vs-queue deterministically without a paid model call.
- E2E covers: steer delivered while generating; queue when the toggle is off;
  queue when the agent does not advertise; order preserved when a message is
  already queued.

## Decision record

Create an ADR for the steer-admission trigger and the attribution contract. It
supersedes the mid-turn-steering non-goal in ADR-0049 and records that the
capability gate is negotiated rather than name-based.

## Risks

- **The fold is undocumented and `@internal`.** It can regress without a
  changelog entry; upstream docs still describe strictly sequential queueing.
  Mitigation: the negotiated gate asserts only the concurrent-prompt
  precondition, both outcomes are specified as correct, fixtures pin the
  observed shape, and the toggle stays a kill-switch.
- **Version-pinned upstream claims age badly.** A contributor measured this as
  impossible on claude 2.1.175; it works on 2.1.218–2.1.220. Mitigation: never
  gate on a version string; gate on the negotiated capability and degrade.
- **Changing `supportsPromptHandoff` touches shipped experiment behavior.**
  Mitigation: the change only narrows eligibility, and the foreground-idle path
  keeps its own tests.
- **Attribution is the sharp edge.** A predecessor settlement mistaken for a
  real completion corrupts transcript, usage, and tool state. Mitigation: the
  generation-keyed suppression ADR-0049 already specifies, plus a fixture test
  asserting no completion is emitted for the predecessor.
- **Two overlapping RPCs raise cancel complexity.** Mitigation: explicit cancel
  scenario in the spec and a test that the gate is released.

## Verification

Run once per fresh worktree before anything else, per the root guide:

```bash
cd apps && pnpm install --frozen-lockfile
```

- Backend: `cd apps/backend && go test -race ./internal/agentctl/... ./internal/orchestrator/... ./internal/runtimeflags/... ./internal/profiles/... ./internal/common/config/...`
- Backend lint: `make -C apps/backend lint`
- Web unit: `cd apps/web && pnpm test`
- Web types: `cd apps/web && pnpm run typecheck`
- Web lint: `cd apps/web && pnpm lint`
- i18n: `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet`
- E2E: `cd apps/web && pnpm e2e`

## Tasks

| Task | Wave | Depends on | Status |
|---|---|---|---|
| [task-01-negotiate-steer-capability](task-01-negotiate-steer-capability.md) | 1 | — | done |
| [task-02-runtime-toggle](task-02-runtime-toggle.md) | 1 | — | done |
| [task-03-fold-handoff-fixtures](task-03-fold-handoff-fixtures.md) | 1 | — | done |
| [task-04-adapter-steer-admission](task-04-adapter-steer-admission.md) | 2 | 01, 03 | done |
| [task-05-orchestrator-steer-admission](task-05-orchestrator-steer-admission.md) | 3 | 01, 02, 04 | done |
| [task-06-session-steer-contract](task-06-session-steer-contract.md) | 3 | 01, 05 | done |
| [task-07-composer-steer-affordance](task-07-composer-steer-affordance.md) | 4 | 06 | done |
| [task-08-mock-agent-and-e2e](task-08-mock-agent-and-e2e.md) | 5 | 04, 05, 07 | done |
| [task-09-adr-and-verification](task-09-adr-and-verification.md) | 6 | all | done |

## Assets

- [`assets/mid-turn-steering-composer.png`](assets/mid-turn-steering-composer.png) — the
  composer showing the steer-delivery affordance on a steer-eligible session.

## Validation Results

Re-run on 2026-08-04 against the branch merged with `main`. Per-task command
results are recorded in each task file's own `## Validation Results` section.

- `cd apps/backend && go test -race ./internal/...`: passed for every package
  this plan touches; three unrelated failures on this host are itemized and
  attributed in `task-09-adr-and-verification.md`.
- `make -C apps/backend lint`: `0 issues.`
- `cd apps/web && pnpm run typecheck && pnpm lint && pnpm run i18n:check && pnpm run i18n:ratchet`: all passed.
- `cd apps/web && pnpm test`: 8431 passed; the 6 failures are pre-existing on
  clean `main` in files this plan does not touch.
- `cd apps/web && pnpm e2e`: CI-owned for this branch; green on head `33a4dc3`
  across all 14 E2E shards and all 6 container shards.
