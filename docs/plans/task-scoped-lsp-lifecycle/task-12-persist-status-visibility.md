---
id: "12-persist-status-visibility"
title: "Persist LSP status visibility"
status: completed
wave: 6
depends_on: ["11-public-documentation"]
plan: "plan.md"
spec: "../../specs/platform/requirements/lsp-file-intelligence.md"
---

# Task 12: Persist LSP Status Visibility

## Acceptance

- User settings persist a validated `lsp_status_hidden_languages` list; missing data means every
  registered language remains visible.
- Editor settings expose one translated Show in task status control per language and explain that
  visibility does not start, stop, enable, or disable its task server.
- Frontend hydration, dirty-state comparison, save payloads, and live store updates preserve the
  preference across reloads.

## TDD sequence

1. Add failing backend service and store tests for valid/invalid IDs, empty defaults, and round-trip.
2. Add failing frontend hydration, dirty-state, and language-card tests for visible-by-default and
   per-language toggling.
3. Thread the field through backend model/DTO/service/store and frontend HTTP/store/settings state.
4. Add English, pseudo, and Simplified Chinese labels and descriptions; run focused i18n checks.

## Verification

```bash
cd apps/backend && go test ./internal/user/...
cd apps/web && pnpm exec vitest run components/settings/lsp-language-cards.test.tsx components/settings/settings-dirty.test.ts lib/ssr/user-settings.test.ts
cd apps/web && pnpm run typecheck
```

## Output contract

Record RED/GREEN commands, exact files, persistence behavior, and any compatibility handling.

## Results

Completed 2026-08-06.

- RED: focused backend user-setting tests failed because the hidden-language field did not exist;
  focused frontend hydration, dirty-state, card, and view-model tests produced 16 expected failures.
- Added validated `lsp_status_hidden_languages` transport, event, boot-state, model, and SQLite JSON
  persistence. Missing legacy data normalizes to a non-nil empty list, keeping every registered
  language visible.
- Added one translated **Show in task status** switch per language under **Settings > General >
  Editors**. Save/hydration/live-store paths preserve the list, while the explanatory copy makes
  clear that visibility never changes task policy or process state.
- GREEN: `go test ./internal/user/... ./internal/backendapp` passed; the seven focused frontend
  files passed 73 tests; web typecheck passed.
