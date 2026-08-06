---
id: "08-frontend-protocol-bridge"
title: "Frontend task protocol bridge"
status: completed
wave: 4
depends_on: ["07-lifecycle-reconciliation"]
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 08: Frontend Task Protocol Bridge

## Acceptance

- Frontend snapshots, live updates, and actions are keyed by task/language, reject stale revisions,
  and never send session/runtime identity or origin in control requests.
- A browser shares one downstream attachment per task/language across same-task session models,
  consumes task-host capabilities, and never sends initialize/shutdown/exit or owns a process timer.
- Monaco provider routing, diagnostics, navigation, document synchronization, configuration
  behavior, TypeScript built-in suppression, and save-race guarantees remain intact; releasing all
  browser editor leases can detach but cannot change task policy/generation.

## TDD sequence

1. Add failing API/store/hook tests for task-scoped shapes, all policies/phases/evidence, action
   payloads, hydration/live-update races, lower-revision rejection, and legacy localStorage removal.
2. Add failing manager harness cases for two sessions sharing one task/language attachment, attach
   handshake/capabilities, no browser initialize/shutdown, reconnect, and final detach without Stop.
3. Add failing provider/document tests for source-session URI mapping, same task-host URI across
   session models, diagnostic replay, model-scoped suppression, ordered sync, and save races.
4. Implement the domain slice/API/hook and refactor `LSPClientManager` to task ownership. Keep
   session identity only at the Monaco model/navigation edge.
5. Run focused tests, typecheck, and lint; remove obsolete idle/storage/placement dependencies only
   after their replacement tests pass.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run lib/api/domains/lsp-api.test.ts lib/state/slices/lsp hooks/domains/lsp lib/lsp/lsp-client-manager.test.ts lib/lsp/lsp-client-manager.document-sync.test.ts lib/lsp/lsp-providers.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint lib/api/domains/lsp-api.ts lib/state/slices/lsp hooks/domains/lsp lib/lsp hooks/use-lsp.ts
```

## Files likely touched

- `apps/web/lib/types/http-lsp.ts`
- `apps/web/lib/api/domains/lsp-api.ts`
- `apps/web/lib/api/domains/lsp-api.test.ts`
- `apps/web/lib/state/slices/lsp/lsp-slice.ts`
- `apps/web/lib/state/slices/lsp/lsp-slice.test.ts`
- `apps/web/lib/state/slices/lsp/types.ts`
- `apps/web/lib/state/slices/index.ts`
- `apps/web/lib/ws/handlers/lsp.ts`
- `apps/web/lib/ws/handlers/lsp.test.ts`
- `apps/web/hooks/domains/lsp/use-task-lsp.ts`
- `apps/web/hooks/domains/lsp/use-task-lsp.test.tsx`
- `apps/web/hooks/use-lsp.ts`
- `apps/web/hooks/use-lsp.test.tsx`
- `apps/web/lib/lsp/lsp-client-manager.ts`
- `apps/web/lib/lsp/lsp-client-types.ts`
- `apps/web/lib/lsp/lsp-client-storage.ts` (removed)
- `apps/web/lib/lsp/lsp-client-*.test.ts`
- `apps/web/lib/lsp/lsp-providers.ts`
- `apps/web/lib/lsp/lsp-editor-models.ts`
- `apps/web/hooks/use-lsp-file-opener.ts`

## Dependencies

Task 07 supplies stable task HTTP/events, revisions, lifecycle truth, and the attachment proxy.

## Parallelism

Sequential. Task 09 consumes these hooks, view data, and current-language attachment behavior.

## Inputs

- Spec: API surface; shared document/editor scenarios; browser persistence boundary.
- Current manager/provider/file URI/model isolation tests and state-slice/WS handler conventions.
- Frontend AGENTS rule: domain hooks/store own data access; components do not fetch directly.

## Output contract

Report task-key migration, removed session/localStorage/idle ownership, RED/GREEN test counts,
typecheck/lint results, and any protocol feature limitation. Update task/plan status and actual files.

## Results

Completed 2026-08-05.

- Added task/language HTTP types, strict action clients, a revision-aware Zustand slice, semantic
  WebSocket updates, and `useTaskLsp`. Lower revisions cannot rewind live state; equal revisions
  can carry newer task-host work evidence.
- Re-keyed the browser attachment manager to `(task, language)`. Same-task sessions share one
  attachment while session IDs remain only at the Monaco model/file edge. The attached envelope is
  now the capability/workspace source.
- Removed browser `initialize`, `initialized`, `shutdown`, `exit`, progress ownership, manual
  localStorage policy, and the two-minute editor idle timer. Final editor release closes only the
  downstream attachment.
- Preserved provider registration, diagnostic replay to every same-task session model, canonical
  document deduplication, navigation placeholders, TypeScript suppression, ordered changes, and
  save-race protection. Effective server configuration is supplied by the task host rather than an
  editor attachment.
- RED evidence: the initial API/slice/WS suites failed on missing task-domain modules; the new
  task-scope manager test failed with two session-keyed sockets; the hook suite failed before the
  task controller bridge existed.
- GREEN: `cd apps && pnpm --filter @kandev/web test -- --run lib/api/domains/lsp-api.test.ts lib/state/slices/lsp hooks/domains/lsp lib/ws/handlers/lsp.test.ts hooks/use-lsp.test.tsx lib/lsp/lsp-client-manager.test.ts lib/lsp/lsp-client-manager.task-scope.test.ts lib/lsp/lsp-client-manager.document-sync.test.ts lib/lsp/lsp-client-manager.progress.test.ts lib/lsp/lsp-providers.test.ts` — 10 files, 65 tests passed.
- GREEN: `cd apps/web && pnpm run typecheck`.
- GREEN: `cd apps/web && pnpm exec eslint lib/api/domains/lsp-api.ts lib/state/slices/lsp hooks/domains/lsp lib/ws/handlers/lsp.ts lib/lsp hooks/use-lsp.ts` — no findings.

PR remediation on 2026-08-06 made capacity merging independently sequence-aware. REST hydration and
live language events share the backend capacity epoch/revision; lower revisions and older backend
epochs cannot overwrite newer counts, while a newer backend epoch is accepted after restart.
The LSP slice race regressions, related frontend suites, and web typecheck pass.
