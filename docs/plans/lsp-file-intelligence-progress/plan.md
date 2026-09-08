---
spec: docs/specs/platform/requirements/lsp-file-intelligence.md
created: 2026-07-27
status: completed
---

# Implementation Plan: LSP Project Progress

## Overview

Extend the browser-owned LSP connection with standard work-done progress before changing the toolbar presentation. The connection generation owns all runtime progress so replacement and teardown cannot leak stale work. A shared details body then renders in an anchored fine-pointer popover or coarse-pointer tablet drawer, followed by deterministic desktop and tablet Playwright coverage.

## Backend

The progress protocol itself requires no backend payload transform because both WebSocket proxy hops forward JSON-RPC bodies unchanged. Review hardening also makes task-host binary discovery, managed cache roots, and npm/Go auto-install resolve through the process manager's task environment, resolves Windows npm shims through PATHEXT and Go's default workspace through `USERPROFILE`, rejects Rust auto-install on task hosts without a packaged strategy through a distinct browser-visible close code, reports process-start failure through categorical `4008` while retaining the host error in logs, keeps task-host decisions out of the main backend's global settings policy, checks the persisted executor runtime before any cold execution is created for LSP, and acquires connection capacity before a supported task host can start or resume.

## Frontend

### Progress contract and connection ownership

- Add `apps/web/lib/lsp/lsp-progress.ts` with pure validation and immutable transitions for `begin`, `report`, and `end` payloads.
- Extend `ManagedLspConnection` with generation-owned progress state and registered string/number tokens.
- Update `lsp-client-manager.ts` to advertise `window.workDoneProgress`, supply a client-generated initialize token, accept `window/workDoneProgress/create`, consume `$/progress`, expose a referentially stable progress snapshot, and notify subscribers.
- Clear initialization and work state on stop, idle teardown, crash, retry, or connection replacement.
- Keep model-scoped TypeScript suppression separate from the synchronous LSP-provider registration guard so cold Monaco loads still wrap overlapping lazy built-in providers while unrelated sessions, unadvertised capabilities, and unwired Monaco features retain built-in intelligence.
- Preserve detailed pre-bridge install errors when the following WebSocket close contains only a generic mapped reason.
- Preserve the message from a JSON-RPC initialize error object while translating categorical task-host close codes instead of rendering backend prose.
- Translate Monaco completion trigger context into the LSP request context advertised by the client capability.
- Register completion, hover, definition, references, and signature help only when the initialized server advertises each provider; use exact completion triggers and signature-help trigger/retrigger characters.
- Supply Monaco's current-word range for completion items without `textEdit`, while preserving explicit server `TextEdit` and `InsertReplaceEdit` ranges.
- Keep per-language configuration mutable on the shared connection, notify initialized servers when settings change, and answer later `workspace/configuration` requests from that live value.
- After confirmed persistence, synchronously flush the newest live editor snapshot as `textDocument/didChange`, then route `textDocument/didSave` to matching open documents for servers that requested save synchronization. Include the persisted snapshot only for `includeText` servers when the buffer has not advanced; raced saves omit stale optional text and retain the newer dirty buffer.
- Resolve LSP targets and regular in-workspace Monaco `file://` targets to repository-scoped workspace requests while preserving an existing task-root-relative editor key; return the opener's handled result so targets outside the active workspace continue through Monaco rather than being swallowed.

### Status disclosure

- Extend `useLsp` and `useMonacoEditorLsp` to expose the current progress snapshot.
- Replace the one-click toolbar toggle with a disclosure-first `LspStatusButton`.
- Keep an explicit per-session/language Stop override in the browser runtime so global auto-start cannot reacquire a lease on later editor or configuration renders; explicit Start clears the override.
- Add a shared progress body that separates connection readiness from project work, renders a determinate bar only for server percentages, uses tabular elapsed time, preserves concurrent-work counts, and avoids project-wide completion claims.
- Use `useTouchDrawer`: fine pointers receive an anchored popover; coarse-pointer Monaco/tablet layouts receive an inset bottom drawer with one internal scroll owner and touch-sized controls. Phone CodeMirror viewing remains unchanged.
- Gate each Editors auto-install checkbox with the task-host-independent preference list. Forward enabled preferences to agentctl so its actual platform decides whether installation can run, and render each language's install prerequisites and destination visibly from the same localized metadata source used by Settings.

## Tests

- **Work progress transitions:** `apps/web/lib/lsp/lsp-progress.test.ts` covers token validation, clamping, omitted-field preservation, independent concurrent tokens, unknown/malformed payloads, completion, and reset.
- **Protocol integration:** `apps/web/lib/lsp/lsp-client-manager.test.ts` proves initialize capability/token advertisement, pre-initialize progress, server-created numeric tokens, subscriber updates, and stale-generation isolation.
- **Document synchronization:** focused manager, capability, and both save-hook tests prove canonical repo-aware routing, `includeText` handling, `didChange`-before-`didSave` ordering, preservation of edits made during persistence, and no save notification after rejected persistence.
- **Navigation identity:** file-opener and Monaco registration tests prove an attached-repository target reuses an existing task-root-relative editor key, regular in-workspace `file://` targets open, and unresolved targets return unhandled to Monaco.
- **Completion, signature help, and configuration:** focused provider and manager tests prove completion range fallback/override behavior, incomplete-list preservation, capability-gated optional provider registration, advertised completion and signature-help triggers, all 25 standard completion-kind mappings, connection reuse, live configuration notification, and updated request responses.
- **Provider ownership and semantic tokens:** focused Monaco/manager/provider tests prove TypeScript built-in suppression follows both the owning session model and matching advertised provider, leaves unadvertised and unwired features available, disposes independent connections separately, and completes valid empty semantic-token arrays without self-scheduled polling.
- **Task-host setup guidance:** agentctl and frontend mapping tests prove a task host without an installer closes with `4007` before or after auto-install opt-in, process launch failure closes with `4008`, and categorical statuses—including a bare `4003` without a detailed status payload—render localized guidance instead of backend transport prose.
- **Task-host transport safety:** agentctl tests observe the bounded write deadline on the pre-bridge ready frame, prove stdout-forwarder exit releases an active stdin write, use virtual time to verify the 30-second stdin cutoff, cancel an active auto-install on browser disconnect, and hand the pending first client frame into the post-install bridge; existing bridge tests retain process-cleanup coverage for failed and blocked WebSocket writes.
- **Initialization failure:** the manager regression proves a JSON-RPC initialize error object renders its `message` field and still cleans up the failed connection for Retry.
- **Presentation helpers:** focused pure-helper tests cover labels, lifecycle actions, and locale-aware elapsed-time formatting; a rendered language-card regression proves auto-install prerequisites remain visible without tooltip interaction.
- **Auto-start lifecycle:** hook coverage stops an auto-started lease, rerenders with changed configuration, and proves only explicit Start reacquires it; desktop E2E repeats the boundary across a matching editor reopen.
- **Task-host environment:** focused Go tests cover absolute-only PATH and Go result directories, task-HOME cache roots, GOBIN and Windows USERPROFILE result lookup, pre-install registry discovery, PATHEXT-aware managed npm shims, platform-gated Rust installation, process-manager environment exposure, read-only rejection of cold unsupported executors, and capacity rejection before supported execution startup.

## E2E Tests

- **Desktop reported progress:** extend `apps/web/e2e/tests/lsp/lsp-file-intelligence.spec.ts` and the fake server so a held initialize operation reports title, message, and percentage; verify the popover, incomplete-results warning, completion copy, and Stop action.
- **Save synchronization:** the full task-host protocol scenario saves a navigated Kotlin file immediately after editing, then verifies canonical `didChange`-before-`didSave` delivery with the persisted content before continuing editor-provider checks.
- **Completion retriggering:** the full task-host protocol scenario returns an incomplete completion list, types another character, and verifies Monaco follows up with LSP trigger kind `3` before accepting the refreshed item.
- **Desktop no-report fallback:** verify an initialized server that emits no work progress says so without a percentage or ETA.
- **Coarse-pointer tablet:** extend `mobile-lsp-file-intelligence.spec.ts` at tablet width to verify the same progress in a bottom drawer, a touch-sized trigger, viewport containment, and no horizontal overflow.
- Preserve the existing phone assertion that no LSP process or control starts.

## Mobile Design Contract

- **Desktop outcome:** the file-toolbar status control opens detailed connection and project-work state.
- **Mobile entry point:** phone has none because LSP remains unsupported there; a coarse-pointer tablet uses the same Monaco toolbar trigger.
- **Nearest exemplar:** `PRStatusChipDrawer` supplies the popover/drawer split, fixed drawer header, and internal scrolling geometry.
- **Hierarchy and action:** connection state first, oldest active work item second, warning and concurrent count next, one Start/Stop/Retry action last.
- **Surface:** anchored popover for fine pointers; inset bottom drawer for coarse pointers because the content is temporary status detail.
- **Geometry:** the drawer owns vertical scrolling, stays below `80dvh`, clears shared safe-area treatment, uses 44px touch controls, and does not introduce document horizontal overflow.
- **Shared logic:** one progress snapshot, formatter, and body drive both presentations.

## Risks

- LSP percentages are optional and do not represent a universal project-wide ETA.
- Servers may create several simultaneous tokens or send malformed/late frames.
- Progress can begin before the initialize response, so handlers must be installed first.
- Existing E2E flows assume clicking the toolbar control starts or stops immediately and must migrate to explicit actions.

## Implementation Tasks

- [x] [Task 01: Work-done progress protocol](task-01-progress-protocol.md) (completed)
- [x] [Task 02: Responsive progress disclosure](task-02-progress-disclosure.md) (completed)
- [x] [Task 03: Desktop and tablet E2E](task-03-progress-e2e.md) (completed)

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/lsp/lsp-progress.test.ts lib/lsp/lsp-client-manager.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint lib/lsp hooks/use-lsp.ts components/editors
make -C apps/backend test
cd apps/web && pnpm e2e:run tests/lsp/lsp-file-intelligence.spec.ts
cd apps/web && pnpm e2e:run --no-build -- --project=mobile-chrome tests/lsp/mobile-lsp-file-intelligence.spec.ts
cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --no-build -- --project=containers tests/docker/lsp-file-intelligence.spec.ts tests/ssh/lsp-unsupported-executor.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```
