---
id: task-01-profile-fields
title: Add fallback_model and auto_fallback to agent profiles
status: done
wave: 1
depends_on: []
plan: docs/plans/no-silent-model-fallback/plan.md
spec: docs/specs/agents/requirements/no-silent-model-fallback.md
---

# Task 1 — Profile fields: `fallback_model` + `auto_fallback`

## Change

Add two fields to `agent_profiles` and thread them end-to-end:

- `fallback_model TEXT NOT NULL DEFAULT ''` — optional single ACP model ID.
- `auto_fallback INTEGER NOT NULL DEFAULT 0` — explicit opt-in to legacy
  automatic fallback.

Files:

- `apps/backend/internal/agent/settings/store/sqlite.go` — migration
  `r.migrate.Apply("agent_profiles.fallback_model", ...)` and
  `agent_profiles.auto_fallback` following the existing pattern (see
  `agent_profiles.command_prefix`); scan + insert paths for the new
  columns.
- `apps/backend/internal/agent/settings/models/models.go` — `AgentProfile`
  gains `FallbackModel string` + `AutoFallback bool` with JSON/db tags
  mirroring `Model`.
- `apps/backend/internal/agent/settings/dto/dto.go` — `AgentProfileDTO`
  gains `fallback_model,omitempty` + `auto_fallback`; the create/update
  request DTOs accept them.
- `apps/backend/internal/agent/settings/controller/profile_crud.go` —
  `toProfileDTO` / create / update request→model mapping.
- `apps/web/lib/types/agent-profile.ts` — `AgentProfile` +
  `AgentProfilePayload` gain `fallbackModel` / `autoFallback`.
- `apps/web/lib/api/domains/agent-profile-normalize.ts` —
  `normalizeAgentProfile` / `toAgentProfilePayload` round-trip.

Runtime precedence rule: when `auto_fallback` is true, `fallback_model` is
ignored. Saving both is allowed (no cross-field rejection).

## Acceptance

1. A profile created via `POST /api/v1/agents/:id/profiles` with
   `fallback_model` and `auto_fallback` round-trips through
   `GET /api/v1/agents` (DTO includes both fields).
2. `PATCH /api/v1/agent-profiles/:id` updates both fields.
3. Store test: scan/insert preserves both fields; migration applies to an
   existing DB (existing rows get `''` / `0`).
4. Frontend normalize test: wire ↔ state round-trip.

## Verification

```sh
make -C apps/backend test ./internal/agent/settings/...
cd apps/web && pnpm vitest run lib/api/domains/agent-profile-normalize.test.ts
cd apps/web && pnpm run typecheck
```

## Risks

- Migration ordering: add the two `Apply` calls near the other
  `agent_profiles.*` migrations; existing rows default to strict mode.
- The settings store uses manual scan structs — verify all SELECT column
  lists are updated.
