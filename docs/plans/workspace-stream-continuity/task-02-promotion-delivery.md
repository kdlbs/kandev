---
id: "02-promotion-delivery"
title: "Prove promotion delivery in Changes"
status: completed
wave: 2
depends_on: ["01-callback-lifetime"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.2
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.20
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.29
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.43
system_design:
  - ../../specs/platform/system-design/workspace-stream-continuity.md
---

# Task 02: Prove promotion delivery in Changes

## Summary and inputs

Prove that the corrected workspace stream reaches its publisher and the existing desktop/phone Changes surfaces.
Read the [plan](plan.md), [requirement](../../specs/platform/requirements/workspace-git-status.md), design, and Task 01 results.
Read `/e2e`, `/mobile-parity`, and web `AGENTS.md` before browser work.
Task 01 owns the behavioral RED gate. This order owns integrated GREEN evidence and delivery validation.

## Responsibilities

1. Add a real workspace WebSocket fixture that reaches `StreamManager` and the manager Git publisher after startup generation advances.
2. Assert environment, repository, execution attribution, membership, and settled detail survive the reused stream.
3. Add desktop and mobile promotion scenarios using managed isolated backends and disposable workspaces.
4. Prepare a CREATED session with workspace-only execution, open Files/Changes, and start the agent through the existing task-description Start agent action.
5. Prove the workspace-only status, start-created response, and post-promotion stream events identify the same execution.
6. After startup settles, mutate a real file in that canonical worktree.
   Observe the corresponding stream event and visible Changes row.
7. Observe settled diff/detail state without focus cycling, navigation, reload, or another explicit Git read after the mutation.
8. Record screenshots and source-correlated timing. Update package results and remove the proposed-criterion note after completion.

Reuse the existing Git helpers and session page objects. Keep the completed-workspace restoration test as an adjacent desktop regression.
Use production runtime identity evidence from available status/log fixtures to prove promotion, rather than inferring it from a visible banner.
Arm stream and outbound-refresh observers before the mutation. A correlated refresh response cannot satisfy the stream assertion.
Keep initial snapshot requests separate from post-mutation evidence. Record any recovery requests and fail proof that depends on them.
Do not inject Git status into the store or replace the stream's status payload.

On phone, enter Changes through bottom navigation with `.tap()` and use the existing full-height diff surface.
No UI markup or geometry change is planned. Both viewports must show the same new path and settled detail.
Clean fixture mutations and restore shared settings in `finally` or `afterEach`, including failure paths.

## Acceptance

- The real workspace transport reaches the manager publisher after promotion with correct scope and unchanged payload quality.
- Desktop and phone show the post-promotion Git mutation and settled detail through stream delivery without a corrective read. The browser evidence ties workspace-only status, agent startup, and mutation delivery to the same execution.
- Managed browser checks and targeted backend integration checks pass, with a fresh backend/frontend build and all package results recorded accurately.

## Exclusions

No layout, copy, new polling policy, artificial status injection, live-instance mutation, or remote-executor certification.

## Files and ownership

- New `apps/backend/internal/agent/runtime/lifecycle/streams_workspace_promotion_test.go`: real transport-to-publisher regression.
- New `apps/web/e2e/tests/session/workspace-stream-promotion.spec.ts`: desktop scenario.
- New `apps/web/e2e/tests/session/mobile-workspace-stream-promotion.spec.ts`: phone scenario.
- New sibling `workspace-stream-promotion-helpers.ts`: shared setup and causal observers.
- `completed-workspace-restoration-helpers.ts`, `session-page.ts`, and Git helpers: reuse existing APIs, extend only for a missing necessary operation.
- Package requirement/design/plan/work orders: implementation status and actual verification results.

## Verification

Run sequentially from repository root. If dependencies are absent, first install from `apps/` with the frozen lockfile.
Managed browser runs rebuild the backend and web assets and enforce the repository worker budget.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'TestWorkspaceStreamPromotion' -count=1 -timeout=120s -v)
(cd apps/web && pnpm e2e:run --host --project chromium tests/session/workspace-stream-promotion.spec.ts tests/session/completed-workspace-restoration.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/session/mobile-workspace-stream-promotion.spec.ts tests/task/mobile-changes-panel.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The desktop and mobile browser checks use the managed host runner because the local Docker runtime image needs an unavailable build image on this host; each runner still builds the backend and frontend and starts isolated backends for its tests.
Validate that each browser command discovers the intended tests.
If a planned check cannot run, report its exact blocker.
Record elapsed times as diagnostic measurements, not universal latency guarantees.

## Dependencies and parallelism

Requires Task 01 completion. Reuse its correction without expanding lifecycle scope.
`sequential`. This work order does not authorize delegation.

## Results

The browser promotion scenarios use a prepared CREATED session. They observe its workspace-only status before the UI sends the first prompt through `start_created`, then match that response and the post-mutation stream events to the same execution ID. Completed-session resume was excluded from the continuity proof because it starts a different runtime execution; the existing completed-workspace restoration test remains in the desktop run.

The targeted promotion tests pass on desktop and phone. The managed desktop run passed both promotion and completed-workspace restoration; the managed phone run passed all ten promotion and Changes-panel tests. A fresh host-managed run built the backend and frontend. Web typecheck and targeted ESLint passed, as did the lifecycle race suite, specification checks, and final diff checks. See the [fix plan](plan.md) for the complete verification record.
