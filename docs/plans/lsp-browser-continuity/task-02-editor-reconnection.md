---
id: "02-editor-reconnection"
title: "Editor reconnection and status"
status: done
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
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.8
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.9
system_design:
  - ../../specs/platform/system-design/lsp-file-intelligence-01.md
  - ../../specs/platform/system-design/lsp-file-intelligence-02.md
---

# Task 02: Editor reconnection and status

## Summary

Let a returning Monaco editor reattach to a retained LSP lease, rebuild its providers, and show current status and analysis. Recover brief transport loss automatically while keeping actual server exits actionable.

## In scope

- Store an opaque lease hint for tab restoration, consume the resumed handshake with effective dynamic registrations and without a second `initialize`, register providers, and synchronize current documents before accepting fresh diagnostics.
- Add bounded reconnect attempts and a localized reconnecting status. Keep Stop as an acknowledged explicit action, send acknowledged release at the existing two-minute last-editor idle timeout, and preserve browser-local manual enablement. A hinted existing lease may resume after auto-start is disabled, unless explicitly stopped; an unhinted new tab follows current policy. Duplicated tabs never steal an attached lease.
- Add the frontend feature key and enabled/disabled client behavior. When the flag is off, preserve the existing browser-owned connection and close-code handling. Translate `4006` only as confirmed process exit and `4009` or abnormal close as transport failure when on.
- Reuse the existing desktop toolbar/status-bar and coarse-pointer tablet drawer; keep phone viewing LSP-free. Add focused unit/component coverage and all required locales.

## Out of scope

- A new LSP dashboard, phone controls, or layout redesign.
- Browser E2E and public documentation, covered by Task 03.

## Acceptance

1. A resumed connection rebuilds Monaco providers from retained and dynamic capabilities, synchronizes open documents, shows current project progress without a second LSP initialize request, and accepts only fresh qualifying diagnostics.
2. Unexpected transport loss (`4009` or abnormal close) shows reconnecting and retries the retained lease; confirmed process exit (`4006`) shows server-exited Retry; stale responses from a replaced browser attachment cannot change the new status.
3. Explicit Stop ends only this window's lease, and tablet shows the same state/action in its existing drawer while phone mounts no LSP connection.
4. The last-editor idle timeout releases its lease, a duplicated tab cannot steal an attached lease, and a hinted tab resumes its lease after an auto-start preference change unless explicitly stopped.

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
(cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm exec eslint lib/lsp hooks/use-lsp.ts components/editors/lsp-status-button.tsx)
```

## Files likely touched

- `apps/web/lib/lsp/lsp-client-manager.ts` and its focused tests
- `apps/web/lib/lsp/lsp-json-rpc.ts`, progress and document-sync helpers
- `apps/web/hooks/use-lsp.ts` and tests
- `apps/web/lib/state/slices/features/types.ts` and feature contract tests
- `apps/web/components/editors/lsp-status-button.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/lsp.json` and verbatim records only if needed

## Dependencies

Task 01 supplies the lease/resume protocol and close-code distinction.

## Risks

- Monaco provider disposal during reconnect can erase another session's markers or leave stale built-in suppression.
- A tab with unsaved edits must not show diagnostics produced for the previous attachment's buffer.
- A server that omits diagnostic versions cannot restore markers safely on a resumed lease; the editor keeps markers clear until a fresh process starts.
- Automatic retries must be bounded and cancel when the user explicitly stops or leaves the editor.
- Disabled rollout must preserve the old `4006` proxy-stream handling; `4009` must never inherit the true process-exit label.

## Parallelism

`sequential`

## Inputs

- [Plan and preview](plan.md)
- `REQ-PLATFORM-LSP-FILE-INTELLIGENCE-002` and both linked system designs.
- Task 01's tested handshake and close-code contract.

## Results

Implemented lease-hint restoration, bounded reconnect, resumed capability/progress state, document synchronization before diagnostics, acknowledged Stop/idle release, localized reconnect status, feature gating, and the phone LSP boundary. The focused web LSP, hook, storage, dynamic-capability, and feature-contract suite passed (58 tests); web typecheck, changed-file ESLint, and the pseudo-locale/i18n checks passed.

Code-review remediation advertises dynamic registration only for the supported completion, hover, definition, references, signature-help, and semantic-token providers. The semantic-token registration maps to Monaco's supported capability shape. A resumed handshake that reports `initialized: false` sends the one-time `initialized` notification before document synchronization and attachment readiness. All 134 LSP Vitest tests passed; web typecheck and changed-file ESLint passed.
