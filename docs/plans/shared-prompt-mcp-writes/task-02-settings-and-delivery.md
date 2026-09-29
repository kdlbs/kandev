---
id: TASK-SHARED-PROMPT-WRITES-02
title: Prompt operator control and live delivery
status: completed
wave: 2
depends_on: [TASK-SHARED-PROMPT-WRITES-01]
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-SHARED-PROMPT-WRITES-001
acceptance_criteria:
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.5
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.7
system_design:
  - ../../specs/integrations/system-design/shared-prompt-writes.md
---

# Prompt operator control and live delivery

## Scope and files

Prompt change event and gateway wiring; frontend DTO/API/cache refresh; existing prompt edit form, locale catalogs; desktop/mobile E2E; public docs.

## Acceptance

- Implement the referenced criteria with targeted Red-Green-Refactor evidence.
- Preserve existing human editing, prompt expansion, and authentication behavior.
- Record all required check results after the final changes.

## Exclusions

MCP deletion and unrelated settings or workflow redesign.

## Dependencies and parallelism

TASK-SHARED-PROMPT-WRITES-01. Sequential; no subagents.

## Risks

Permission bypass, concurrent revocation, migration data loss, and stale readback.

## ASCII UI preview

UI-01: Settings > Prompts, editing a custom prompt (desktop and phone).

```text
@review-policy                         [Edit] [Delete]
[Prompt name                                      ]
[Prompt content editor                            ]
Allow agent edits                            [off]
Agents can change this prompt wherever it is used.
[Cancel]
                      [Discard] [Save changes]
```

The new permission row shares the existing page scroll owner and save bar. On
phones the label wraps and its switch row has a 44px touch target. No overlay or
navigation change. Built-in edit forms omit the permission control. Labels use
i18n. Geometry in the drawing is illustrative; order and shared save semantics
satisfy AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.5 and .7.

## Verification

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/settings/use-custom-prompts.test.tsx components/settings/prompts-settings.test.tsx lib/ws/handlers/prompts.test.ts lib/ws/use-websocket.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --project chromium tests/settings/shared-prompt-mcp-writes.spec.ts tests/settings/prompts-settings.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-prompts-settings.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Results

Passed 20 web unit tests across four files, web type checking, changed-file
ESLint, and complete i18n validation. Seven Chromium prompt tests and two
mobile-Chrome tests passed through the managed host runner. Captured desktop and
phone screenshots from synthetic fixtures. The phone test verifies a 44px switch
target, saving/reopening permission, and no horizontal overflow; the desktop test
verifies live MCP writes, protected prompts, and preservation of a local draft.

Red evidence: missing prompt-change routing/cache invalidation, reconnect
invalidation, and permission UI failed before implementation. All now pass.
Public docs (47 pages), validator tests (62), specification/catalog checks, and
PR documentation coverage preflight passed. CI, automated review disposition,
and merge remain external delivery gates tracked in the PR.
