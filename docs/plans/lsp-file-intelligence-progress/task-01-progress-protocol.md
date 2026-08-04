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
- Verified:
  - `pnpm --filter @kandev/web test -- --run lib/lsp/lsp-progress.test.ts lib/lsp/lsp-client-manager.test.ts`
  - `pnpm run typecheck`
