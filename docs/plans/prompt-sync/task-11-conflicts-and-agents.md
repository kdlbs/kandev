---
id: "11-conflicts-and-agents"
title: "Name conflicts, lossless switching, and agent guidance"
status: done
wave: 9
depends_on: ["10-override-switch"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-001
  - REQ-INTEGRATIONS-PROMPT-SYNC-003
  - REQ-INTEGRATIONS-PROMPT-SYNC-007
  - REQ-INTEGRATIONS-PROMPT-SYNC-008
  - REQ-INTEGRATIONS-PROMPT-SYNC-009
  - REQ-INTEGRATIONS-PROMPT-SYNC-010
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-001.3
  - AC-INTEGRATIONS-PROMPT-SYNC-003.3
  - AC-INTEGRATIONS-PROMPT-SYNC-007.1
  - AC-INTEGRATIONS-PROMPT-SYNC-007.2
  - AC-INTEGRATIONS-PROMPT-SYNC-008.1
  - AC-INTEGRATIONS-PROMPT-SYNC-008.2
  - AC-INTEGRATIONS-PROMPT-SYNC-008.3
  - AC-INTEGRATIONS-PROMPT-SYNC-008.4
  - AC-INTEGRATIONS-PROMPT-SYNC-008.5
  - AC-INTEGRATIONS-PROMPT-SYNC-008.6
  - AC-INTEGRATIONS-PROMPT-SYNC-008.7
  - AC-INTEGRATIONS-PROMPT-SYNC-009.1
  - AC-INTEGRATIONS-PROMPT-SYNC-009.2
  - AC-INTEGRATIONS-PROMPT-SYNC-009.3
  - AC-INTEGRATIONS-PROMPT-SYNC-009.4
  - AC-INTEGRATIONS-PROMPT-SYNC-009.5
  - AC-INTEGRATIONS-PROMPT-SYNC-010.1
  - AC-INTEGRATIONS-PROMPT-SYNC-010.2
  - AC-INTEGRATIONS-PROMPT-SYNC-010.3
  - AC-INTEGRATIONS-PROMPT-SYNC-010.4
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 11: Name conflicts, lossless switching, and agent guidance

## Summary

Test-phase feedback on Task 10:

- The Local override switch moved between row states.
- A repository file whose name matched an existing prompt was skipped with a
  warning. That made an override nobody chose, with no way to resolve it.
- Turning an override off discarded the local text.
- Agents needed clear rules for synced prompts.

## In scope

- Backend: `custom_prompts.local_content` and `conflict`; guarded ownership
  transitions; conflict-aware `ApplySyncedPrompts` with `Resolutions`;
  `Check`, `SaveAndSync`, and `SyncWithResolutions` in `promptsync`;
  `POST /api/v1/prompt-sync/check`; `POST /config` saves then syncs; agent
  guidance error and MCP `conflict` and `edit_guidance`; E2E reset keeps
  built-in prompts; `GET /kept-versions` and `DELETE /config` with
  `restore_local`.
- Web: switch-first trailing controls with the confirmation below the
  header; check-then-review save flow; removal review for kept local
  versions; conflict banner and review dialog;
  Conflict badge; app-wide conflict indicator; copy in seven locales.

## Out of scope

Telling agents apart from people on the HTTP API (see the requirement's out
of scope).

## ASCII UI preview

See [plan.md](plan.md#ascii-ui-preview), UI-05 and UI-06.

## Verification

```bash
cd apps/backend
go test -trimpath ./internal/prompts/... ./internal/promptsync/ ./internal/mcp/handlers/ ./internal/mcp/server/ ./internal/backendapp/
KANDEV_TEST_POSTGRES_DSN=... go test -trimpath -count=1 ./internal/prompts/store/ -run Postgres
go run ./cmd/sqlguard ./internal
cd ../web
pnpm exec vitest run components/settings components/app-sidebar components/navigation hooks/domains/settings lib/api/domains/prompt-sync-api.test.ts
pnpm e2e:run --host -- e2e/tests/settings/prompt-sync.spec.ts e2e/tests/settings/prompt-sync-conflicts.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome -- e2e/tests/settings/mobile-prompt-sync.spec.ts e2e/tests/settings/mobile-prompts-settings.spec.ts
```

## Results

- RED: the store, service, promptsync, MCP, and hook tests failed to build or
  failed on the old skip-and-warn and discard behavior before the change.
- Go: prompts, promptsync, MCP handlers and server, and backendapp pass; the
  PostgreSQL prompt store tests pass against a throwaway cluster; sqlguard is
  clean.
- Web unit: the settings, sidebar, navigation, and prompt hook suites pass.
- E2E: desktop 4 passed (including the two conflict specs); phone 4 passed.
- Removal review (AC-INTEGRATIONS-PROMPT-SYNC-009.5), added after a test-round
  question: the store, promptsync, hook, and dialog tests failed first; the
  PostgreSQL release test passes; the desktop and phone conflict specs now end
  by removing Prompt Sync with Restore my version, and pass (4 and 4).
