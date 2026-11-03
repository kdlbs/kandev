---
id: "03-question-image-interface"
title: "Render and inspect question images"
status: complete
wave: 3
depends_on:
  - "02-authorized-image-reads"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGES-001
  - REQ-TASKS-QUESTION-IMAGES-002
  - REQ-TASKS-QUESTION-IMAGES-003
  - REQ-TASKS-QUESTION-IMAGES-004
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGES-001.2
  - AC-TASKS-QUESTION-IMAGES-001.3
  - AC-TASKS-QUESTION-IMAGES-001.4
  - AC-TASKS-QUESTION-IMAGES-002.4
  - AC-TASKS-QUESTION-IMAGES-002.5
  - AC-TASKS-QUESTION-IMAGES-003.1
  - AC-TASKS-QUESTION-IMAGES-003.2
  - AC-TASKS-QUESTION-IMAGES-003.3
  - AC-TASKS-QUESTION-IMAGES-003.4
  - AC-TASKS-QUESTION-IMAGES-003.5
  - AC-TASKS-QUESTION-IMAGES-004.1
  - AC-TASKS-QUESTION-IMAGES-004.2
  - AC-TASKS-QUESTION-IMAGES-004.4
system_design:
  - ../../specs/tasks/system-design/question-image-previews.md
---

# Task 03: Render and inspect question images

## Summary

Complete the operator flow on desktop and phone. Show each image above its question
and provide inspection without changing selection, drafts, or submission behavior.

The user authorized a compact-thumbnail adjustment after reviewing captures.
Limit thumbnail height to 128px on desktop and 96px on phones. Keep the full image
and tap-to-inspect behavior. Treat the image and label as one accessible target.
The desktop and phone image E2E cases must show a short prompt on initial entry,
check rendered thumbnail bounds, and retain the existing inspection and answer checks.

## In scope

- Extend the web question type. Add a question-image API client and hook using
  identity-only routes, cancellation, stale-response fencing, and revocable blob URLs.
- Add a small image component to pending and historical questions. Reuse shared
  hosts so task chat, Quick Chat, Office, Inbox, and coordinator retain one behavior.
- Provide opt-in inspection with Fit and Actual size. Preserve existing image
  attachment consumers. Suspend question shortcuts while inspection is open.
- Add loading, failure, unsupported, and Retry states. Fetch only the active
  expanded question, with two concurrent reads maximum. Load history on expansion.
- Add all copy in seven locales. Use `i18n:zh-hant` for the Traditional Chinese pair.
  Add focused component/hook tests and desktop/phone E2E proof.

## Out of scope

Answer-state redesign, durable unsent drafts, uploads, annotations, rich-output
forms, generic image-viewer redesign, and global image caches.

## Acceptance

1. A three-question bundle shows correct images, retains independent answers,
   supports one final bundled submission, and behaves correctly without images.
2. Inspection and responsive changes preserve question position, focus, choices,
   and drafts. Viewer keys cannot trigger question or parent-dialog actions.
3. Phone layout, accessibility, localization, lazy loading, source-session
   ownership, errors, and stale-response handling pass focused rendered checks.

## UI preview

Use the [combined structured preview](plan.md#ui-preview). The host requires native
rich output instead of ASCII/SVG/HTML sketches. The following excerpts retain its labels.

| View | Required structure |
| --- | --- |
| UI-01: Desktop question | Existing header. Scrolling body: description, uncropped thumbnail, Inspect, prompt, choices, text. Existing navigation and completion actions. |
| UI-02: Phone question | One focused question. Image and answers remain in one vertical sequence. Full-height inspection has Close, a canvas, Fit, and Actual size. |
| UI-03: Loading/error | Reserved image area. Localized status or reason. Retry when useful. Prompt and answer controls remain usable. |
| UI-04: Desktop inspection | Bounded dialog. Description and Close. Fit/Actual size controls and one canvas scroller. Escape returns focus to Inspect. |

These views implement AC-003.1 through AC-003.5. The question panel remains the
normal phone entry. A dense screenshot justifies full-height inspection rather
than a short choice drawer. Use `100dvh`, safe areas, and contained scrolling.

Nearest shipped exemplars: `ClarificationPanelSection`, `ImagePreviewDialog`,
`mobile-clarification.spec.ts`, and the canonical mobile surface guide.
Use `useResponsiveBreakpoint` at 768px. Keep one shared answer state across modes.
Phone/coarse-pointer controls use 48px nominal size and measure at least 44px.
Keep desktop controls at the surrounding 28px density.

Apply the design's `better-ui` image outline, focus, radius, icon, hover, and
reduced-motion rules. Do not add expressive animation to question navigation.
Descriptions are plain agent content. Application copy uses translation keys.

## TDD and rendered cases

Add `use-question-images.test.ts` for active-only reads, two-read concurrency,
explicit retry, generation changes, aborted fetches, and blob cleanup. Assert
requests contain only saved identities. Test a session switch while a read is pending.

Add `question-images.test.tsx` and `question-image-viewer.test.tsx`. Cover uncropped
layout, alt/accessibility names, errors, unsupported runtime, repeated image association,
Escape isolation, focus restoration, and retained drafts. Test existing dialog
callers without the opt-in mode to prevent attachment regressions.

Create the desktop and phone E2E files named below. Seed actual workspace images
through existing fixture patterns. Use a deterministic mock-agent scenario for
three questions, including a shared image. Assert returned answers by question ID,
not carousel position. Ensure no response arrives merely from opening an image.

Measure phone hit targets and document overflow. Inspect light/dark screenshots,
long descriptions, full-page images, reduced motion, and keyboard focus. Include
767px fine-pointer and 768px boundary cases in the desktop file. Exercise phone
text entry without hiding completion controls. Recheck draft state after resizing.

## Verification

Run from the repository root. Install dependencies once in this worktree before
any pnpm command. The managed E2E runner rebuilds production assets.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-question-images.test.ts components/task/chat/question-images.test.tsx components/task/chat/question-image-viewer.test.tsx components/task/chat/clarification-input-overlay.test.tsx components/task/chat/clarification-panel-section.test.tsx components/task/chat/messages/clarification-request-message.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/question-image-previews.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-question-image-previews.spec.ts)
git diff --check
```

Run eslint and Prettier checks on concrete changed frontend files. Record the
exact commands in Results. If existing image-dialog tests are changed, include
that suite too. Use the normal E2E capture fixture for rendered evidence.

## Files likely touched

- `apps/web/lib/types/http-agents.ts`
- New `apps/web/lib/api/domains/question-image-api.ts`
- New `apps/web/hooks/domains/session/use-question-images.ts` and test
- New `apps/web/components/task/chat/question-images.tsx`, `question-image-viewer.tsx`, and tests
- `question-image-draft-context.tsx` and its tests; provider in `task-layout.tsx` for responsive remounts
- `apps/web/components/task/chat/clarification-input-overlay.tsx`
- `apps/web/components/task/chat/messages/clarification-request-message.tsx`
- `apps/web/components/task/chat/image-preview-dialog.tsx` only for compatible opt-in reuse
- `apps/web/src/locales/*/task.json` and generated locale outputs as required
- New `apps/web/e2e/tests/chat/question-image-previews.spec.ts` and `mobile-question-image-previews.spec.ts`
- `apps/web/e2e/tests/task/mobile-threads-composer-disclosure.spec.ts` for picker transition regression checks
- Mock-agent or script fixtures under `apps/web/e2e` and `apps/backend/cmd/mock-agent` as needed

## Dependencies

Tasks 01 and 02. Read scoped web/chat guidance, mobile-parity, better-ui, and e2e.
Read mock-agent guidance before modifying that fixture implementation.

## Risks

Quick Chat and clarification both handle Escape. Image-only state must not reset
answers. Mounted hidden UI can accidentally fetch files. Existing image-dialog
responsive width variants can override unprefixed phone sizing.

## Parallelism

Sequential. Shared question and image-viewer contracts require one integration pass.

## Inputs

Design sections Presentation and inspection, Loading and memory, and Failures.
Read existing clarification component tests and mobile E2E before implementing.

## Results

Implemented on 2026-10-24. The final focused UI run passed 97 tests in nine files.
It covers the image loader, binary API, viewer guard, transient drafts, overlay,
panel, history, answer group, and task layout owner:

```bash
cd apps/web
pnpm exec vitest run components/task/chat/question-image-draft-context.test.tsx components/task/chat/question-image-viewer.test.tsx hooks/domains/session/use-question-images.test.ts lib/api/domains/question-image-api.test.ts components/task/chat/clarification-input-overlay.test.tsx components/task/chat/clarification-panel-section.test.tsx components/task/chat/messages/clarification-request-message.test.tsx hooks/domains/session/use-clarification-group.test.ts components/task/task-layout-repository.test.tsx
NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

All commands passed. The default 4 GiB typecheck exhausted its heap; the 8 GiB run
passed. Localization covers seven languages and the generated pseudo-locale.
The generator also synchronized two existing pseudo-locale entries.

The desktop image suite passed two cases. It verifies one keyed bundle, retained
drafts, Escape/focus, missing files, lazy history, reload, backend restart, fresh
file bytes, tall-image scrolling, rejection, reduced motion, and 767px/768px
boundaries. The final phone suite passed 15 image, clarification, and rich-output
cases. Exact integration commands and logs are in Task 04.

A RED browser case found that responsive layout remounts discarded question drafts.
A task-owned, bounded memory context now retains image-question text, choices,
and position. It clears settled bundles and is discarded on task navigation or
reload. Image-free and late-answer state keep their existing lifecycle. A unit
regression also caught and corrected an unstable provider key during metadata
refresh. The durable design records this added implementation boundary.

Pure image markup is verified in browser integration cases. No separate
`question-images.test.tsx` was needed. Existing attachment dialogs are unchanged.
Light/phone and desktop captures were inspected. Dark captures use the existing
finite-animation wait before capture; assets remain ignored under `apps/web/.pr-assets`.

ESLint passed on all concrete changed TS/TSX files. The final provider check used:

```bash
pnpm exec eslint components/task/chat/question-image-draft-context.tsx components/task/chat/question-image-draft-context.test.tsx components/task/chat/clarification-input-overlay.tsx components/task/task-layout.tsx e2e/tests/chat/question-image-previews.spec.ts
```

Logs: `/tmp/question-images-final-web-tests.log`,
`/tmp/question-images-final-typecheck.log`, `/tmp/question-images-final-i18n.log`,
`/tmp/question-images-final-eslint.log`, and `/tmp/question-images-draft-lint.log`.

### Compact-thumbnail adjustment

The phone regression failed before the change because the prompt had no viewport
intersection. Log: `/tmp/question-images-checks/kandev-run.e2e.1lyrOC7E.log`.
The implemented bounds are 8rem on desktop and 6rem below 768px. The image and
Inspect label form one target, with a 48px minimum height and width on phones and
coarse pointers. The label no longer adds a separate control-height row.

After the production change, the phone case passed and both desktop cases passed.
These cases cover initial prompt visibility, thumbnail bounds, 767px/768px/1280px
layouts, touch target dimensions, draft preservation, inspection, and submission.
Actual desktop and phone captures show the image, prompt, and first choice before
scrolling. Additional choices and custom text remain in the existing scroll area.

Commands used the managed runner after sourcing `/tmp/question-images-browser-env.sh`,
with `E2E_PORT_OFFSET=23` and `CAPTURE_PR_ASSETS=true`:

```bash
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/chat/mobile-question-image-previews.spec.ts
pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/chat/question-image-previews.spec.ts
```

Logs: `/tmp/question-images-checks/kandev-run.e2e.5sAfIjhZ.log` (1 phone case)
and `/tmp/question-images-checks/kandev-run.e2e.LjN1vXd2.log` (2 desktop cases).
The Vite build, targeted ESLint, Prettier, and specification checks passed.
This adjustment does not close Task 04's existing remote-runtime and broad-suite gaps.


PR review correction: question and choice image hooks now share a two-read
scheduler for each bundle, question, and owner. Queued requests abort on cleanup;
idle groups are removed. A deferred multi-hook regression failed with three
simultaneous requests before the fix. It now verifies the shared limit, queued
cancellation, and progress of a surviving choice. The existing owner-change test
drains its deferred mocks during cleanup.

`pnpm exec vitest run hooks/domains/session/use-question-images.test.ts` passed
(three tests). The helper is
`apps/web/hooks/domains/session/question-image-scheduler.ts`.


### PR CI remediation

The task-keyed draft provider remounted the desktop layout during task navigation.
This discarded pending Changes attention and failed both inactive-task focus cases
in `tests/layout/changes-panel-focus.spec.ts`. The provider now replaces its cache
on task changes and preserves child component identity. A new unit case failed
before the fix and passed after it; it checks task isolation and a single layout
mount across task switches.

The mobile thread composer case reopened the picker before the previous close
transition settled. It now waits for the first sheet to close and its finite
animations to finish before opening the picker again, as it already did for the
second selection. Assertions, timeouts, and retry policy remain unchanged.


Focused validation after the CI fixes:

- Draft context unit tests: four passed, including task isolation without a layout remount.
- Original inactive-task/saved-layout focus E2E cases: two passed without changing either test.
- Full mobile composer disclosure spec plus both mobile image cases: seven passed,
  with retries disabled. Log: `/tmp/question-images-checks/kandev-run.e2e.mzFKW8VP.log`.
- Web typecheck and changed-file lint passed.

- Desktop image previews: three passed with retries disabled, including choice
  inspection, reload, and draft retention across the phone breakpoint.
  Log: `/tmp/question-images-checks/kandev-run.e2e.KWL3Dxky.log`.
