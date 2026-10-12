---
created: 2026-10-07
status: implemented
requirements:
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002
system_design:
  - ../../specs/platform/system-design/session-subscription-recovery.md
legacy_specs: []
---

# Implementation Plan: Silent Transcript Refresh

## Overview

After an agent turn, the web client reconciles the transcript in the background. When the
conversation revision check finds a gap, it also runs a full history recovery. Both showed
"Loading conversation..." above a transcript that was already on screen, so the whole page
jumped. This plan makes those background reconciles silent while keeping failures visible.

## Work orders

- [x] [Task 01: Silent background transcript refresh](task-01-silent-background-refresh.md)

## Verification

- `cd apps/web && pnpm exec vitest run hooks/domains/session`
- `cd apps/web && pnpm e2e:run tests/chat/turn-end-history-refresh.spec.ts`
- `cd apps/backend && go test ./cmd/mock-agent/`
