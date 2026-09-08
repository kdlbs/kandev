---
id: "03-frontend-contract-dispatch"
title: "Frontend contract and dispatch"
status: done
wave: 3
depends_on: ["02-backend-mcp-contract"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-rich-output.md"
---

# Task 03: Frontend contract and dispatch

## Acceptance

1. A pure fail-closed parser accepts the version 1 file, chart, and metrics
   union and rejects malformed, over-limit, and forward-version payloads.
2. Completed rich-output messages remain standalone while ordinary tool
   activity grouping is unchanged.
3. Kandev dispatch gives the rich renderer current session and file-opening
   context without changing unregistered-tool fallback behavior.

## Verification

```sh
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run components/task/chat/messages/kandev/rich-output/parse.test.ts components/task/chat/types.test.ts hooks/use-processed-messages.test.ts components/task/chat/messages/kandev-tool-message.test.tsx
```

## Files likely touched

- `apps/web/components/task/chat/messages/kandev/rich-output/types.ts`
- `apps/web/components/task/chat/messages/kandev/rich-output/parse.ts`
- `apps/web/components/task/chat/messages/kandev/rich-output/parse.test.ts`
- `apps/web/components/task/chat/types.ts`
- `apps/web/components/task/chat/types.test.ts`
- `apps/web/hooks/use-processed-messages.ts`
- `apps/web/hooks/use-processed-messages.test.ts`
- `apps/web/components/task/chat/messages/kandev-tool-message.tsx`
- `apps/web/components/task/chat/messages/kandev/registry.ts`
- `apps/web/components/task/chat/message-renderer.tsx`

## Dependencies

Task 02.

## Parallelism

Sequential. Parser and message identity are shared by native block rendering.

## Inputs

- Spec API surface and state/failure scenarios.
- Existing Kandev renderer registry and `groupActivityMessages` behavior.

## Output contract

Report RED and GREEN evidence, actual files, exact focused test result, and
task/plan status update.

## Results

- RED: the parser import was absent; rich tool calls collapsed into one
  activity group; the shared message predicates and host context were absent.
- GREEN: 94 focused parser, predicate, grouping, dispatch, and existing Kandev
  renderer tests passed.
- Added an exact version 1 parser with size/count/path/numeric checks, shared
  tool identity, standalone grouping, renderer registration, and session plus
  repository-aware file-opening context.
