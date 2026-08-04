---
spec: docs/specs/lsp-file-intelligence/spec.md
created: 2026-07-27
status: completed
---

# Implementation Plan: LSP Project Progress

## Overview

Extend the browser-owned LSP connection with standard work-done progress before changing the toolbar presentation. The connection generation owns all runtime progress so replacement and teardown cannot leak stale work. A shared details body then renders in an anchored fine-pointer popover or coarse-pointer tablet drawer, followed by deterministic desktop and tablet Playwright coverage.

## Backend

The progress protocol itself requires no backend payload transform because both WebSocket proxy hops forward JSON-RPC bodies unchanged. Review hardening also makes task-host binary discovery and npm/Go auto-install resolve commands and results through the process manager's task environment, resolves Windows npm shims through PATHEXT, rejects Rust auto-install on task hosts without a packaged strategy, and keeps that task-host decision out of the main backend's global settings policy.

## Frontend

### Progress contract and connection ownership

- Add `apps/web/lib/lsp/lsp-progress.ts` with pure validation and immutable transitions for `begin`, `report`, and `end` payloads.
- Extend `ManagedLspConnection` with generation-owned progress state and registered string/number tokens.
- Update `lsp-client-manager.ts` to advertise `window.workDoneProgress`, supply a client-generated initialize token, accept `window/workDoneProgress/create`, consume `$/progress`, expose a referentially stable progress snapshot, and notify subscribers.
- Clear initialization and work state on stop, idle teardown, crash, retry, or connection replacement.
- Keep runtime TypeScript suppression separate from the synchronous LSP-provider registration guard so cold Monaco loads still wrap lazy built-in providers.
- Preserve detailed pre-bridge install errors when the following WebSocket close contains only a generic mapped reason.

### Status disclosure

- Extend `useLsp` and `useMonacoEditorLsp` to expose the current progress snapshot.
- Replace the one-click toolbar toggle with a disclosure-first `LspStatusButton`.
- Add a shared progress body that separates connection readiness from project work, renders a determinate bar only for server percentages, uses tabular elapsed time, preserves concurrent-work counts, and avoids project-wide completion claims.
- Use `useTouchDrawer`: fine pointers receive an anchored popover; coarse-pointer Monaco/tablet layouts receive an inset bottom drawer with one internal scroll owner and touch-sized controls. Phone CodeMirror viewing remains unchanged.
- Gate each Editors auto-install checkbox with the task-host-independent preference list. Forward enabled preferences to agentctl so its actual platform decides whether installation can run.

## Tests

- **Work progress transitions:** `apps/web/lib/lsp/lsp-progress.test.ts` covers token validation, clamping, omitted-field preservation, independent concurrent tokens, unknown/malformed payloads, completion, and reset.
- **Protocol integration:** `apps/web/lib/lsp/lsp-client-manager.test.ts` proves initialize capability/token advertisement, pre-initialize progress, server-created numeric tokens, subscriber updates, and stale-generation isolation.
- **Presentation helpers:** focused pure-helper tests cover labels, lifecycle actions, and locale-aware elapsed-time formatting without adding shallow React markup tests.
- **Task-host environment:** focused Go tests cover PATH-based command discovery, GOBIN result lookup, pre-install registry discovery, PATHEXT-aware managed npm shims, platform-gated Rust installation, and process-manager environment exposure.

## E2E Tests

- **Desktop reported progress:** extend `apps/web/e2e/tests/lsp/lsp-file-intelligence.spec.ts` and the fake server so a held initialize operation reports title, message, and percentage; verify the popover, incomplete-results warning, completion copy, and Stop action.
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
