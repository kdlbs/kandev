---
id: "12-compact-review"
title: "Instant switch, compact review, and two-version removal"
status: done
wave: 10
depends_on: ["11-conflicts-and-agents"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-007
  - REQ-INTEGRATIONS-PROMPT-SYNC-008
  - REQ-INTEGRATIONS-PROMPT-SYNC-009
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-007.1
  - AC-INTEGRATIONS-PROMPT-SYNC-007.2
  - AC-INTEGRATIONS-PROMPT-SYNC-008.2
  - AC-INTEGRATIONS-PROMPT-SYNC-008.5
  - AC-INTEGRATIONS-PROMPT-SYNC-008.7
  - AC-INTEGRATIONS-PROMPT-SYNC-008.8
  - AC-INTEGRATIONS-PROMPT-SYNC-009.1
  - AC-INTEGRATIONS-PROMPT-SYNC-009.3
  - AC-INTEGRATIONS-PROMPT-SYNC-009.5
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 12: Instant switch, compact review, and two-version removal

## Summary

Test-phase feedback on Task 11:

- Turning Local override off asked for a confirmation; switching is lossless,
  so both directions now apply at once.
- The conflict banner was easy to miss.
- Removal skipped overrides that had been edited, because Kandev did not
  store the repository version of an override.
- The review list repeated the explanation for every prompt.
- A prompt whose text already matched its file was listed as a conflict.

## In scope

- Backend: rename the kept column to `alternate_content` (the version not in
  use) and swap it on override and revert; record the repository version on
  overrides every sync; `DetachOverride` for removed files; identical text is
  adopted without a conflict; `TwoVersionPrompts` with `in_use`;
  `DELETE /config` takes `choices`.
- Web: instant switch with a toast naming the version; compact review rows
  with one legend; `PromptConflictNotice` at the top of the page; removal
  rows start on the version in use; copy in seven locales.

## Out of scope

Backend MCP changes.

## ASCII UI preview

See [plan.md](plan.md#ascii-ui-preview), UI-05 and UI-06.

## Verification

```bash
cd apps/backend
go test -trimpath ./internal/prompts/... ./internal/promptsync/ ./internal/backendapp/ ./internal/mcp/handlers/
KANDEV_TEST_POSTGRES_DSN=... go test -trimpath -count=1 ./internal/prompts/store/ -run Postgres
cd ../web
pnpm exec vitest run components/settings hooks/domains/settings lib/api/domains/prompt-sync-api.test.ts
pnpm e2e:run --host -- e2e/tests/settings/prompt-sync.spec.ts e2e/tests/settings/prompt-sync-conflicts.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome -- e2e/tests/settings/mobile-prompt-sync.spec.ts e2e/tests/settings/mobile-prompts-settings.spec.ts
```

## Results

- RED: the new store, service, promptsync, hook, and component tests failed
  before the change (missing methods, old confirmation, old copy).
- Go: prompts, promptsync, backendapp, and MCP handlers pass; the PostgreSQL
  prompt store tests (including the override/revert swap) pass against a
  throwaway cluster; sqlguard is clean.
- Web: the settings and prompt hook suites pass.
- E2E: desktop 4 passed; phone 4 passed (the phone conflict spec after fixing
  its label locator).
- `make fmt`, `make typecheck`, `make lint`, and `pnpm run i18n:check`: pass.
  `make test`: only the known host-environment failures.
- Simplification pass over the whole PR (requested in review): removed dead
  helpers and duplicate types (`classifySyncErr`, `defaultJitter`,
  `Service.Store`, `Reader.ListPath`, workflowsync `dirEntry`/`fileGetter`),
  reused `execChanged` and `hasPromptExtension`, merged the switch hook into
  `PromptOverrideSwitch`, replaced the review copy hooks with a `kind` key
  table and dropped the duplicate removal intro keys, shared the default
  prompt directory and interval check, and memoized the review hook. Fixed
  three issues it surfaced: an ownership-only sync reported `unchanged`; a
  stale conflict flag left the banner's Review doing nothing (it now
  refreshes the prompt list); and Enter could confirm the destructive removal
  step. Verification afterwards: fmt, typecheck, lint, and i18n pass; desktop
  (4), phone (4), and Workflow Sync GitLab (6) E2E pass; `make test` shows
  only the known host-environment failures.
