---
spec: docs/specs/ui/acp-model-configuration-summary.md
decision: docs/decisions/2026-07-18-turn-configuration-snapshots.md
created: 2026-07-18
status: implemented
---

# Implementation Plan: Turn Configuration Attribution

## Overview

Capture the effective session configuration once when a turn is created, persist it in existing turn metadata, and render message attribution from that immutable snapshot. Reuse the task-session ACP baseline to show only changed options while retaining provider names and order.

## Waves

Wave 1:

- [x] [task-01-backend-turn-snapshot](task-01-backend-turn-snapshot.md) — done

Wave 2:

- [x] [task-02-frontend-turn-attribution](task-02-frontend-turn-attribution.md) — done

Wave 3:

- [x] [task-03-integration-and-verification](task-03-integration-and-verification.md) — done

Tasks are sequential because the frontend contract depends on the backend metadata shape, and integration verification depends on both. Runtime policy keeps all work in the current workspace without delegated agents.

## Backend Design

Likely files:

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/service/service_turns.go`
- `apps/backend/internal/task/service/service_turns_test.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`

Add a typed, JSON-tolerant turn configuration snapshot. At `StartTurn`, resolve the effective model/mode/options from the profile snapshot, persisted provider runtime state, explicit overrides, and the persisted ACP selector state. Persist selected option IDs, raw values, display names, provider order, and the captured baseline under one turn metadata key. Existing prompt-usage updates must merge metadata without replacing the snapshot.

No database migration or new API is planned: turn metadata already persists in SQLite/Postgres and is included in boot/API/WebSocket turn payloads.

## Frontend Design

Likely files:

- `apps/web/components/task/chat/messages/message-actions.tsx`
- `apps/web/components/task/chat/messages/message-session-config.ts`
- `apps/web/components/task/chat/messages/message-session-config.test.ts`
- `apps/web/components/task/chat/messages/chat-message.test.tsx`

Move parsing and formatting into a pure helper. Resolve model attribution from message metadata, provider-refined turn metadata, then the captured snapshot. Render only snapshot options whose raw value differs from the captured baseline, preserving provider order and `Name: Value` labels. Do not render mode in the compact row. Legacy turns without a snapshot may show a model but must not fall back to current session options.

This changes data selection and text content inside the existing truncated metadata row; it does not introduce a new control, layout, or viewport-specific interaction. Focused unit/component coverage satisfies mobile parity, with existing truncation retained.

## Verification

Targeted red/green cycles:

```bash
rtk go test -run 'TestStartTurn.*Config|TestPersistPromptMetadata.*Config' ./internal/task/service ./internal/orchestrator
rtk pnpm --filter @kandev/web test -- --run components/task/chat/messages/message-session-config.test.ts components/task/chat/messages/chat-message.test.tsx
```

Run Go commands from `apps/backend` and pnpm commands from `apps/`.

Final verification:

```bash
rtk make fmt
rtk node apps/web/scripts/generate-release-notes.mjs
rtk node apps/web/scripts/generate-changelog.mjs
rtk make typecheck
rtk make test
rtk make lint
```

## Risks

- A turn can be created before a provider has emitted a settled ACP selector snapshot. The turn must preserve the best configuration known at creation and must not be mutated later.
- Prompt-usage updates currently add model and usage to turn metadata. Their read-modify-write path must retain the snapshot.
- Legacy messages currently borrow mutable session options. Removing that fallback intentionally reduces detail to avoid false historical attribution.
