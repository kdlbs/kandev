---
id: "01-progress-protocol"
title: "Work-done progress protocol"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/lsp-file-intelligence.md"
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
- Review hardening: both Monaco save paths now flush the newest live content before emitting capability-gated `textDocument/didSave` after successful persistence. Unchanged buffers retain canonical repo-aware `includeText` snapshots; buffers edited during persistence remain dirty and omit stale optional save text instead of rewinding LSP state.
- Review hardening: LSP navigation into an attached repository preserves an existing task-root-relative editor identity, preventing duplicate tabs and keeping cursor reveal on the tree-opened file.
- Review hardening: command-palette content-search selection now carries the active session through pending and immediate cursor reveal, so identical open repository paths in other task sessions cannot make the jump ambiguous.
- Review hardening: installer failures without details, WebSocket errors, and reasonless pre-bridge or post-bridge closes now resolve their fallback copy through the active locale instead of leaking English into localized status UI and toasts.
- Review hardening: categorical task-host close codes for unsupported executors, capacity, and stream failures ignore English transport prose and resolve their status through the active frontend locale.
- Review hardening: completion items without `textEdit` use Monaco's current-word range while `InsertReplaceEdit` keeps its dual range, live LSP JSON settings update the reused connection and its configuration request handler, and task hosts without an installer close with localized manual-install guidance through `4007` before or after preference opt-in.
- Review hardening: TypeScript built-in suppression now follows per-connection model ownership instead of a global flag, so unrelated sessions keep Monaco intelligence; valid empty semantic-token arrays return an empty payload without periodic client polling.
- Review hardening: Go post-install discovery now includes the task environment's `USERPROFILE\go\bin`, covering default Windows Local PC setups without explicit `GOBIN`, `GOPATH`, or `HOME`.
- Review hardening: the browser-facing LSP capacity slot is acquired before `GetOrEnsureExecution`, so an over-cap request cannot start or resume a supported task host; all resolution failures release the provisional slot.
- Review hardening: missing-binary close code `4001` now ignores backend prose and resolves through the active locale, Editors cards expose install prerequisites inline for pointer, keyboard, and touch users, and the test-only duplicate language table was removed in favor of the localized Settings metadata source.
- Review hardening: the Monaco editor opener now opens regular targets inside the active task workspace and propagates `false` for unresolved targets, while agentctl reports unexpected server exit as categorical `4006` with no English close reason so the browser localizes it.
- Review hardening: JSON-RPC initialize errors now preserve their `message` instead of rendering `[object Object]`, while task-host process-launch failures retain details in backend logs and close through localized categorical `4008` with no English transport reason.
- Review hardening: task teardown during auto-install keeps the standard `1001` going-away close but omits the English `task stopping` reason, allowing the browser's existing localized pre-bridge fallback to own the user-visible status.
- Review hardening: Monaco completion registration now uses only the trigger characters advertised by the active server, and a named 25-value mapping preserves every standard LSP completion category in Monaco instead of relying on mismatched numeric enums.
- Review hardening: Monaco signature help is absent when the server omits that capability and otherwise preserves the server's trigger and retrigger characters; a bare install-failure close ignores generic task-host prose and resolves through the active locale while a preceding detailed installer payload remains authoritative.
- Review hardening: LSP completion-list `isIncomplete` state reaches Monaco so continued typing can request refreshed results through trigger kind `3` instead of retaining stale partial suggestions.
- Review hardening: completion, hover, definition, references, and signature help register only when advertised; TypeScript built-in suppression now follows the exact overlapping provider set, preserving built-in fallbacks for omitted capabilities and unwired rename, code-action, highlight, and inlay features.
- Review hardening: every task-host LSP WebSocket frame now uses the same five-second write deadline, so an unread pre-bridge status or ready frame cannot pin the handler or leak its owned language-server process.
- Review hardening: a terminating stdout forwarder closes language-server stdin to release active writes immediately, while a cross-platform 30-second cutoff closes a pipe whose server remains alive but stops reading; both paths converge on owned-process cleanup.
- Review hardening: a single pending task-host WebSocket read cancels connection-owned auto-install work when the browser stops or disconnects, then hands the first client frame into the bridge after a successful install without concurrent reads or a dropped initialize request.
- Review hardening: a failed or timed-out initial `installing` WebSocket write now closes the stream before agentctl acquires auto-install work; the forced-write-failure regression proves the installer is never entered without a consumer.
- Review hardening: Go post-install lookup now rejects relative `GOBIN`, every relative `GOPATH` entry, `HOME`, and `USERPROFILE`, preventing repository-controlled directories from supplying the launched `gopls` binary while retaining absolute task-host fallbacks.
- Verified:
  - `pnpm --filter @kandev/web test -- --run lib/lsp/lsp-progress.test.ts lib/lsp/lsp-client-manager.test.ts`
  - `pnpm exec vitest run lib/lsp/lsp-providers.test.ts --reporter=dot`
  - `pnpm exec vitest run lib/lsp/lsp-providers.test.ts lib/lsp/lsp-client-manager.test.ts lib/lsp/lsp-client-manager.progress.test.ts` (45 passed, including advertised completion triggers and all 25 standard completion kinds)
  - `pnpm exec vitest run lib/lsp/lsp-providers.test.ts lib/lsp/lsp-language-mapping.test.ts lib/lsp/lsp-client-manager.progress.test.ts` (33 passed, including signature-help capability gating/triggers and localized bare `4003` fallback)
  - `pnpm exec vitest run lib/lsp/lsp-providers.test.ts lib/lsp/lsp-client-manager.test.ts lib/lsp/lsp-client-manager.progress.test.ts lib/lsp/lsp-language-mapping.test.ts` (55 passed, including incomplete completion-list preservation)
  - `pnpm exec vitest run lib/lsp/lsp-providers.test.ts components/editors/monaco/builtin-providers.test.ts lib/lsp/lsp-client-manager.test.ts lib/lsp/lsp-client-manager.progress.test.ts lib/lsp/lsp-client-manager.document-sync.test.ts lib/lsp/lsp-language-mapping.test.ts` (63 passed, including optional-provider gating and feature-scoped TypeScript fallback)
  - `pnpm exec vitest run lib/lsp/lsp-providers.test.ts components/editors/monaco/builtin-providers.test.ts lib/lsp/lsp-client-manager.test.ts lib/lsp/lsp-client-manager.progress.test.ts lib/lsp/lsp-client-manager.document-sync.test.ts` (49 passed)
  - `pnpm run typecheck`
  - `pnpm exec eslint lib/lsp/lsp-providers.ts lib/lsp/lsp-providers.test.ts e2e/tests/lsp/lsp-file-intelligence.spec.ts`
  - `go test ./internal/agentctl/server/api ./internal/lsp/installer ./internal/tools/installer`
  - `go test ./internal/tools/installer ./internal/lsp/installer` (Windows USERPROFILE result discovery included)
  - `GOOS=windows GOARCH=amd64 go test -c ./internal/tools/installer`
  - `go test ./internal/agent/runtime/lifecycle ./internal/gateway/websocket`
  - `go test -race ./internal/gateway/websocket -run 'Test(HandleLSPConnectionChecksCapacityBeforeEnsuringExecution|ResolveLSPExecution)'` (capacity ordering and release)
  - `make lint`
  - `GOOS=windows GOARCH=amd64 go test -c ./internal/lsp/installer` and `./internal/agentctl/server/api`
  - `pnpm e2e:run -- --project=chromium tests/lsp/lsp-file-intelligence.spec.ts` (13 passed)
  - `pnpm e2e:run tests/lsp/lsp-file-intelligence.spec.ts -- --grep "runs Kotlin intelligence through the task host"` (1 production-build Chromium scenario passed)
  - `pnpm exec vitest run lib/lsp/lsp-document-sync.test.ts lib/lsp/lsp-client-manager.test.ts lib/lsp/lsp-client-manager.document-sync.test.ts hooks/use-file-save-delete.test.ts components/task/task-center-panel-restoration.test.ts` (46 passed)
  - `pnpm exec vitest run hooks/use-file-save-delete.test.ts hooks/use-lsp-file-opener.test.ts lib/lsp/lsp-client-manager.document-sync.test.ts components/task/task-center-panel-restoration.test.ts lib/lsp/lsp-document-sync.test.ts` (35 passed)
  - `pnpm test` (1,122 files; 8,657 passed, 4 skipped)
  - `pnpm lint`, `pnpm run typecheck`, `pnpm i18n:check`, and `pnpm i18n:ratchet`
  - `pnpm exec vitest run lib/lsp/lsp-language-mapping.test.ts components/settings/lsp-language-cards.test.tsx components/settings/lsp-language-options.test.ts` (13 passed)
  - `pnpm exec eslint lib/lsp/lsp-json-rpc.ts lib/lsp/lsp-language-mapping.test.ts components/settings/lsp-language-cards.tsx components/settings/lsp-language-cards.test.tsx components/settings/lsp-language-options.ts components/settings/lsp-language-options.test.ts`
  - `pnpm exec vitest run hooks/use-lsp-file-opener.test.ts components/editors/monaco/monaco-init.test.ts lib/lsp/lsp-language-mapping.test.ts components/settings/lsp-language-cards.test.tsx components/settings/lsp-language-options.test.ts` (23 passed)
  - `go test ./internal/agentctl/server/api -count=1` (categorical server-exit close included)
  - `pnpm exec vitest run lib/lsp/lsp-client-manager.test.ts lib/lsp/lsp-client-manager.progress.test.ts lib/lsp/lsp-language-mapping.test.ts hooks/use-lsp-file-opener.test.ts components/editors/monaco/monaco-init.test.ts components/settings/lsp-language-cards.test.tsx components/settings/lsp-language-options.test.ts` (56 passed)
  - `go test ./internal/agentctl/server/api -count=1` (categorical process-start failure included)
  - `go test ./internal/agentctl/server/api -run 'TestHandleLSPStream(BridgesFramesAndStopsOwnedProcess|StopsProcessWhenForwardingToWebSocketFails|PeerCloseReleasesBlockedForwarderWrite)' -count=1` (ready-frame deadline and bridge cleanup)
  - `go test ./internal/agentctl/server/api -count=1` (bounded task-host status and ready writes included)
  - `go test ./internal/agentctl/server/api -run 'Test(RunLSPBridgeForwarderExitUnblocksStdinWrite|WriteLSPStdinWithTimeoutClosesBlockedWrite)' -count=1` (forwarder cleanup and virtual-time stdin cutoff)
  - `go test -race ./internal/agentctl/server/api -run 'Test(RunLSPBridgeForwarderExitUnblocksStdinWrite|WriteLSPStdinWithTimeoutClosesBlockedWrite)' -count=1`
  - `go test ./internal/agentctl/server/api -run 'Test(HandleLSPStreamAutoInstallHandsFirstClientMessageToBridge|LSPAutoInstallIsCanceledByClientDisconnect|LSPAutoInstallIsCanceledAndDrainedByInstanceTeardown)' -count=1`
  - `go test -race ./internal/agentctl/server/api -run 'Test(HandleLSPStreamAutoInstallHandsFirstClientMessageToBridge|LSPAutoInstallIsCanceledByClientDisconnect)' -count=1`
  - `go test ./internal/tools/installer -run 'Test(FindGoBinaryWithRunnerRejectsRelativeDirectories|FindGoBinaryWithRunnerUsesUserProfileFallback|GoInstallStrategyUsesRunnerEnvironmentForLookupAndResult)' -count=1`
  - `go test ./internal/tools/installer -count=1`
  - `node --test scripts/validate-public-docs.test.mjs scripts/notify-docs-workflow.test.mjs && node scripts/validate-public-docs.mjs`
