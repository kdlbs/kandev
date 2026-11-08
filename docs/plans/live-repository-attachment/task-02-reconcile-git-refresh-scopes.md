---
id: "02-reconcile-git-refresh-scopes"
title: "Reconcile Git repository refresh scopes"
status: done
wave: 2
depends_on:
  - "01-preserve-chat-file-context"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.25
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.26
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.39
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.40
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.41
system_design:
  - ../../specs/platform/system-design/git-repository-scope-lifecycle.md
---

# Task 02: Reconcile Git repository refresh scopes

## Summary

Repair #5071 by excluding a file-only parent from Git refresh publication.
Reconcile the complete current scope set and retire its obsolete refresh entries.
Preserve valid unnamed roots and failures in current named repositories.

## In scope

- Apply Git tracker eligibility to explicit refresh paths while retaining Files tracking and poll-mode propagation.
- Keep each aggregate response within one selected source's scope set.
- Add runtime inventory generation, complete-response reconciliation, and late-result guards.
- Refresh an active Changes owner after attachment without a remount.
- Extend legacy retirement to refresh state and preserve current named mirrors.
- Keep generic unnamed-root error feedback without a blank repository sentence.

## Out of scope

New retry infrastructure, global cache clearing, database snapshot migration, repositoryless initial-state changes, and provider transport changes.

## Acceptance

1. `TestManager_LiveRepositoryAttachmentRefreshScopes` fails before correction for plain-parent explicit refresh publication.
   It passes for poll-mode and base-branch refreshes while a real root Git repository still publishes beside named children.
2. A real handler/store/Changes regression fails before correction for retained unnamed refresh failure.
   Current inventory reconciliation clears it, preserves named snapshots, rejects late retired-root frames, and retains mixed current failures.
3. Coordinator tests cover in-flight pre-attachment responses, failed reads, shared sessions, reconnect, and bounded automatic retry.
   All bookkeeping obeys scope generation. An incomplete or unrelated response cannot retire a failure.

## ASCII UI preview

Use [UI-02 in the plan](plan.md#ascii-ui-preview), mapped to Platform `.25/.26/.41`.

```text
Desktop Changes              Phone Changes tab
MASTER: changed-file.md       MASTER: changed-file.md
SECONDARY: clean              SECONDARY: clean
No refresh warning           No refresh warning

Actual failure: keep the existing warning and the failed current scope.
```

Retain current list and disclosure composition. Do not create new copy or controls.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/agentctl/server/process -run 'Test(Manager_|RescanRepositories_|RebindWorkspace|.*PollMode)')
(cd apps/backend && go test -tags fts5 ./internal/backendapp -run 'Test.*Git(Status|Refresh)')
(cd apps/web && pnpm exec vitest run lib/ws/handlers/git-status.test.ts lib/state/slices/session-runtime/git-status-multi-repo.test.ts lib/ws/handlers/agent-session.test.ts hooks/domains/session/git-status-refresh-coordinator.test.ts hooks/domains/session/use-session-git-refresh.test.tsx components/task/changes-panel-git-status.test.ts components/task/changes-panel-refresh-status.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings lib/state/slices/session-runtime/types.ts lib/state/slices/session-runtime/session-runtime-slice.ts lib/ws/handlers/git-status.ts lib/ws/handlers/agent-session.ts hooks/domains/session/git-status-refresh-coordinator.ts hooks/domains/session/git-status-refresh-recovery-utils.ts hooks/domains/session/use-session-git-refresh.ts components/task/changes-panel-git-status.ts components/task/changes-panel-refresh-status.tsx)
(
  mapfile -t attachment_go_files < <(git ls-files --modified --others --exclude-standard -- 'apps/backend/**/*.go')
  if ((${#attachment_go_files[@]})); then bash scripts/lint-go-changed "${attachment_go_files[@]}"; fi
)
git diff --check
```

Task 03 owns fresh desktop and phone browser proof for this outcome.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`.
- New `apps/backend/internal/agentctl/server/process/manager_attachment_refresh_scopes_test.go` and `manager_poll_mode_inherit_test.go`.
- `apps/backend/internal/backendapp/helpers.go` and `helpers_git_status_test.go`.
- `apps/web/lib/state/slices/session-runtime/types.ts`, `session-runtime-slice.ts`, and Git status tests.
- `apps/web/lib/ws/handlers/git-status.ts`, `agent-session.ts`, and their tests.
- `apps/web/hooks/domains/session/git-status-refresh-coordinator.ts` and its tests.
- `apps/web/hooks/domains/session/git-status-refresh-recovery-utils.ts`.
- `apps/web/hooks/domains/session/use-session-git-refresh.ts` and its tests.
- `apps/web/components/task/changes-panel-git-status.ts`, `changes-panel-refresh-status.tsx`, and their tests.

## Dependencies

Task 01 owns the preceding edit to `agent-session.ts`.
This dependency serializes the shared handler and the final integration order.

## Risks

A Git root can remain valid beside named children.
An unavailable current repository must remain unavailable even after another repository succeeds.
Old responses, timeout paths, and sibling-source fallbacks must not recreate retired state.

## Parallelism

`sequential`

## Inputs

Read the scope lifecycle design and linked Platform criteria.
Use the existing multi-repository tracker, status handler, recovery coordinator, and Changes tests as patterns.

## Results

Implemented Git-tracker eligibility for explicit refresh paths, single-source
aggregate responses, runtime inventory generations, complete-response
reconciliation, late-frame guards, active-Changes replacement requests, and
legacy refresh retirement.

Behavioral evidence: the recorded RED reproduction kept
`failedRepositories: [""]` after both named scopes were ready, with and without
legacy cleanup. The new inventory and Changes regressions now retire the empty
scope while preserving current named failures, and the plain-parent tracker no
longer publishes an empty-name snapshot.

Commands and results:

- Agentctl manager/rescan/poll-mode disclosure tests: passed.
- Backend aggregate Git refresh tests: passed.
- Coordinator, Git-status handler, store, and Changes panel Vitest suites:
  passed.
- `(cd apps/web && pnpm run typecheck)`: passed.
- Changed-file Oxlint and `scripts/lint-go-changed`: passed.
- `git diff --check`: passed.

Remaining limitation: a valid unnamed root failure keeps the generic
unavailable notice rather than a repository sentence. The same is true for an
unavailable root beside named children.
