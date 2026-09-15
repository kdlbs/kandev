---
created: 2026-09-13
status: complete
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-001
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-003
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-004
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-005
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-006
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/agents/system-design/harness-session-continuity.md
legacy_specs: []
---

# Implementation plan: reconcile durable sessions with agent survival

## Overview

Integrate PR #3598 with merged PR #3467 without losing either delivery guarantees or safe process adoption.
This is a follow-up to the [completed durable-session package](../durable-agent-sessions/plan.md).
Its original eighteen work orders remain historical implementation records.
This package owns the remaining integration work, not a repeat of that implementation.

The platform system owns this reconciliation because durable transport identity and projection determine safe adoption.
Executor ownership proof and process lifetime remain executor contracts.
The existing [boundary ADR](../../decisions/2026-09-10-durable-harness-session-boundaries.md) remains authoritative.
No new architecture decision is required to apply its separation to surviving processes.

## Verified baseline and instructions

- PR #3598 and local HEAD: `ab81ebcf578ecc313e96c0d952a56315c02190e2`.
- PR #3467 final head: `97c0647f51ad7bf3a06c031aac60d91e7416c02e`.
- PR #3467 merge and observed `origin/main`: `e75c8b37c88f6688f2d3dd62e51408339e7c17d7`.
- Current branch merge base: `89bf7657a28fd1ed58452d57abe631c175e08a05`.
- Planning checkout was clean. No production edits, branch merge, or runtime probes occurred during planning.
- `git merge-tree --write-tree --name-only HEAD origin/main` reports eleven conflict paths.
- The user requested a plan for another agent. This package does not launch that agent or authorize a push.

Refresh both heads before implementation. Record any new commits and reassess affected contracts before resolving conflicts.
Preserve this branch's recent effect-conflict, passthrough, steer, canonical-publication, and mock-timeout fixes.
Do not reset to the older comparison head `e1e15874a`.

## Scope

### In scope

- Integrate current main and preserve both branches' logic, tests, and documentation.
- Restore durable identity, protocol capability, submission state, and replay position during adoption.
- Keep terminal recovery within ordered durable projection for compatible peers.
- Preserve legacy outcome recovery, ownership fencing, explicit continuation, cancellation, and queue admission.
- Reconcile shared desktop/mobile recovery controls and their typed API errors.
- Prove the combined behavior with targeted restart, storage, and browser regressions.

### Out of scope

- New remote-process survival, host-reboot survival, or automatic context continuation.
- A new feature flag, removal of the merged survival toggle, or changing its shipped defaults.
- Replacing the ownership proof, secret store, queue lock, journal, or SQL inbox.
- Unrelated CI repairs, release publication, or merging PR #3598.

## Technical approach

The detailed contract is in [surviving-process adoption](../../specs/platform/system-design/durable-agent-delivery.md#surviving-process-adoption).
Durable delivery remains automatic for compatible retained storage with survival enabled or disabled.
The existing `features.agentSurvival` toggle controls detached lifetime only.
Treat the merged survival design's old terminal-only recovery description as the legacy-peer path.
Task 01 reconciles that description and its linked work package after importing main.

### Conflict-resolution map

Paths in this table are repository-relative.

| File | Required resolution |
| --- | --- |
| `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go` | Preserve recovery guards, reconstruction, deadlines, detach handling, and original-workspace identity. Tasks 02 and 03 add durable adoption. |
| `apps/backend/internal/agent/runtime/lifecycle/persistence.go` | Preserve both filtered recovery metadata sets, including original workspace and Office profile identity. |
| `apps/backend/internal/agentctl/server/process/manager.go` | Preserve journal ownership and commit barriers, survival buffering, terminal retention, stop release, and every error producer. |
| `apps/backend/internal/agentctl/types/streams/agent.go` | Keep `ControlTurnID`, durable `TurnID`, and all delivery fields. They are different identities. |
| `apps/backend/internal/orchestrator/handlers/handlers.go` | Preserve guard conflicts and typed continuation errors. |
| `apps/backend/internal/orchestrator/handlers/handlers_test.go` | Keep both error families and add the combined precedence case. |
| `apps/web/components/task/chat/session-stopped-banner.tsx` | Preserve guard state and explicit continuation without exposing unsafe replacement. |
| `apps/web/hooks/domains/session/use-session-recovery-actions.ts` | Combine guard and continuation state with operation-generation protection. |
| `apps/web/hooks/domains/session/use-session-recovery-actions.test.ts` | Retain both regression sets and both failure contexts. |
| `apps/web/lib/services/session-recovery-service.ts` | Retain guard parsing, localized errors, and `continue_from_history`. |
| `apps/web/lib/services/session-recovery-service.test.ts` | Combine the add/add tests. Do not choose one side. |

Automatic merges also require inspection.
These include backend startup ordering, standalone instance configuration, API event writers, process shutdown, and task schema initialization.
Preserve `StartEventWatcher` before recovery and existing queue startup reconciliation after recovery.
Inspect command launch and release cleanup because detached instances must retain their open journal.
Do not hold global execution or admission locks during network requests or replay.

### Documentation reconciliation

Task 01 imports these main-owned sources and preserves their existing IDs:

- `docs/specs/executors/requirements/agent-survival-across-restart.md`
- `docs/specs/executors/requirements/agent-survival-session-state.md`
- `docs/specs/executors/requirements/standalone-control-server-ownership.md`
- `docs/specs/executors/requirements/standalone-control-server-single-driver.md`
- `docs/specs/executors/system-design/agent-survival-across-restart-01.md`, `-02.md`, and `-03.md`
- `docs/plans/agent-survival-across-restart/plan.md` and its single work order

These paths exist at the merge commit, not in the planning checkout.
After import, use local links in both companion plans and designs.
Qualify terminal-slot recovery, prompt-identity reconstruction, and flag-off claims by protocol and process-lifetime scope.
Preserve the accepted ownership and local/worktree-only survival requirements.
Keep completed work-order results as historical evidence and link this pending integration package.

## ASCII UI preview

### UI-01: Session recovery after backend restart

Entry: existing task-session chat and its recovery banner. Labels show intent, not new literal copy.

```text
Desktop: existing chat region
[Recovery status and reason]                      [Stop]
[Retry connection]  [Continue from saved context*]

Phone: existing session chat
[Recovery status and reason]
[Stop                         ]
[Retry connection             ]
[Continue from saved context* ]
```

`*` Only a typed continuation result and resolved ownership permit this action.
During adoption, show recovery progress and suppress replacement actions.
For an unstoppable prior owner, show the guard error and suppress replacement.
For unresolved delivery, keep Stop and a state-only retry available.
After successful adoption, remove the recovery notice without adding a resume message.

Reuse `session-stopped-banner.tsx` and `mobile-session-resume-recovery.spec.ts` as the nearest surfaces.
Keep status before actions and reuse shared recovery state on both viewports.
Use inline presentation because the user needs the conversation while resolving recovery.
The chat remains the scroll owner. Add no drawer, nested scroller, or fixed overlay.
Phone actions stack with at least 44px touch targets and no horizontal overflow.
Desktop controls retain their existing compact size. Preserve focus during asynchronous updates.
All copy uses existing translation keys or complete locale updates.
Spacing is illustrative. State precedence, action eligibility, and mobile access are required.
This view covers continuity criteria 005.1 and 005.2 and delivery criterion 006.2.

## Tests

Each work order names exact commands and new test names.
New names are proposed test deliverables, not claims that those tests already exist.

| Evidence | Acceptance criteria | Owner |
| --- | --- | --- |
| Merge compile and existing contract tests | Delivery 001.1, 007.3; continuity 003.2 | Task 01 |
| Restore stream, incarnation, generation, turn and submission before reconnect | Delivery 003.2, 004.1-004.3, 006.1, 006.3 | Task 02 |
| Negotiate adopted peer without ACP initialization | Delivery 007.1, 007.3-007.5 | Task 02 |
| Commit, replay, terminal projection, ACK and crash boundaries | Delivery 001.1, 005.1-005.3 | Task 03 |
| Park queues and autonomous work; preserve Stop and explicit actions | Delivery 006.2; continuity 001.2, 005.1, 005.2, 006.1-006.4 | Task 04 |
| Real backend restart with partial ACK, subsequent prompt, cancellation, and legacy mode | Delivery 002.1, 003.1, 004.1, 007.2 | Task 05 |

The short IDs in this table refer to the full prefixes in frontmatter and work orders.
Use deterministic barriers for races. Include late-old-event-after-new-turn ordering.
Existing unit tests for replay do not prove recovery through the real adoption path.

## E2E tests

- Extend merged `apps/web/e2e/tests/session/agent-survival-restart.spec.ts` in `chromium`.
- Add `apps/web/e2e/tests/session/mobile-agent-survival-restart.spec.ts` in `mobile-chrome`.
- Retain desktop/mobile pause, resume recovery, and stream-isolation regressions.
- Use the existing disposable `backend.useEnv()` and `backend.restart()` fixture patterns.
- Do not restart the developer's backend or use a live task as test data.
- Rebuild web, backend, agentctl, and mock-agent through the documented E2E runner prerequisites.
- Keep one guarded runner active. Do not increase workers or overlap suites.

## Work orders

All work is sequential, including work delegated later by the user.

- [x] [Task 01: Integrate the merged survival baseline](task-01-merge-baseline.md)
- [x] [Task 02: Restore adopted delivery identity and capability](task-02-adoption-identity.md)
- [x] [Task 03: Reconcile ordered replay and terminal recovery](task-03-terminal-replay.md)
- [x] [Task 04: Preserve recovery admission and UI guards](task-04-recovery-guards.md)
- [x] [Task 05: Prove combined restart behavior](task-05-restart-regressions.md)

## Verification results

Implementation and planning validation passed on 2026-09-14:

- Catalog validation: 268 decisions and 880 specifications.
- Full specification lint and all 36 linter tests passed.
- Six-document package audit: valid local links, requirement/acceptance IDs, design paths, and dependency order.
- Source paths exist locally, including the desktop and mobile restart fixtures.
- The combined implementation passed the work-order backend, frontend, build, browser, schema, specification, lint, and whitespace checks.
- The first full mobile browser attempt had one existing delayed-resume timing failure; the focused retry and the complete mobile rerun passed all seven tests.
- PostgreSQL, Docker, SSH, Kind, retained-executor replacement, and live harness compatibility evidence remain environment-dependent and were not claimed by this package.

The five reconciliation work orders are complete. The implementation commit
contains production and regression-test changes plus public and agent guidance;
the planning and specification edits in this workspace remain intentionally
uncommitted.

## Risks

- Mechanical conflict resolution can compile while bypassing durable identity or projection.
- An adopted client has no initialize-time capability cache. Missing cache is not proof of a legacy peer.
- ACK pruning makes a wrong zero cursor a recovery failure, not an empty transcript.
- Direct terminal application can release queued work before missing output reaches canonical storage.
- A control-server counter cannot replace a durable Kandev turn identity.
- A configured journal failure must never activate the legacy terminal fallback.
- A rolling upgrade can adopt an older agentctl binary. Ownership compatibility does not establish delivery compatibility.
- Existing persistence and browser results are historical. Missing PostgreSQL or runtime dependencies remain explicit evidence gaps.

## Handoff

Implement Tasks 01 through 05 in order after the user's separate implementation request.
Use TDD for changed logic and record exact commands and results in each work order.
Keep the original eighteen tasks complete and this package pending until its own evidence passes.
Before any authorized push, use the repository commit, push, and PR-fixup skills.
This planning turn does not merge main, change production code, publish, or start another agent.

## Streaming repair follow-up (2026-09-15)

The [streaming repair package](../durable-agent-stream-repair/plan.md) tracks confirmed replay, legacy, batching, capacity, and recovery defects.
Its seven work orders are pending. Historical completed task results above remain unchanged and do not prove these repairs.
Release readiness requires the follow-up evidence; earlier green CI does not cover the new regressions.
