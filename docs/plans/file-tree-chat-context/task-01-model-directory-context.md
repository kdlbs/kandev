---
id: "01-model-directory-context"
title: "Model directory context items"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/file-tree-chat-context.md"
---

# Task 01: Model Directory Context Items

## Intent

Teach the existing session-scoped context-file pipeline to represent directory paths explicitly while preserving file callers, legacy session storage, prompt formatting, and ephemeral send behavior.

## Acceptance

- Optional directory identity persists and hydrates without invalidating existing file-only entries, and path-based deduplication/clearing semantics remain unchanged.
- Pending directory context renders with a folder icon and without file preview/open behavior; file context retains its current behavior.
- Message construction names file and directory paths in hidden context while keeping outbound `context_files` metadata backward-compatible.

## TDD sequence

1. Add failing store, builder/renderer, formatter, and send-retention cases for directories and legacy entries.
2. Extend the minimum shared types, persistence mapping, item builder, renderer, and prompt formatter needed to pass them.
3. Rerun the focused suite and typecheck; refactor only duplicated file/directory branching.

## Files likely touched

- `apps/web/lib/state/context-files-store.ts`
- `apps/web/lib/state/context-files-store.test.ts`
- `apps/web/lib/types/context.ts`
- `apps/web/components/task/chat-context-items.ts`
- `apps/web/components/task/chat-context-items.test.ts`
- `apps/web/components/task/chat/context-items/file-item.tsx`
- `apps/web/components/task/chat/context-items/file-item.test.tsx`
- `apps/web/components/task/chat/context-items/context-chip.tsx`
- `apps/web/hooks/use-message-handler.ts`
- `apps/web/hooks/use-message-handler.test.ts`
- `apps/web/components/task/chat/chat-input-area.test.tsx`

## Dependencies

None.

## Parallelism

`sequential` — this establishes the shared context model consumed by the file-tree action.

## Verification

- `cd apps && pnpm install --frozen-lockfile`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/state/context-files-store.test.ts components/task/chat-context-items.test.ts components/task/chat/context-items/file-item.test.tsx hooks/use-message-handler.test.ts components/task/chat/chat-input-area.test.tsx`
- `cd apps/web && pnpm run typecheck`

## Inputs

- Spec `What`, `Failure modes`, and `Persistence guarantees`.
- Plan `Directory-aware context items` and `Tests`.
- Existing file-context precedents in `apps/web/lib/state/context-files-store.ts`, `apps/web/components/task/chat-context-items.ts`, and `apps/web/hooks/use-message-handler.ts`.

## Output contract

Report RED/GREEN evidence, actual files changed, exact commands and test counts, compatibility notes for legacy storage and outbound metadata, blockers/risks, and synchronized task/plan status in this conversation.

## Results

- RED: the new focused cases failed on missing persisted `isDirectory`, directory
  item open/preview branching, and untyped prompt formatting; the existing send
  retention cases were already green.
- GREEN: `cd apps && pnpm --filter @kandev/web test -- --run
  lib/state/context-files-store.test.ts components/task/chat-context-items.test.ts
  components/task/chat/context-items/file-item.test.tsx hooks/use-message-handler.test.ts
  components/task/chat/chat-input-area.test.tsx` — 5 files, 41 tests passed.
- `cd apps/web && pnpm run typecheck` — passed.
- Compatibility: `isDirectory` is optional for legacy session-storage entries;
  directory identity is presentation-only and outbound `context_files` remains
  `{ path, name }`.
