---
created: 2026-10-22
status: done
requirements:
  - REQ-UI-SETTINGS-MANUAL-SAVE-001
system_design:
  - ../../specs/ui/system-design/settings-manual-save.md
legacy_specs: []
---

# Implementation Plan: Jira Settings Refresh Ordering

## Overview

Preserve the acknowledged Jira settings baseline when an older mounted read
finishes later. One sequential work order first proves the race through the real
providers/coordinator/API, then adds local request ownership and verifies it.
The user explicitly authorized package creation followed immediately by TDD in
this same session. Stop after normal hooks and a clean commit for independent
parent validation; push/PR need a later separate authorization.

## Scope

Own only Jira configuration reads and acknowledged Save/removal admission.
Reuse active AC-UI-SETTINGS-MANUAL-SAVE-001.4 and existing saved-baseline/Reset
behavior. Preserve canonical normalization, credentials, later edits, initial
load, actual 90-second health polling, later fresh reads/null/errors, and truthful
write feedback. Exclude other integrations, Query migration, global coordination,
backend/schema/auth changes, initial-GET recovery expansion, and sibling work.

## Root cause and technical approach

`useJiraConfigRefresh` unconditionally publishes completed GETs after Save has
accepted a newer POST response. Old config or 204 null replaces that baseline,
marks the saved form dirty, and makes Reset stale. Supplied actual-provider
source diagnostic: two expected behavior failures and one old-error control pass.
Use one bounded mounted request owner shared by initial/OAuth/background reads
and successful mutation admission. Latest read sequence plus write revision
fences superseded publications; lifetime cleanup fences disposed owners.
Successful writes remain admitted, with existing submitted-draft comparison.

## ASCII UI preview

UI-01: Existing Jira settings, desktop and phone. State semantics only.
Entry: Settings > Workspace > Integrations > Jira. Existing content scrolls;
the shared safe-area-aware Save surface stays fixed when dirty.

```text
[Jira integration]  [Enabled]
[Site] [Authentication] [Project: SAVED]
[Saved credential] [Auth health]
Before: old GET arrives -> [Reset] [Save changes] (incorrectly dirty)
After:  old GET arrives -> clean, no dirty surface
Later edit: [Project: NEWER] -> [Reset] [Save changes]
Reset: [Project: SAVED] -> clean
```

Phone uses the shipped dedicated workspace integration settings route and
existing touch Save/Reset controls. No layout, navigation, or hitbox changes.

## Tests

`jira-settings-refresh.test.tsx` protects AC-UI-SETTINGS-MANUAL-SAVE-001.4,
Reset, normalization, edits during Save, failure controls, overlapping reads,
removal, and workspace/lifetime isolation using real providers/coordinator/API
and deferred wire responses only. The canonical stale success/null regression
must fail causally before production changes. Existing settings/form tests stay.

## E2E tests

`jira-settings-refresh.spec.ts` (chromium) and
`mobile-jira-settings-refresh.spec.ts` (mobile-chrome) verify the actual timer,
Save, old read release, Reset, next Save, and reload against disposable mock-Jira
backends. Own disposable workspace/repository and explicit worktree executor;
check shared seed HEAD/full status without resetting it. Release held routes and
clean all owned data in finally. Native phone emulation is not a physical device.

## Work orders

- [x] [Task 01: Preserve saved Jira baseline](task-01-preserve-saved-baseline.md)

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/jira/jira-settings-refresh.test.tsx components/jira/jira-settings.test.tsx components/jira/jira-form-state.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/jira/jira-settings.tsx components/jira/use-jira-config-publication.ts components/jira/jira-settings-refresh.test.tsx e2e/tests/integrations/*jira-settings-refresh*)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/integrations/jira-settings-refresh.spec.ts -- --retries=0
GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/integrations/mobile-jira-settings-refresh.spec.ts -- --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Verification results

Implemented and locally validated. The work order records exact results and
limitations: permanent causal RED, 30 focused tests passing, final desktop and
Pixel 5 browser checks passing with no retries, full typecheck, strict changed-file
lint, localization, specification and public-doc gates. Frozen dependency install
completed before package commands. Normal hooks and the clean committed parent
validation checkpoint remain the final local delivery gate. Push/PR require a
separate later grant.

## Risks

Read lifetime must handle StrictMode and workspace replacement; write sequence
must not accidentally suppress successful writes. Avoid editing shared seed or
operator runtime/data. Main/open PR heads must be refreshed before final handoff.
