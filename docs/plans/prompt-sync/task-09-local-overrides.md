---
id: "09-local-overrides"
title: "Local overrides"
status: done
wave: 7
depends_on: ["07-view-synced-prompts", "08-review-remediation"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-007
  - REQ-INTEGRATIONS-PROMPT-SYNC-004
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-007.1
  - AC-INTEGRATIONS-PROMPT-SYNC-007.2
  - AC-INTEGRATIONS-PROMPT-SYNC-007.3
  - AC-INTEGRATIONS-PROMPT-SYNC-007.4
  - AC-INTEGRATIONS-PROMPT-SYNC-007.5
  - AC-INTEGRATIONS-PROMPT-SYNC-007.6
  - AC-INTEGRATIONS-PROMPT-SYNC-004.5
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 09: Local overrides

## Summary

Let an installation override a synced prompt locally and revert to the
repository version (follow-up from the issue discussion on #5391).

## In scope

- Prompt store/service: `override` source, guarded `SetPromptSource`,
  `OverrideSyncedPrompt`, `RevertOverride`, write guards on `source <> 'sync'`,
  release of overrides, `Overridden` in the apply result.
- `promptsync`: `Override` and `Revert` (revert re-syncs), HTTP routes.
- Web: Override action on synced rows, Overridden badge and Revert on
  overrides, status card list, API client, copy in seven locales.
- E2E: override, edit, sync leaves it, revert restores the repository text.

## Out of scope

Converting a clashing manual prompt into an override.

## ASCII UI preview

See [plan.md](plan.md#ascii-ui-preview), UI-05.

## Acceptance

AC-INTEGRATIONS-PROMPT-SYNC-007.1 through 007.6 and the updated 004.5.

## Verification

```bash
cd apps/backend
go test -trimpath -race ./internal/prompts/... ./internal/promptsync ./internal/mcp/handlers -count=1
cd ../web
pnpm exec vitest run hooks/domains/settings/use-prompt-sync components/settings
pnpm e2e:run --host -- tests/settings/prompt-sync.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome -- tests/settings/mobile-prompt-sync.spec.ts
```

## Results

- RED: store, service, promptsync, hook, API, and component tests failed
  before the change (missing `override` source, actions, and UI).
- `go test -trimpath -race ./internal/prompts/... ./internal/promptsync -count=1`: pass.
- `go test -trimpath ./internal/mcp/handlers -run 'Prompt|Override|Sync' -count=1`: pass.
- `pnpm exec vitest run components/settings/prompts-settings components/settings/prompt-sync-section.test.tsx hooks/domains/settings lib/api/domains/prompt-sync-api.test.ts lib/settings`: pass.
- `pnpm e2e:run --host -- tests/settings/prompt-sync.spec.ts`: 2 passed.
- `pnpm e2e:run --host --no-build --project mobile-chrome -- tests/settings/mobile-prompt-sync.spec.ts tests/settings/mobile-prompts-settings.spec.ts`: 3 passed.
- `pnpm run lint`, `pnpm run typecheck`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet`: pass.
