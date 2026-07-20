---
spec: docs/specs/ui/acp-model-configuration-summary.md
created: 2026-07-15
status: implemented
---

# Implementation Plan: ACP Model Configuration Summary

## Overview

Preserve provider descriptions, persist a write-once task-session config baseline, carry it with session model updates, and let task chat opt into a compact changed-values label. The shared selector keeps its current all-values default so profile settings remain unchanged.

## Backend

### Preserve ACP descriptions

Likely files:

- `apps/backend/internal/agentctl/types/streams/agent.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/meta_convert.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/meta_convert_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/session_models_test.go`
- `apps/backend/internal/agent/hostutility/types.go`
- `apps/backend/internal/agent/settings/dto/dto.go`
- `apps/backend/internal/utility/dto/dto.go`

Add optional description fields to normalized option/value types and copy them from both typed ACP structures and `_meta` fallback payloads. Ensure settings/utility discovery and live session events preserve the same optional metadata.

### Persist and publish the initial baseline

Likely files:

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/models/models_test.go`
- `apps/backend/internal/task/service/service_turns.go`
- `apps/backend/internal/task/service/service_turns_test.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/backendapp/helpers.go`

Add dedicated task-session metadata for the initial ACP option values with write-once persistence. Capture the first settled effective option set, load an existing baseline after restart, and include the baseline in `session.models_updated`. Persist provider snapshots as live runtime state and explicit user selections as separate overrides applied last during restoration, so delayed provider events cannot clobber user intent.

The implementation must avoid overwriting a baseline during repeated initial events or concurrent restart/recovery. Existing sessions without a baseline establish it on their first settled update.

## Frontend

Likely files:

- `apps/web/lib/types/backend.ts` or its generated/source payload definition
- `apps/web/lib/state/slices/session-runtime/types.ts`
- `apps/web/lib/ws/handlers/session-models.ts`
- `apps/web/lib/ws/handlers/session-models.test.ts`
- `apps/web/components/model-config-selector.tsx`
- `apps/web/components/model-config-selector.test.tsx`
- `apps/web/components/task/model-selector.tsx`
- `apps/web/components/task/model-selector.test.ts`

Extend dynamic option/value types with optional descriptions and store the persisted initial values delivered by the backend. Add an explicit task-only summary mode or label resolver that always renders the model plus all changed non-model value names in ACP order. Leave the shared selector's existing default label unchanged for profile and utility settings.

Render provider descriptions in the open model/config lists. Add an accessible hover/focus summary for the compact task trigger; touch users receive the same information by opening the selector. Constrain and truncate the toolbar trigger without changing the popover's responsive width.

## Tests

- Go tests prove description conversion for typed and metadata ACP paths.
- Go service/orchestrator tests prove baseline write-once behavior, restart loading, legacy baseline creation, and WebSocket publication.
- TypeScript unit tests prove all-values behavior remains the shared default and task mode shows only changed values in provider order, including return-to-baseline and new/removed options.
- WebSocket handler tests prove descriptions and baseline values reach the store.
- Component tests prove descriptions appear when opening config subselectors and are available through keyboard focus.

## E2E and Mobile

Likely files:

- Existing task chat model-selector E2E spec, if present
- `apps/web/e2e/tests/settings/agent-profile-acp.spec.ts`
- `apps/web/e2e/tests/chat/model-selector-error.spec.ts`
- `apps/web/e2e/tests/chat/mobile-model-selector.spec.ts`

Verify task chat compact rendering and open descriptions on desktop, touch access on mobile, restart persistence through the backend fixture where practical, and unchanged closed-label behavior in agent-profile settings.

## Waves

Wave 1:

- [x] [task-01-backend-contract-and-baseline](task-01-backend-contract-and-baseline.md)

Wave 2:

- [x] [task-02-task-selector-ux](task-02-task-selector-ux.md)

Wave 3:

- [x] [task-03-e2e-and-verification](task-03-e2e-and-verification.md)

## Verification

Targeted:

```bash
cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp/... ./internal/task/... ./internal/orchestrator/...
cd apps && pnpm --filter @kandev/web test -- --run components/model-config-selector.test.tsx components/task/model-selector.test.ts lib/ws/handlers/session-models.test.ts
cd apps/web && pnpm run typecheck
```

E2E after rebuilding production assets:

```bash
cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-profile-acp.spec.ts tests/chat/model-selector-error.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-model-selector.spec.ts
```

Final repository verification:

```bash
make fmt
node apps/web/scripts/generate-release-notes.mjs
node apps/web/scripts/generate-changelog.mjs
make typecheck
make test
make lint
```

## Risks

- Startup emits multiple config states while profile/runtime options are applied; baseline capture must occur only after the intended effective initial state settles.
- Agent-driven dependent option changes are intentionally displayed as changes from the baseline even when the user did not directly select them.
- Description support must cover typed ACP and legacy metadata conversion without changing behavior when descriptions are absent.
