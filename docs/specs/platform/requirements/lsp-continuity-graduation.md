---
status: draft
system: platform
created: 2026-09-25
owners:
  - kandev
---

# LSP Continuity Graduation Requirements

## Overview

Platform owns the task-host language-server lease. This requirement makes the
existing [browser-independent continuity](lsp-file-intelligence.md) behavior
the normal LSP lifecycle without changing when a language server starts.

## Requirements

### REQ-PLATFORM-LSP-CONTINUITY-GRADUATION-001: Default lease lifecycle

**Intent:** A supported editor retains and reattaches to its task-host language
server without an installation-wide release toggle.

#### Acceptance criteria

- **AC-PLATFORM-LSP-CONTINUITY-GRADUATION-001.1:** In the first default-on
  stable release, browser-independent continuity shall be active by default in
  all shipped profiles while an explicit environment value or installation
  override may still disable it.
- **AC-PLATFORM-LSP-CONTINUITY-GRADUATION-001.2:** In a later stable release,
  continuity shall remain active whenever a supported language server is
  started, regardless of the former `features.lspBrowserContinuity` key,
  `KANDEV_FEATURES_LSP_BROWSER_CONTINUITY`, or a stored false override.
- **AC-PLATFORM-LSP-CONTINUITY-GRADUATION-001.3:** Graduation shall not turn on
  LSP auto-start, auto-install, phone LSP, or an unsupported executor. Explicit
  Stop, editor-idle release, detached expiry, capacity eviction, server exit,
  task-host shutdown, and backend shutdown shall retain their existing effect.
- **AC-PLATFORM-LSP-CONTINUITY-GRADUATION-001.4:** Desktop and coarse-pointer
  tablet editors shall retain the existing reconnecting, ready, and failure
  states. The phone file viewer shall not start or attach to an LSP lease.
- **AC-PLATFORM-LSP-CONTINUITY-GRADUATION-001.5:** The retirement release shall
  omit the old key from the Feature Toggles page and `/api/v1/features`, and
  permanently reserve its key and environment variable against reuse.

## Out of scope

- Extending the one-hour detached deadline or the LSP capacity limit.
- Persisting a live LSP process through backend or task-host restart.
