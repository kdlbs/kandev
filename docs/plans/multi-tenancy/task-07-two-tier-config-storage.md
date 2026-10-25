---
id: "07-two-tier-config-storage"
title: "Two-tier configuration storage and effective view"
status: todo
wave: 3
depends_on: ["04-service-layer-org-scoping"]
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 07: Two-Tier Configuration Storage and Effective View

## Acceptance

- `executors`, `executor_profiles`, `environments`, `agents`, `agent_profiles`,
  `editors`, `custom_prompts`, and `notification_providers` each gain
  `org_id TEXT NOT NULL DEFAULT ''` and `template_id TEXT NOT NULL DEFAULT ''`
  via idempotent migrations.
- One shared resolver produces the effective view for an org: own rows, plus
  every `org_id = ''` template whose `id` is not the `template_id` of an own
  row. There is exactly one implementation; the eight kinds do not each get
  their own copy.
- List responses carry `source: "instance" | "org"` and `editable`.
- Creating an org row from a template records `template_id` and shadows it
  immediately, with no duplicate appearing in the same response.
- Deleting an org row un-shadows its template in the next response.
- `agent_profiles.workspace_id` (used by Office) keeps working and composes with
  the new tier rather than being replaced by it.
- With the flag off, every one of these reads returns exactly today's rows.

## Verification

- `go test ./internal/agent/settings/... ./internal/agent/executor/... ./internal/task/repository/...`
- `go test ./internal/... -run 'TestEffectiveConfigView|TestTemplateShadowing'`
- `KANDEV_TEST_POSTGRES_DSN=... go test ./internal/agent/settings/store/...`

## Files Likely Touched

- `apps/backend/internal/agent/settings/store/sqlite.go`
- `apps/backend/internal/agent/executor/`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/notifications/`, `internal/prompts/`, `internal/editors/`
- New shared resolver, e.g. `apps/backend/internal/tenancy/effective.go`
- `apps/backend/internal/tenancy/registry.go`

## Inputs

- Spec: What (two-tier configuration), API surface (`source` / `editable`).
- Patterns: existing `deleted_at` soft-delete and `workspace_id` scoping in
  `internal/agent/settings/store/sqlite.go`.

## Output Contract

Report the single resolver's call sites (must be all eight kinds), the
shadow/un-shadow tests, flag-off byte-identity evidence, RED/GREEN commands,
and set this task plus its plan checkbox to done.
