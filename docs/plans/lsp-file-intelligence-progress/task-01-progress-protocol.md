---
id: "01-progress-protocol"
title: "Work-done progress protocol"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 01: Work-Done Progress Protocol

## Acceptance

- The initialize request advertises work-done support and carries a connection-generation token before any server progress can arrive.
- Valid `begin`, `report`, and `end` notifications update only their registered string or number token; malformed, unknown, and stale-generation frames do nothing.
- Initialization, active work, and the most recently completed server item are observable through one stable manager snapshot and clear with connection ownership.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/lsp/lsp-progress.test.ts lib/lsp/lsp-client-manager.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/lib/lsp/lsp-progress.ts`
- `apps/web/lib/lsp/lsp-progress.test.ts`
- `apps/web/lib/lsp/lsp-client-types.ts`
- `apps/web/lib/lsp/lsp-json-rpc.ts`
- `apps/web/lib/lsp/lsp-client-manager.ts`
- `apps/web/lib/lsp/lsp-client-manager.test.ts`

## Dependencies

None.

## Parallelism

Sequential; Task 02 consumes this snapshot.

## Inputs

- Spec sections: Browser LSP progress contract, Readiness and progress state, Failure modes.
- Existing generation checks in `lsp-client-manager.ts`.

## Output Contract

Record RED/GREEN evidence, files changed, exact tests run, remaining risks, and update this task plus `plan.md`.

## Result

- RED: the manager test proved `window.workDoneProgress` and the initialize token were absent; transition tests then proved begin/report/end state was unimplemented.
- GREEN: the client now advertises and registers generation-owned tokens, tracks initialize timing and immutable work snapshots, and ignores malformed, unknown, or stale progress.
- Review hardening: unexpected closes after readiness now retain an error status and close reason for Retry; explicit stop and idle cleanup still clear to disabled.
- Review hardening: a dedicated synchronous registration guard now distinguishes LSP TypeScript providers from Monaco's lazy built-ins, and built-in suppression waits until Monaco's wrappers are installed.
- Review hardening: a generic WebSocket close no longer overwrites a detailed install failure already reported by agentctl.
- Review hardening: completion providers forward Monaco trigger context using LSP enum values, and managed installer caches resolve from the merged task `HOME`.
- Review hardening: cold SSH, Sprites, and remote-Docker sessions are rejected through a read-only runtime lookup before LSP can create or resume an execution.
- Review hardening: both Monaco save paths now flush a pending content change before emitting capability-gated `textDocument/didSave` after successful persistence, with canonical repo-aware URIs and `includeText` snapshots when requested.
- Verified:
  - `pnpm --filter @kandev/web test -- --run lib/lsp/lsp-progress.test.ts lib/lsp/lsp-client-manager.test.ts`
  - `pnpm exec vitest run lib/lsp/lsp-providers.test.ts --reporter=dot`
  - `pnpm run typecheck`
  - `pnpm exec eslint lib/lsp/lsp-providers.ts lib/lsp/lsp-providers.test.ts e2e/tests/lsp/lsp-file-intelligence.spec.ts`
  - `go test ./internal/agentctl/server/api ./internal/lsp/installer ./internal/tools/installer`
  - `go test ./internal/agent/runtime/lifecycle ./internal/gateway/websocket`
  - `make lint`
  - `GOOS=windows GOARCH=amd64 go test -c ./internal/lsp/installer` and `./internal/agentctl/server/api`
  - `pnpm e2e:run -- --project=chromium tests/lsp/lsp-file-intelligence.spec.ts` (13 passed)
  - `pnpm exec vitest run lib/lsp/lsp-document-sync.test.ts lib/lsp/lsp-client-manager.test.ts lib/lsp/lsp-client-manager.document-sync.test.ts hooks/use-file-save-delete.test.ts components/task/task-center-panel-restoration.test.ts` (46 passed)
  - `node --test scripts/validate-public-docs.test.mjs scripts/notify-docs-workflow.test.mjs && node scripts/validate-public-docs.mjs`
