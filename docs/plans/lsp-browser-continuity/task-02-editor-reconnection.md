---
id: "02-editor-reconnection"
title: "Editor reconnection and status"
status: pending
wave: 2
depends_on:
  - "01-runtime-lsp-leases"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-LSP-FILE-INTELLIGENCE-002
acceptance_criteria:
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.2
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.3
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.4
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.6
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.7
system_design:
  - ../../specs/platform/system-design/lsp-file-intelligence-01.md
  - ../../specs/platform/system-design/lsp-file-intelligence-02.md
---

# Task 02: Editor reconnection and status

## Summary

Let a returning Monaco editor reattach to a retained LSP lease, rebuild its providers, and show current status and analysis. Recover brief transport loss automatically while keeping actual server exits actionable.

## In scope

- Store an opaque lease hint for tab restoration, consume the resumed handshake without a second `initialize`, register providers before replay, and synchronize current documents before accepting diagnostics.
- Add bounded reconnect attempts and a localized reconnecting status. Keep Stop as an acknowledged explicit action and preserve browser-local manual enablement.
- Reuse the existing desktop toolbar/status-bar and coarse-pointer tablet drawer; keep phone viewing LSP-free. Add focused unit/component coverage and all required locales.

## Out of scope

- A new LSP dashboard, phone controls, or layout redesign.
- Browser E2E and public documentation, covered by Task 03.

## Acceptance

1. A resumed connection rebuilds Monaco providers from retained capabilities, synchronizes open documents, and restores matching diagnostics and current project progress without a second LSP initialize request.
2. Unexpected transport loss shows reconnecting and retries the retained lease; true process exit shows server-exited Retry; stale responses from a replaced browser attachment cannot change the new status.
3. Explicit Stop ends only this window's lease, and tablet shows the same state/action in its existing drawer while phone mounts no LSP connection.

## ASCII UI preview

`UI-01: Desktop active Monaco file after transient disconnection` and `UI-02: Coarse-pointer tablet LSP drawer` follow the [combined preview](plan.md#ascii-ui-preview) (AC .2, .3, .7).

```text
Desktop: Go  [~] Reconnecting to language server... [Stop]
         Go  [o] Ready | Project work: <server report> [Stop]
Tablet:  [Go LSP: Reconnecting] -> drawer -> status + [Stop]
Phone:   file header + content; no LSP control or attachment
```

The reconnecting state and action are required. Copy and spacing are illustrative and localized. Existing desktop placement, tablet drawer geometry, and phone viewer composition remain the structural baseline.

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/lsp/lsp-client-manager.test.ts lib/lsp/lsp-client-manager.progress.test.ts lib/lsp/lsp-client-manager.document-sync.test.ts hooks/use-lsp.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm exec eslint lib/lsp hooks/use-lsp.ts components/editors/lsp-status-button.tsx)
```

## Files likely touched

- `apps/web/lib/lsp/lsp-client-manager.ts` and its focused tests
- `apps/web/lib/lsp/lsp-json-rpc.ts`, progress and document-sync helpers
- `apps/web/hooks/use-lsp.ts` and tests
- `apps/web/components/editors/lsp-status-button.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/lsp.json` and verbatim records only if needed

## Dependencies

Task 01 supplies the lease/resume protocol and close-code distinction.

## Risks

- Monaco provider disposal during reconnect can erase another session's markers or leave stale built-in suppression.
- A tab with unsaved edits must not show diagnostics produced for the previous attachment's buffer.
- Automatic retries must be bounded and cancel when the user explicitly stops or leaves the editor.

## Parallelism

`sequential`

## Inputs

- [Plan and preview](plan.md)
- `REQ-PLATFORM-LSP-FILE-INTELLIGENCE-002` and both linked system designs.
- Task 01's tested handshake and close-code contract.

## Results

Pending.
