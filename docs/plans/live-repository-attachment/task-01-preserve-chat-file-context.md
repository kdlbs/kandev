---
id: "01-preserve-chat-file-context"
title: "Preserve relative chat file context"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.6
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.7
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.8
system_design:
  - ../../specs/tasks/system-design/chat-file-link-context.md
---

# Task 01: Preserve relative chat file context

## Summary

Repair #5070 by retaining the confirmed native workspace separately from the Files root.
Resolve unqualified links from that workspace and preserve explicit source-qualified targets.

## In scope

- Add the read-only current-generation workspace projection and nullable session-detail field.
- Publish the actual execution workspace on ready/materialization events.
- Preserve the field through session merge, provider context, and memoized Markdown.
- Resolve paths under the Files boundary without searching sibling repositories.
- Update the two public reference pages named by the plan.

## Out of scope

Git refresh state, agent restart or relocation, arbitrary shell CWD tracking, new message storage, and file-service authority changes.

## Acceptance

1. `TestTaskSessionAgentWorkspaceProjection` proves distinct effective, primary, and native roots after promotion and an ordinary session update.
   Include parent-root launch, current incarnation, missing generation, DTO null, and partial-event omission.
2. A behavioral provider/resolver regression fails before correction and passes afterward for existing and new relative links.
   Include explicit prefixes, source aliases, nested placements, two branches of one repository, duplicate filenames, and both hydration orders.
3. Existing file-link safety and single-root behavior pass. Unknown expanded-workspace context blocks an unqualified target.
   The public reference pages describe the additive field and unchanged-CWD behavior.

## ASCII UI preview

Use [UI-01 in the plan](plan.md#ascii-ui-preview), mapped to Tasks `.7/.8`.

```text
Desktop: Chat [work order] -> File panel: master/implementation_plans/...md
Phone:   Chat [work order] -> Back | file title | Close
                            Exact contents in existing file scroller
```

Preserve existing controls and phone safe areas. The path illustrates identity, not new UI copy.

## Verification

Run from the repository root. If workspace dependencies are absent, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite -run 'TestTaskSession(WorkspacePath|AgentWorkspace)')
(cd apps/backend && go test -tags fts5 ./internal/task/dto -run 'Test.*(TaskSession|AgentWorkspace)')
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(NotifyWorktreeMaterialized|AgentWorkspace|.*AgentctlReady)')
(cd apps/web && pnpm exec vitest run lib/markdown/file-link-target.test.ts components/shared/task-markdown-file-link-provider.test.tsx components/shared/markdown-components.test.tsx components/shared/memoized-markdown.test.tsx components/shared/markdown-fallback-store-subscription.test.tsx lib/state/slices/session/session-slice.upsert.test.ts lib/ws/handlers/agent-session.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings lib/types/http.ts lib/state/slices/session/session-merge.ts lib/ws/handlers/agent-session.ts lib/markdown/file-link-target.ts components/shared/task-markdown-file-link-provider.tsx components/shared/markdown-components.tsx components/shared/memoized-markdown.tsx)
(
  mapfile -t attachment_go_files < <(
    git diff --name-only --diff-filter=ACMR origin/main...HEAD -- 'apps/backend/**/*.go'
    git ls-files --modified --others --exclude-standard -- 'apps/backend/**/*.go'
  )
  mapfile -t attachment_go_files < <(printf '%s\n' "${attachment_go_files[@]}" | sort -u)
  if ((${#attachment_go_files[@]})); then bash scripts/lint-go-changed "${attachment_go_files[@]}"; fi
)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Task 03 owns fresh desktop and phone browser proof for this outcome.

## Files likely touched

- `apps/backend/internal/task/models/models.go` and `internal/task/dto/dto.go` within `apps/backend`.
- New `apps/backend/internal/task/dto/agent_workspace_projection_test.go`.
- `apps/backend/internal/task/repository/sqlite/session.go` and new `session_agent_workspace_test.go`.
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`, `events.go`, and `manager_workspace_rescan.go`.
- New `apps/backend/internal/agent/runtime/lifecycle/agent_workspace_event_test.go`.
- `apps/web/lib/types/http.ts`, `lib/state/slices/session/session-merge.ts`, and `session-slice.upsert.test.ts`.
- `apps/web/lib/ws/handlers/agent-session.ts` and its tests.
- `apps/web/lib/markdown/file-link-target.ts` and its tests.
- Shared `task-markdown-file-link-provider`, `markdown-components`, and `memoized-markdown` components and their tests.
- `docs/public/websocket-api.md` and `docs/public/automation-and-mcp.md`.

## Dependencies

None. Task 03 completes the browser acceptance for this slice.

## Risks

Session SQL scan order, null-versus-omitted semantics, stale hydration, and parent-root agents require explicit regression cases.
Registered source prefixes must come from exact worktree placement, including remote and nested cases.

## Parallelism

`sequential`

## Inputs

Read the linked design, requirement criteria, proposed ADR, and plan evidence.
Use the existing source-alias tests and completed multi-repository chat-root package as compatibility references.

## Results

Implemented the additive `agent_workspace_path` projection, ready-event
publication, session merge/null semantics, and relative-link resolution.

Behavioral evidence: the recorded RED reproduction returned
`implementation_plans/...` instead of `master/implementation_plans/...` before
the fix; the resolver regression now returns the primary checkout prefix.

Commands and results:

- Session, DTO, and lifecycle projection tests: passed.
- Resolver, provider, Markdown context, memoized, session merge, and ready-event
  Vitest suites: passed.
- `(cd apps/web && pnpm run typecheck)`: passed.
- `(cd apps/web && pnpm run i18n:ratchet)`: passed.
- Changed-file Oxlint and `scripts/lint-go-changed`: passed.
- Public-doc validation and `git diff --check`: passed.
- Review follow-up: the fallback Markdown selector now uses `useShallow`, and
  `markdown-fallback-store-subscription.test.tsx` exercises the real
  `StateProvider` with an active session and an unrelated store update. It
  fails with `Maximum update depth exceeded` without the shallow comparison
  and passes with it.

Remaining limitation: the projection describes the recorded native working
directory; it does not observe a later shell `cd`.
