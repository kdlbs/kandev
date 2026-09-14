---
id: 02-create-integration
title: Integrate task creation
status: done
wave: 2
depends_on:
  - 01-reference-editor
plan: plan.md
requirements:
  - REQ-UI-PROMPT-ALIAS-002
acceptance_criteria:
  - AC-UI-PROMPT-ALIAS-002.1
  - AC-UI-PROMPT-ALIAS-002.2
  - AC-UI-PROMPT-ALIAS-002.3
  - AC-UI-PROMPT-ALIAS-002.4
  - AC-UI-PROMPT-ALIAS-002.5
  - AC-UI-PROMPT-ALIAS-002.6
  - AC-UI-PROMPT-ALIAS-002.7
system_design:
  - ../../specs/ui/system-design/prompt-alias-rendering.md
---

# Task 02: Integrate task creation

## Summary

Enable reference chips only for new tasks, including canvas presets.
Preserve shared form services and prove the complete desktop and phone interactions.

## In scope

- Explicit create-only mode through dialog/form props and the unchanged public form handle.
- Attachments, enhancement, voice/plugin insertion, drafts, launch preview, and retry.
- Desktop/phone E2E migration and public documentation for the changed creation interaction.

## Out of scope

- Changes to New Agent or task-edit insertion behavior, backend expansion, or canvas launch defaults.

## Acceptance

- General and canvas creation serialize aliases through the existing task payload.
- Desktop/phone tests prove UI-01 to UI-03, including interaction and measured geometry.
- Shared form tests and New Agent regressions pass with unchanged non-create semantics.

## ASCII UI preview

UI-01/02 excerpt from the [full preview](plan.md#ascii-ui-preview), criteria `.1` through `.7`:

```text
Desktop                         Phone
Goal [@prompt] [x]               Goal
[Attach] [Enhance]               [@prompt] [x]
Options                         [Attach] [Enhance]
[Cancel] [Start task]            ... scrolling form ...
                                [Cancel] [Start task]
                                Tap chip -> preview drawer
```

UI-03 keeps the same draft after creation failure. Missing prompts stay ordinary text.
Measure drawer containment, internal scrolling, footer access, and touch hit areas.
Capture and inspect screenshots against the full previews.

## Verification

Run from the repository root, sequentially. The mobile chips spec is a new deliverable.
Inventory `task-description-input` consumers before edits, and add exact commands
for every additional changed test before marking this work order done.

```bash
(cd apps/web && pnpm exec vitest run components/task-create-dialog-selectors.test.tsx components/task-create-dialog-form-body.test.tsx components/task-create-dialog.test.tsx components/task/new-session-form-prompt.test.tsx components/canvas/canvas-task-create-launcher.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
make -C apps/backend build
make -C apps/backend e2e-plugin-package
(cd apps/web && pnpm run build:e2e)
(cd apps/web && pnpm e2e:raw --project=chromium --retries=0 tests/task/task-create-prompt-autocomplete.spec.ts tests/task/task-create-prompt-autocomplete-qa.spec.ts tests/canvas/plugin-canvas.spec.ts tests/session/new-session-dialog.spec.ts)
(cd apps/web && pnpm e2e:raw --project=mobile-chrome --retries=0 tests/task/mobile-task-create-prompt-chips.spec.ts tests/canvas/mobile-plugin-canvas.spec.ts tests/session/mobile-new-session-dialog.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task-create-dialog.tsx`, `task-create-dialog-form-body.tsx`, and prop types/builders.
- `apps/web/components/task-create-dialog-selectors.tsx` and its tests.
- `apps/web/hooks/use-task-create-prompt-mention.ts` or the create-only replacement adapter.
- `apps/web/e2e/pages/kanban-page.ts` and the named E2E specs.
- `docs/public/developer-tools.md`, `docs/public/canvases.md`.
- This package's results and companion-plan links, without replacing historical results.

## Dependencies

Task 01. Run its tests again if integration changes its implementation.

## Risks

The editor ref must expose synchronous text during plugin insert-then-submit.
Preserve open-cycle draft identity and do not treat editor blur as dialog dismissal.
Avoid contenteditable HTML comparisons as proof of persisted description text.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/prompt-alias-rendering.md), requirement 002.
- [Design](../../specs/ui/system-design/prompt-alias-rendering.md#task-creation-editor).
- Shared form tests, canvas E2E, and `mobile-prompt-mention-composer.spec.ts` viewport helpers.

## Results

Implemented create-only editor integration, preserved textarea behavior for
session and edit flows, migrated affected description assertions, added public
documentation, and added the phone prompt-chip workflow.

Verification on 2026-09-12:

- `cd apps/web && pnpm exec vitest run components/task-create-dialog-selectors.test.tsx components/task-create-dialog-form-body.test.tsx components/task-create-dialog.test.tsx components/task/new-session-form-prompt.test.tsx components/canvas/canvas-task-create-launcher.test.tsx`: 5 files and 57 tests passed.
- `cd apps/web && pnpm run typecheck`: passed.
- `cd apps/web && pnpm run lint`: passed with zero warnings.
- `cd apps/web && pnpm run i18n:check`: passed, with 7,994 referenced keys and all five catalogs complete.
- `cd apps/web && pnpm run i18n:ratchet`: passed; 1 added and 10 modified files clean, guard allowlist intact.
- `make -C apps/backend build`: passed.
- `make -C apps/backend e2e-plugin-package`: passed.
- `cd apps/web && pnpm run build:e2e`: passed.
- `cd apps/web && pnpm e2e:raw --project=chromium --retries=0 tests/task/task-create-prompt-autocomplete.spec.ts tests/task/task-create-prompt-autocomplete-qa.spec.ts tests/canvas/plugin-canvas.spec.ts tests/session/new-session-dialog.spec.ts`: 26 tests passed.
- `cd apps/web && pnpm e2e:raw --project=mobile-chrome --retries=0 tests/task/mobile-task-create-prompt-chips.spec.ts tests/canvas/mobile-plugin-canvas.spec.ts tests/session/mobile-new-session-dialog.spec.ts`: 7 tests passed.
- `cd apps/web && pnpm e2e:raw --project=chromium --retries=0 tests/task/enhance-prompt.spec.ts tests/plugins/composer-actions.spec.ts`: 12 tests passed.
- `cd apps/web && pnpm e2e:raw --project=mobile-chrome --retries=0 tests/task/mobile-task-create-escape.spec.ts tests/plugins/mobile-composer-actions.spec.ts`: 5 tests passed.
- `cd apps/web && pnpm run e2e:sleep-ratchet`: passed; no added sleeps.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 46 published pages validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
