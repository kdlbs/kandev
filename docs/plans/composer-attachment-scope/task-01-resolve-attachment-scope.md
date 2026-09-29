---
id: "01-resolve-attachment-scope"
title: "Resolve attachment scope and verify delivery"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
acceptance_criteria:
  - AC-TASKS-PROMPT-ATTACHMENTS-001.12
  - AC-TASKS-PROMPT-ATTACHMENTS-001.13
  - AC-TASKS-PROMPT-ATTACHMENTS-001.14
  - AC-TASKS-PROMPT-ATTACHMENTS-001.15
  - AC-TASKS-PROMPT-ATTACHMENTS-001.16
  - AC-TASKS-PROMPT-ATTACHMENTS-001.17
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
---

# Task 01: Resolve attachment scope and verify delivery

## Summary

Resolve the exact task workspace across editable session composers. Prevent
missing scope from bypassing uploads or plan implementation, and prove
file-backed delivery in browsers.

## In scope

- Shared scope hook, exact Office identity, authoritative cold-route reads that
  take precedence over changing cache state, failure/retry, request
  deduplication, and stale-response protection.
- Upload, message, and plan-implementation guards; localized feedback; restored
  unready drafts; full task/session identity reset; and late-upload cleanup.
- Targeted unit tests and the complete surface matrix in the plan.
- Public attachment documentation and all six locale catalogs for new copy.

## Out of scope

Office comment attachment migration, read-only view editing, backend changes,
new delivery modes, dead-code removal, and new runtime flags.

## Acceptance

1. Real pasted files upload in the owning workspace on a cold Office route with
   an unrelated active workspace selected. Quick Chat and workflow scope
   resolution remain covered by focused shared-composer tests.
2. Every selected file needs a ready descriptor before message send or plan
   implementation. Scope/upload errors retain the draft and expose recovery.
   Restored files cannot fall back to inline submission, and late results cannot
   cross task/session identity.
3. Desktop and phone checks prove upload, submission, transcript display, and
   recovery. Read-only views and ordinary text paste remain unchanged.

## ASCII UI preview

UI-01, excerpt from [the full preview](plan.md#ascii-ui-preview):

```text
[image.png: uploading] [Remove]
[Message text                 ]
[Attach]       [Send: disabled]
Error: [Scope unavailable] [Retry]
Ready: [image.png] [Remove] [Send]
Plan: [Implement disabled while any attachment is incomplete]
```

Phone chips wrap above the full-width prompt. Controls retain 44px touch targets.
Use the existing Chat scroll owner and safe-area handling. Desktop keeps its
normal control density. Maps to AC-TASKS-PROMPT-ATTACHMENTS-001.12 through .17.

## Verification

Run from the repository root. Run the first two new regressions before the fix.
The managed E2E runner rebuilds artifacts and owns process cleanup.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/composer-workspace.test.ts hooks/domains/task/use-composer-workspace.test.ts components/task/chat/use-chat-input-state.test.ts components/task/chat/clipboard-attachments.test.ts components/task/chat/chat-input-toolbar.test.tsx hooks/domains/kanban/use-plan-actions.test.ts hooks/domains/kanban/use-plan-actions.runner.test.ts lib/local-storage.test.ts)
(cd apps && pnpm --filter @kandev/desktop e2e)
node --test apps/desktop/e2e/desktop-launch-smoke.test.mjs
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/composer-workspace.ts hooks/domains/task/use-composer-workspace.ts components/task/chat/chat-input-area.tsx components/task/passthrough-chat-composer.tsx components/task/chat/use-chat-input-state.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/composer-attachment-scope.spec.ts tests/chat/attachment-paste-warning.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-composer-attachment-scope.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/chat/composer-workspace.ts` and its existing test.
- New `apps/web/hooks/domains/task/use-composer-workspace.ts` and `.test.ts`.
- `apps/web/components/task/chat/chat-input-area.tsx`.
- `apps/web/components/task/passthrough-chat-composer.tsx`.
- `apps/web/components/task/chat/use-chat-input-state.ts` and its existing test.
- `apps/web/hooks/domains/kanban/use-plan-actions.ts` and its existing tests.
- `apps/web/components/task/chat/chat-input-toolbar*.tsx` and toolbar tests.
- `apps/web/lib/local-storage.ts` and its existing tests.
- `apps/desktop/e2e/desktop-launch-smoke.mjs` and its existing test.
- `apps/web/components/task/chat/clipboard-attachments.test.ts` only if coverage needs extension.
- New `apps/web/e2e/tests/chat/composer-attachment-scope.spec.ts`.
- New `apps/web/e2e/tests/chat/mobile-composer-attachment-scope.spec.ts`.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/task.json`.
- `docs/public/tasks-and-workflows.md`.

## Dependencies

None. Execute sequentially in the primary session.

## Risks

Do not substitute the active workspace or reuse the error-context provider as
an attachment scope API. Use the task API through the shared hook. Tests must
cover cold fetches, mixed cache records, and task switches during pending work.
Do not remove the live simple comment composer or broaden read-only run views.

## Parallelism

`sequential`

## Inputs

- [Attachment requirements](../../specs/tasks/requirements/prompt-attachments.md), criteria .12 through .17.
- [Attachment design](../../specs/tasks/system-design/prompt-attachments.md#composer-workspace-resolution).
- [File-backed attachment ADR](../../decisions/2026-08-04-file-backed-prompt-attachments.md).
- Existing resolver, upload-state, and clipboard tests.
- Existing `attachment-paste-warning.spec.ts` and mobile attachment tests.
- `apps/web/AGENTS.md`, `/tdd`, `/e2e`, `/mobile-parity`, and `/docs-maintainer`.

## Results

Implemented the shared workspace resolver and hook, including Office task
identity, authoritative cold reads, retry, request deduplication, and stale
response isolation. File submission now waits for every selected browser file
to have a ready descriptor regardless of scope; uploads start when scope
resolves, and late uploads are deleted when their draft owner changes. Scope
failures retain the draft and show localized recovery. Shared and passthrough
composers use the hook. The public attachment guide and all six locale catalogs
were updated.

Validation passed: the final focused Vitest command passed 153 tests across 7
files. TypeScript, i18n checks, Vite production build, targeted ESLint without
warnings, Prettier, public-doc validation (62 tests and 47 pages), spec
validation, and spec lint passed. Deferred hook cases cover cached scope
loss/conflict, task A-to-null-to-A resolution and rejection/retry, both
file-decode/scope completion orders, stale decode discard, restored legacy
draft recovery, and explicit retry without an automatic retry loop. Plan
message dispatch, workflow advance, and desktop/tablet/phone toolbar guards are
covered. The desktop smoke unit suite passed 17 tests, and its native E2E passed
startup and two-window conflict recovery. The latest managed Chromium rerun
passed the Office cold-route attachment and unreadable-image fallback cases
(2 tests). The latest mobile-chrome rerun passed the Office real-file upload,
retry, and send case (1 test).
Browser checks use synthetic paste and do not prove native OS clipboard
behavior. This package adds no real-file E2E scenario for Quick Chat, general
run transcripts, or passthrough; those surfaces use the shared composer path.
