---
id: "01-expand-preview-images"
title: "Expand preview images"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-IMAGE-PREVIEW-EXPANSION-001
acceptance_criteria:
  - AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.1
  - AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.2
  - AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.3
  - AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.4
  - AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.5
system_design:
  - ../../specs/agents/system-design/image-preview-expansion.md
---

# Task 01: Expand preview images

## Summary

Connect ready rich-output image previews to the attachment dialog. Prove image
identity, input support, dismissal, and phone geometry with targeted tests.

## In scope

- Write failing behavior tests before production changes using `/tdd` and `/e2e`.
- Reuse the image dialog and preserve source, disclosure, and target lifetime.
- Add necessary image-viewer touch sizing and focused shared-viewer coverage.
- Update the existing public file-preview guidance and record validation results.

## Out of scope

Other file viewers, new image formats, zoom controls, global dialog redesign,
new schemas, transports, settings, and broad cleanup.

## Acceptance

1. Both rich-output versions open the correct image without another read or file-open callback.
2. Click, tap, Enter, and Space open the dialog. Dismissal returns focus, and target replacement removes the dialog.
3. Phone image and close targets meet 44px minimums. The dialog stays contained, and existing disclosure and attachment behavior pass regression checks.

## ASCII UI preview

UI-01 and UI-02, excerpt from the [full preview](plan.md#ascii-ui-preview).
Covers AC .1-.5.

```text
Desktop: [shot.png] [Hide preview] [Open file]
         [ click image -> fitted image dialog ]
Phone:   [shot.png]
         [Hide preview] [Open file]
         [ tap image -> fitted image dialog ]
Dialog:  [larger image] [Close]
         Close -> original image trigger
```

Keep the transcript as scroll owner outside the modal. Lock background scrolling
while open. Touch targets and viewport containment are required. Spacing is illustrative.

## Verification

Run from repository root. Install dependencies once if this worktree has no
workspace install. Execute browser commands sequentially with managed fresh builds.

```sh
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx components/task/chat/messages/kandev/rich-output/rich-output-view-state.test.tsx hooks/domains/session/use-workspace-file-preview.test.ts components/task/chat/image-preview-dialog.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/messages/kandev/rich-output/file-preview-block.tsx components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx components/task/chat/messages/kandev/rich-output/rich-output-view-state.test.tsx components/task/chat/image-preview-dialog.tsx components/task/chat/image-preview-dialog.test.tsx e2e/tests/chat/rich-output.spec.ts e2e/tests/chat/mobile-rich-output.spec.ts e2e/tests/chat/rich-output-image-helpers.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/rich-output.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-rich-output.spec.ts tests/chat/mobile-message-attachment-layout.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

The dialog test file is new work in this task. Test the real shared viewer,
including its noninteractive branch and existing attachment defaults. Record
actual outcomes and phone screenshot evidence. Run the documentation coverage
preflight in `.github/scripts/pr-docs.cjs` against the final changed-file set.

## Files likely touched

- `apps/web/components/task/chat/messages/kandev/rich-output/file-preview-block.tsx`
- `apps/web/components/task/chat/image-preview-dialog.tsx`
- `apps/web/components/task/chat/image-preview-dialog.test.tsx` (new)
- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx`
- `apps/web/e2e/tests/chat/rich-output.spec.ts`
- `apps/web/e2e/tests/chat/mobile-rich-output.spec.ts`
- `apps/web/e2e/tests/chat/rich-output-image-helpers.ts` (new focused geometry and image-fixture helpers)
- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-view-state.test.tsx` (accessible trigger selectors)
- `docs/public/automation-and-mcp.md`
- Paired specifications and this package for final status and results

## Dependencies

None. Execute this work order in the primary session after an explicit
implementation request.

## Risks

The dialog portal outlives layout wrappers unless component identity removes
it. Shared viewer changes require attachment-default coverage. Do not weaken
existing assertions when updating decorative thumbnail selectors.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/image-preview-expansion.md)
- [System design](../../specs/agents/system-design/image-preview-expansion.md)
- [Existing disclosure plan](../image-preview-defaults/plan.md)
- Existing renderer target tests, `seedRichOutputImage`, and attachment dialog callers

## Results

Completed on 2026-10-20. All acceptance criteria passed.

- Dependency bootstrap: `(cd apps && pnpm install --frozen-lockfile)` passed without lockfile changes.
- TDD RED: the renderer suite failed five cases on the missing image trigger. Its other 22 cases passed.
- Geometry RED: the real phone browser test measured a 178px dialog image against a 285px inline preview. The explicit dialog width corrected that failure.
- The exact four-file Vitest command above passed 37 tests after the final production edit. Log: `/tmp/image-expansion-vitest-final.log`.
- The exact typecheck command above passed, including catalog drift validation. Log: `/tmp/image-expansion-typecheck-final.log`. The default Node heap failed first. The final run used 6 GiB.
- The exact focused ESLint command above passed. Log: `/tmp/image-expansion-lint-final.log`.
- `(cd apps/web && pnpm run i18n:check)` passed all catalog, translation, copy, plural, and punctuation checks. Log: `/tmp/image-expansion-i18n.log`.
- Final mobile managed run passed six tests, including the existing attachment-layout regression. Log: `/tmp/kandev-run.e2e.pjWi1cYD.log`.
- Final desktop managed run passed six tests, including actual enlargement, keyboard dismissal/focus, and narrow fine-pointer controls. Log: `/tmp/kandev-run.e2e.XIZ0XP6l.log`.
- `node --test scripts/validate-public-docs.test.mjs` passed 63 tests. `node scripts/validate-public-docs.mjs` validated 48 published pages.
- Specification catalog, specification lint, documentation coverage, and `git diff --check` passed, including final lifecycle updates.

The browser commands required local environment setup. The isolated backend
initially inherited a non-loopback bind from machine configuration. Set
`KANDEV_SERVER_HOST=127.0.0.1` for these runs. Chromium also required installed
Nix library directories in `LD_LIBRARY_PATH`. A temporary launcher at
`/tmp/image-expansion-env.py` supplied these values without changing repository
configuration or the user's running instance.

Exact final browser commands from repository root:

```sh
python3 /tmp/image-expansion-env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/chat/mobile-rich-output.spec.ts tests/chat/mobile-message-attachment-layout.spec.ts
python3 /tmp/image-expansion-env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/chat/rich-output.spec.ts
```

The mobile command rebuilt backend, frontend, and plugin fixtures after the
final production edit. The desktop command reused those unchanged artifacts.
No checks used a live developer instance. No commit or push was requested.


Final phone capture was repeated once with a dedicated output directory because
the desktop runner clears its default output directory. The focused scenario
passed (log: `/tmp/kandev-run.e2e.fuzoXhly.log`). Command:

```sh
python3 /tmp/image-expansion-env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-rich-output.spec.ts --grep 'mobile image previews expand' --output e2e/test-results-image-expansion-phone
```

Inspected `apps/web/e2e/test-results-image-expansion-phone/chat-mobile-rich-output-mo-d1831-hable-hide-and-open-actions-mobile-chrome/expanded-image-phone.png`.
The image is larger than its inline preview, retains its aspect ratio, and fits
the viewport. The close target remains visible. The result matches UI-02.
No actionable UI-polish findings remain in this changed interaction.

REQ-AGENTS-IMAGE-PREVIEW-EXPANSION-001 and AC .1-.5 are covered. The requirement
is active, the design is current, and the plan is implemented. No known
implementation limitation remains within the defined scope.
