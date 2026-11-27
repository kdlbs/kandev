---
id: "02-show-command-diagnostics"
title: "Show retained errors and truthful recovery feedback"
status: done
wave: 2
depends_on:
  - "01-retain-command-diagnostics"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-PREP-003
  - REQ-AGENTS-MCP-PREP-004
acceptance_criteria:
  - AC-AGENTS-MCP-PREP-003.3
  - AC-AGENTS-MCP-PREP-004.1
  - AC-AGENTS-MCP-PREP-004.2
  - AC-AGENTS-MCP-PREP-004.3
  - AC-AGENTS-MCP-PREP-004.4
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
---

# Task 02: Show retained errors and truthful recovery feedback

## Summary

Consume the safe diagnostic consistently during live updates and reload. Show
an accurate preparation-stage explanation, safe command detail and session-busy
feedback in the existing desktop/phone preparation surface.

## In scope

- Optional typed `mcp_diagnostic` in WS/HTTP DTOs, `PrepareStepInfo`, live mappers
  and saved-result hydration; validate closed fields and bounded safe messages.
- Stage-aware localized explanation and plain-text diagnostic detail inside
  `AgentMcpPrepareActions`, ahead of existing recovery controls. Ignore malformed
  diagnostics while retaining generic failure/recovery behavior.
- Consume backend `error_code` through the existing `ApiError`/busy predicate;
  test the real handler-shaped response, not a fabricated client-only shape.
- English, pt-pt, zh-cn, zh-hk, zh-tw, ja, ko copy; generate Traditional Chinese
  with `i18n:zh-hant` and pseudo with the existing generator.
- Unit/component plus focused desktop/phone E2E and operator guidance in
  `docs/public/agents-and-profiles.md` (existing how-to subsection).

## Out of scope

Rendering legacy raw MCP `error`/`output`, provider tool calls, new OAuth flows,
new retry admission, standalone dialogs, new scroll owners or unrelated API
error-envelope changes.

## Acceptance

1. Component, WS and hydration tests initially fail for retained diagnostics,
   then prove live/completed/reloaded parity, malformed and legacy fallbacks,
   proper approval-stage explanation, unchanged authentication warning severity
   and stale-attempt rejection. Render safe detail as text, never HTML/Markdown.
2. Busy-envelope tests show the current-turn explanation without replacing the
   original preparation diagnostic. Retry command failure shows its safe cause;
   successful retry removes only that server's current failure detail.
3. Desktop/phone E2E prove retained details survive reload and recovery remains
   usable. Long messages wrap without horizontal document overflow; phone and
   coarse-pointer actions have measured targets at least 44px, including a
   narrow fine-pointer check. Record screenshots/rendered comparison to UI-01.

## ASCII UI preview

UI-01: Failed approval with illustrative retained runner error. See the
[complete preview](plan.md#ascii-ui-preview), AC-004.1/004.4.

```text
Desktop:
  Could not complete MCP server approval.
  Error details
  Operation: Approval    Stage: Waiting for command
  Exit status: 0
  exec: WaitDelay expired before I/O complete
  [Retry connection]
  Wait for the current turn to finish before retrying.

Phone:
  Could not complete MCP
  server approval.
  Error details
  Operation: Approval
  Stage: Waiting for command
  Exit status: 0
  exec: WaitDelay expired
  before I/O complete
  [      Retry connection      ]
  Wait for the current turn to
  finish before retrying.
```

Required: existing inline preparation region, hierarchy above, wrapping text,
stacked phone actions and one conversation scroll owner. Reuse the shipped
task conversation/`AgentMcpPrepareActions` composition and safe-area behavior.
No new navigation or overlay. Example cause and spacing are illustrative.

## Verification

Run from repository root. Install `apps/` dependencies once if missing; do not
overlap managed E2E runs. These commands build fresh test artifacts.

```bash
(cd apps/web && pnpm exec vitest run lib/ws/handlers/executor-prepare.test.ts lib/state/slices/session-runtime/prepare-result.test.ts lib/state/slices/session/set-task-sessions-prepare.test.ts lib/api/domains/session-mcp-actions.test.ts components/task/agent-mcp-prepare-actions.test.tsx components/session/prepare-progress.test.tsx components/session/prepare-progress-status.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/types/executor-payloads.ts lib/ws/handlers/executor-prepare.ts lib/state/slices/session-runtime/types.ts lib/state/slices/session-runtime/prepare-result.ts lib/api/domains/session-api.ts components/task/agent-mcp-prepare-actions.tsx components/session/prepare-progress.tsx)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/session/agent-mcp-preparation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-agent-mcp-preparation.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/lib/types/executor-payloads.ts`, `lib/state/slices/session-runtime/types.ts`,
  `prepare-result.ts`, `lib/ws/handlers/executor-prepare.ts` and their tests.
- `apps/web/lib/api/domains/session-api.ts`, `session-mcp-actions.test.ts`.
- `apps/web/components/task/agent-mcp-prepare-actions.tsx` and tests;
  `apps/web/components/session/prepare-progress.tsx` and tests as needed.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/task.json`.
- `apps/web/e2e/helpers/agent-mcp-preparation.ts` and desktop/mobile specs above.
- `docs/public/agents-and-profiles.md`; this package and paired spec statuses.

## Dependencies

Task 01's diagnostic schema and recovery error-code envelope. Execute in the
primary conversation, following mobile-parity and the existing E2E fixtures.

## Risks

Legacy text suppression is intentional. Accept only the new diagnostic field;
do not make old arbitrary error text visible. Error details are runtime data,
while authored labels/explanations require translation. Keep authentication
warnings, preparation-generation fencing and per-server recovery semantics.

## Parallelism

`sequential`

## Inputs

- REQ-AGENTS-MCP-PREP-004; AC-003.3 and the paired design's diagnostic/UI sections.
- Existing preparation WS/hydration tests, recovery component tests and
  `agent-mcp-preparation` desktop/phone E2E helpers.
- Mobile-parity mobile language/control-size guidance and public-doc authoring guide.

## Results

Implemented typed diagnostic validation and live/reload mapping while keeping
legacy raw MCP fields suppressed. The preparation UI now shows localized,
stage-aware safe details, retains the original cause during busy recovery, and
clears only the recovered server's diagnostic. Added desktop and phone E2E
coverage and documented the retained details in the public agent guide.

Validation passed on 2026-10-07:

- 8 focused frontend test files passed (42 tests); the stale-attempt diagnostic
  regression passed separately.
- `pnpm run typecheck` and targeted ESLint passed.
- `pnpm run i18n:zh-hant`, `pnpm run i18n:pseudo`, and `pnpm run i18n:check`
  passed.
- `pnpm e2e:run --project chromium tests/session/agent-mcp-preparation.spec.ts`
  passed (2 tests).
- `pnpm e2e:run --project mobile-chrome tests/session/mobile-agent-mcp-preparation.spec.ts`
  passed (2 tests).
- `pnpm --filter @kandev/web build:vite` passed from `apps/`.
- `node --test scripts/validate-public-docs.test.mjs` passed (62 tests);
  `node scripts/validate-public-docs.mjs` validated 47 published pages.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.

Review follow-up on 2026-10-07:

- A prompt started during native command verification now causes the retry to
  restore the prior preparation snapshot, publish the restored rows under the
  new attempt marker, and complete without reloading or replaying ACP. The
  lifecycle regression verifies the busy error, both target diagnostics, other
  server preservation, current-marker progress/completion, and persisted
  reload round-trip.
- Failed retry feedback now uses its normalized diagnostic to distinguish
  approval from tool verification failures. Authentication-required feedback
  and controls remain unchanged; tests cover approval, `list_tools`, and
  authentication-required responses.
- `(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'Test.*(Cursor.*MCP|Cursor.*Mcp|MCPDiagnostic)' -count=1)` passed.
- `(cd apps/web && pnpm exec vitest run components/task/agent-mcp-prepare-actions.test.tsx lib/api/domains/session-mcp-actions.test.ts)` passed (13 tests).
- Updated public guidance to distinguish an early busy check from a prompt
  that starts while native commands are already running.
- Cleanup-error separators now belong to the translated label in all supported
  locales. The component test verifies the rendered label and message spacing.
- Desktop E2E assertions verify the legacy raw error/output sentinels remain
  hidden for both MCP rows before and after reload. The phone E2E renders the
  cleanup detail and checks that the label remains readable in the existing
  mobile preparation surface.
- The focused Vitest run passed (14 tests), `i18n:check` passed, and targeted
  ESLint passed. The fresh Vite build and focused desktop and phone E2E runs
  passed (2 tests each).
