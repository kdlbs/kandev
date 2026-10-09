---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-LSP-CONTINUITY-GRADUATION-001
created: 2026-09-25
owners:
  - kandev
---

# LSP Continuity Graduation System Design

## Purpose and boundaries

This design retires the release gate around the broker described in
[LSP file intelligence](lsp-file-intelligence-01.md). It preserves the task-owned
lease decision in [ADR-2026-09-23](../../../decisions/2026-09-23-task-owned-lsp-leases.md)
and the separate user settings for auto-start and auto-install.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-LSP-CONTINUITY-GRADUATION-001` | Lease composition and verification |

## Lease composition and retirement

Promote all shipped profile defaults to true while retaining the runtime
definition, override behavior, and browser-owned fallback for one stable
release. Use that release to exercise reconnect, capacity, explicit Stop,
idle-release, and backend/task-host shutdown on supported Local PC and Local
Docker executors.

In the retirement release, pass the lease manager into the WebSocket gateway
unconditionally and remove the browser-owned LSP proxy branch. Remove the
frontend `useFeature("lspBrowserContinuity")` branch so the LSP client always
uses the lease handshake when it starts. Keep the existing LSP start policy,
executor gate, authorization, capacity accounting, attachment generation checks,
and diagnostic synchronization. Phone remains a file viewer with no LSP client.

Remove the profile key, `FeaturesConfig.LSPBrowserContinuity`, active runtime
definition, `/api/v1/features` field, and frontend default. Retire
`features.lspBrowserContinuity` / `KANDEV_FEATURES_LSP_BROWSER_CONTINUITY` in
the append-only registry; old override rows remain inert. Remove any live
startup-catalog classification of the old environment variable.

## Verification

Replace toggle-matrix assertions with lease lifecycle and old-identity-inert
tests. Run the desktop and tablet reconnection flows plus the phone no-attachment
flow under the production profile without an override. Keep real Docker
coverage for the Local Docker task-host case.
