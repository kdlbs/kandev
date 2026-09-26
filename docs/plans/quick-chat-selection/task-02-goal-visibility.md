---
id: "02-goal-visibility"
title: "Retain and disclose active agent goals"
status: done
wave: 2
depends_on:
  - "01-restore-selection"
plan: "plan.md"
requirements:
  - REQ-AGENTS-GOAL-VISIBILITY-001
acceptance_criteria:
  - AC-AGENTS-GOAL-VISIBILITY-001.1
  - AC-AGENTS-GOAL-VISIBILITY-001.2
  - AC-AGENTS-GOAL-VISIBILITY-001.3
  - AC-AGENTS-GOAL-VISIBILITY-001.4
  - AC-AGENTS-GOAL-VISIBILITY-001.5
  - AC-AGENTS-GOAL-VISIBILITY-001.6
  - AC-AGENTS-GOAL-VISIBILITY-001.7
  - AC-AGENTS-GOAL-VISIBILITY-001.8
system_design:
  - ../../specs/agents/system-design/goal-visibility.md
---

# Task 02: Retain and disclose active agent goals

## Summary

Retain explicit ACP goal state across unrelated metadata updates and display the
active goal beside Todos/PR information above the composer. Deliver backend retention,
frontend hydration, desktop disclosure, phone drawer, and their regression tests together.

## In scope

- Typed validation of the documented Codex goal snapshot and absent/null semantics.
- Goal retention through the existing session metadata, persistence, and WS path.
- Freshness and session isolation across clear, completion, reconnect, and hydration.
- Shared Goal Active chip, read-only details, localization, and public explanation.
- Mock ACP scenarios and desktop/mobile rendered verification.

## Out of scope

- Scheduling/polling changes, goal controls, task/autopilot transitions, and usage dashboards.
- Native transcript-file readers, provider RPCs on hover, and inference from message text.
- Passthrough terminal toolbar redesign and unsupported-provider imitation.

## Acceptance

1. First add `TestHandleSessionInfoEvent_GoalSurvivesUnrelatedMetadata` in a new
   `event_handlers_goal_test.go`. Feed active goal then thread-status metadata and
   assert persisted and published goal retention. Record the expected RED failure.
2. Deliver all eight acceptance criteria, including null clearing, final-response
   persistence, stale hydration rejection, session isolation, and accessible details.
3. Run each required check below. Record actual results and rendered preview comparison
   before marking this work order done. Normal task chat and Quick Chat must both pass.

## Implementation sequence

1. Add a defensive goal parser and typed retained view. Capture sanitized fixture
   objects from the verified extension shape, never a full production transcript.
2. Apply narrow goal-retention semantics in the existing metadata path. Do not
   deep-merge arbitrary provider metadata. Publish retained goal after persistence.
3. Preserve absent/null distinction and freshness in the frontend handler and
   hydration. Add the selected-session goal selector without a parallel task store.
4. Add `AgentGoalChip` and detail content to `ChatStatusBar`, after registered PR
   status and before queue information. Include goal-only status-row visibility.
5. Add explicit hover/focus/click behavior and the phone/coarse-pointer Drawer.
   Scope touch sizes; do not enlarge neighboring fine-pointer desktop controls.
6. Extend mock-agent scenarios with active, unrelated metadata, complete, and
   clear frames. Use them for E2E instead of frontend store injection.
7. Localize all fixed copy and explain the chip in `docs/public/developer-tools.md`.

## ASCII UI preview

Use [UI-03 and UI-04](plan.md#ui-03-goal-active-above-the-composer), covering all
`AC-AGENTS-GOAL-VISIBILITY-001` criteria.

```text
Desktop: [Todos] [PR #123] [Goal Active] [Queue]
                            |
                   + Goal ----------------------+
                   | Active                     |
                   | Objective text             |
                   | The agent may continue     |
                   | automatically between      |
                   | replies.                   |
                   +----------------------------+
         Existing chat input

Phone:   [Todos] [PR #123]
         [Goal Active]
         Existing chat input

Tap opens a bottom drawer:
         + Goal ---------------------- Close ---+
         | Active                               |
         | Objective text (internal scrolling)  |
         | Continuation explanation             |
         +--------------------------------------+
```

Chip icon is static. Idle keeps the chip. Non-active/clear removes it and closes
details. Session switching closes old details. Hover-to-content movement keeps the
desktop popover open. Escape closes only details, and focus returns when possible.
Phone uses a fixed drawer header, safe-area clearance, and at least a 44px trigger.
Labels and spacing are illustrative; ordering, surfaces, and behavior are required.

## Verification

Run from the repository root after Task 01. Managed E2E rebuilds current sources.
Do not run desktop and mobile simultaneously. New test files below belong to this task.

```bash
(cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp -run 'Test.*(Goal|SessionInfo)' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run 'TestHandleSessionInfoEvent|Test.*Goal' -count=1)
(cd apps/backend && go test ./cmd/mock-agent -count=1)
(cd apps/web && pnpm exec vitest run lib/ws/handlers/session-info.test.ts lib/agent-goal.test.ts components/task/chat/agent-goal-chip.test.tsx components/task/chat/chat-status-bar.test.tsx components/task/chat/chat-input-area.test.tsx lib/state/slices/session/session-merge-goal.test.ts lib/state/hydration)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/agent-goal.spec.ts tests/chat/quick-chat.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-agent-goal.spec.ts tests/chat/mobile-quick-chat-tabs.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The backend suite must include tests for all supported statuses, malformed data,
duplicate/older snapshots, current attachment identity, and ordered null clearing.
Hydration tests defer HTTP until after live completion/clear, then release it.
Frontend component tests also prove no backend/provider request occurs on disclosure.
E2E checks active+idle retention, reload, non-first Quick Chat selection, sibling
session isolation, long objective containment, hover/focus/click, touch, and clear/completion.
Assert the touch trigger's rendered bounds, not only its CSS class.
Generate Traditional Chinese with `pnpm run i18n:zh-hant` from `apps/web` before final checks.

## Files likely touched

- New goal parser/tests under `apps/backend/internal/agentctl/server/adapter/transport/acp/` as needed for provider normalization.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go` and `conversion_test.go`.
- New `apps/backend/internal/orchestrator/event_handlers_goal.go` and `_test.go` for narrow retention helpers/tests.
- `apps/backend/internal/orchestrator/event_handlers_streaming.go` for the existing persistence and publish integration.
- Session-info lifecycle types/forwarding only if presence or attachment identity needs typed transport support.
- `apps/backend/cmd/mock-agent/scenarios.go` and a new goal scenario file with tests.
- `apps/web/lib/ws/handlers/session-info.ts` and `.test.ts`.
- New `apps/web/lib/agent-goal.ts` and `.test.ts` for the typed goal view.
- `apps/web/lib/state/hydration/hydrator.ts` and related freshness tests as needed.
- New `apps/web/components/task/chat/agent-goal-chip.tsx` and `.test.tsx`.
- `apps/web/components/task/chat/chat-status-bar.tsx` and new `chat-status-bar.test.tsx`.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/` affected chat catalogs.
- New `apps/web/e2e/tests/chat/agent-goal.spec.ts`, `mobile-agent-goal.spec.ts`, and shared helpers if needed.
- `docs/public/developer-tools.md`, owning specs, and this package's results.

## Dependencies

Task 01. Reuse its restored selection behavior when checking the goal in Quick Chat.
Keep the work in the primary session; this package does not authorize delegation.

## Risks

- Whole-object metadata replacement or stale hydration can erase or resurrect a goal.
- Absent and null must stay distinct; an unknown status cannot claim automatic continuation.
- Provider timestamps do not timestamp clear events. Retain source order and local live revisions.
- Session changes must invalidate old objectives and open disclosure state.
- Hover-only interaction would exclude keyboard and touch users.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/goal-visibility.md).
- [Design and verified wire shape](../../specs/agents/system-design/goal-visibility.md).
- `TodoIndicator`, `ChatStatusBar`, `useTouchDrawer`, and existing Drawer patterns.
- Existing `session-info.test.ts` and `TestHandleSessionInfoEvent_PersistsACPDebugInfo`.
- `apps/backend/cmd/mock-agent/AGENTS.md` and `.agents/skills/e2e/SKILL.md`.

## Results

Implemented typed ACP goal retention and read-only disclosure. The backend
recognizes the documented goal extension, preserves it across unrelated session
metadata updates, retains explicit null clearing, rejects malformed or stale
snapshots, and resets the projection when the ACP attachment changes. The
frontend keeps a narrow typed goal projection through live updates and
hydration, prevents late data from resurrecting a cleared goal, and isolates
the selected session. The shared status row now renders the localized Goal
Active chip for task chat and Quick Chat. Desktop uses a hover, focus, and click
popover. Phone and coarse pointers use a bounded bottom drawer with a fixed
header, safe-area handling, internal objective scrolling, and a 44px trigger.
Mock-agent scenarios cover active, unrelated metadata, complete, clear, and
long-objective states. Public documentation and all locale catalogs were
updated.

Verification passed:

- Backend goal, session-info, adapter conversion, mock-agent, and race tests passed.
- `make lint` and `make -C apps/backend build` passed.
- Focused goal Vitest: 10 files, 92 tests passed. The final goal component/session subset passed 22 tests, including live snapshot freshness, fresh reconnect clearing, and a real pointer-mode transition.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- Desktop goal E2E: 3 tests passed. Mobile goal E2E: 1 test passed after the final trigger refactor.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, public-doc validation, and `git diff --check` passed.
