---
id: "06-conversation-forks"
title: "Native conversation forks"
status: done
wave: 6
depends_on:
  - "05-usage-display"
plan: "plan.md"
requirements:
  - REQ-AGENTS-CODEX-NATIVE-004
  - REQ-AGENTS-CODEX-NATIVE-006
  - REQ-COSTS-CONVERSATION-USAGE-001
acceptance_criteria:
  - AC-AGENTS-CODEX-NATIVE-004.1
  - AC-AGENTS-CODEX-NATIVE-004.2
  - AC-AGENTS-CODEX-NATIVE-004.3
  - AC-AGENTS-CODEX-NATIVE-004.4
  - AC-AGENTS-CODEX-NATIVE-006.1
  - AC-AGENTS-CODEX-NATIVE-006.2
  - AC-AGENTS-CODEX-NATIVE-006.3
  - AC-COSTS-CONVERSATION-USAGE-001.2
system_design:
  - ../../specs/agents/system-design/codex-app-server.md
  - ../../specs/costs/system-design/conversation-usage.md
---

# Task 06: Native conversation forks

## Summary and scope

Fork through a completed native turn into a new session on the same task and executor.
Persist lineage, request idempotency, provider identity, inherited-history markers, and the initial usage baseline.
Add the desktop/phone turn action using existing confirmation primitives.

## Exclusions

No new task, Git branch, worktree, or file snapshot. No active-source forks in the initial version.

## Likely files and ownership

- Native adapter optional fork capability and client request wrapper.
- `internal/task/service/`, session models/repository/migrations, and task-owned HTTP/WS action.
- Orchestrator native-session initialization and history/usage baseline binding.
- Shared API/session types, message actions, localized confirmation, and UI-04 E2E files.

## Acceptance

1. Successful forks preserve source history, configuration, executor ownership, and lineage while selecting one persisted destination session.
2. Access denial, source activity, disabled feature, ambiguous RPC loss, and persistence failure create no duplicate selectable session.
3. Copied history does not add usage; desktop and phone clearly communicate shared files and preserve focus on cancel/failure.

## ASCII UI preview

UI-04 from [the plan](plan.md#ui-04-fork-through-completed-turn):

```text
[More] > Fork conversation
Create another session through this turn.
Files stay shared in this workspace.
                       [Cancel] [Fork]
```

Phone uses the existing responsive menu and confirmation surface with a 44px primary action.
Disable Fork with a reason when source or child work remains active.
Success selects the new session; failure leaves the source visible.

## TDD and verification

Create `TestForkOwnershipAndIdempotency` and fake-server native fork tests before implementation.
Inject response loss and database failure after native fork success.
Run from the repository root:

```bash
(cd apps/backend && go test ./internal/agentctl/server/adapter/... ./internal/task/service ./internal/task/repository/sqlite ./internal/task/handlers ./internal/orchestrator/...)
(cd apps/web && pnpm test -- components/task/chat/messages/message-actions.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/codex-app-server.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-codex-app-server.spec.ts)
git diff --check
```

## Risks

An upstream success followed by a lost response cannot be retried blindly.
The native thread can contain prior usage that does not belong to the new Kandev session's recorded totals.

## Results

Native fork boundary and idle/source-work checks passed in transport fake-server tests. The orchestrator ownership/idempotency test passed. Desktop and mobile E2E confirmation flows passed and verify the shared-workspace behavior. PostgreSQL-specific fork coverage, if added, is tracked with Task 07 database validation.
