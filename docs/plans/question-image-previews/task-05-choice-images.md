---
id: "05-choice-images"
title: "Add optional images to clarification choices"
status: done
wave: 4
depends_on:
  - "03-question-image-interface"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGES-005
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGES-005.1
  - AC-TASKS-QUESTION-IMAGES-005.2
  - AC-TASKS-QUESTION-IMAGES-005.3
  - AC-TASKS-QUESTION-IMAGES-005.4
  - AC-TASKS-QUESTION-IMAGES-005.5
system_design:
  - ../../specs/tasks/system-design/question-image-previews.md
---

# Task 05: Add optional images to clarification choices

## Summary

Allow visual alternatives in the existing blocking question flow. Preserve selection,
drafts, source-session access, retry identity, and image-free clients.
The user explicitly requested planning followed by implementation in the same turn.

## In scope

- Add optional `options[].images`, with zero or one strict image reference.
- Bound each question to four total references and each bundle to four distinct IDs.
- Extend operator schema, admission, ownership detection, persistence, reconstruction,
  equality, and authorized lookup. Keep both parent entry points image-free.
- Render independent selection and inspection controls. Include choice-only requests
  in draft retention and provide labelled, lazy historical previews.
- Update public authoring reference and operator instructions.

## Out of scope

Multi-image choice galleries, new answer fields, native provider image schemas,
new executor transports, image uploads, and fixing Task 04's environment limitations.

## Acceptance

1. Validation rejects malformed/oversized/conflicting choice references before waiting.
   Exact retries, pending listings, and history retain references. Access stays scoped.
2. Inspecting any choice never selects, advances, or submits. Keyboard and touch
   preserve the draft and return focus. Selecting still returns the original option ID.
3. Phone and desktop retain compact previews, local error/retry states, lazy history,
   and image-free compatibility. Updated docs describe all limits and examples.

## UI preview

Use [UI-05](plan.md#ui-05-visual-choices-on-desktop-and-phone), AC-005.2 and .5.
Shared order: prompt, choice label/selection button, choice thumbnail/Inspect, next
choice. Each labelled choice group has sibling action controls. Phone uses the same
one-column question scroller and full-height inspector. Desktop uses its bounded
inspector. Loading/error content replaces only the preview. History has a labelled
Show images control per illustrated choice. Host rich output replaces ASCII sketches.

## Verification

Write failing tests before production changes. Use JSON fixtures to prove old code
drops or ignores choice references before adding the field. Cover one choice image,
shared question/choice IDs, aggregate and per-choice limits, nulls, invalid paths,
conflicts, parent rejection, ownership denial, wrong question lookup, persisted
reconstruction, unchanged legacy hashes, and changed choice-image hashes.

Use the managed E2E runner sequentially. Capture real desktop and phone UI.
The new choice scenario includes question+choice images, choice-only images,
reload, draft/position preservation across responsive layouts, inspection without
selection, single-question selection, missing images, selected IDs, and lazy history.

```bash
(cd apps/backend && go test ./internal/clarification/... ./internal/mcp/server)
(cd apps/backend && go test ./internal/mcp/handlers -run 'TestChoiceImages|TestQuestionImagesAdmission' -count=1 -timeout=3m)
(cd apps/backend && go test ./internal/backendapp -run 'TestClarification.*Image' -count=1 -timeout=3m)
(cd apps/web && pnpm exec vitest run components/task/chat/clarification-choice-images.test.tsx components/task/chat/question-image-viewer.test.tsx components/task/chat/question-image-draft-context.test.tsx components/task/chat/clarification-panel-section.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/clarification-input-overlay.tsx components/task/chat/clarification-overlay-parts.tsx components/task/chat/clarification-choice-images.tsx components/task/chat/clarification-choice-images.test.tsx components/task/chat/messages/clarification-request-message.tsx lib/types/http-agents.ts e2e/tests/chat/question-image-helpers.ts e2e/tests/chat/choice-image-helpers.ts e2e/tests/chat/question-image-previews.spec.ts e2e/tests/chat/mobile-question-image-previews.spec.ts)
pnpm --dir apps/web e2e:run --host --project chromium tests/chat/question-image-previews.spec.ts tests/chat/clarification-choice-only.spec.ts
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-question-image-previews.spec.ts
(cd apps/web && pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the existing browser-library environment and isolated port offset if required.
Run changed-Go lint with the repository helper and Prettier on changed web files.

## Files likely touched

- `apps/backend/internal/clarification/{protocol,store.go,bundle_question.go,image_handlers.go}`
- `apps/backend/internal/mcp/{server,handlers}`
- `apps/backend/internal/backendapp/adapters.go` and focused metadata tests
- `apps/web/lib/types/http-agents.ts`
- `apps/web/components/task/chat/{clarification-input-overlay,clarification-overlay-parts,clarification-choice-images}.tsx`
- `apps/web/components/task/chat/messages/clarification-request-message.tsx`
- `apps/web/e2e/tests/chat/*question-image*` and `choice-image-helpers.ts`
- `docs/public/{agent-communication,use-kandev}.md`

## Dependencies

Existing question-image protocol, authorized serving endpoint, and viewer.

## Risks

- A missed option projection can silently lose references after reload.
- Nested controls or bubbling viewer keys can accidentally commit a choice.
- Choice-only requests must trigger the same source ownership and draft guards.
- Real Docker/SSH certification remains unverified in this environment (Task 04).

## Parallelism

`sequential`

## Inputs

REQ-TASKS-QUESTION-IMAGES-005 and the design's Choice image extension section.
Existing question-image tests and clarification choice-only regression tests.

## Results

Implemented without a new file-serving route, migration, dependency, or answer field.
The operator schema, strict decoder, placement limits, source ownership detection,
option persistence, pending projection, retry identity, parent rejection, and saved
reference lookup include choice images. UI controls are siblings. Choice-only drafts
survive responsive remounts. Historical choice images remain labelled and lazy.

RED evidence: the new JSON contract tests failed because choice images were ignored,
invalid references were accepted, and image-bearing detection returned false. The
component regression failed because the choice had no inspection control.

Validation after implementation:

- `go test ./internal/clarification/... ./internal/mcp/server`: passed. Final log:
  `/tmp/choice-images-backend-final.log`.
- Focused `internal/mcp/handlers` choice-image and question-image admission tests:
  passed, including foreign task rejection and empty/nonempty parent image fields.
- Focused `internal/backendapp` image metadata and source-session tests: passed.
- Four Vitest files passed, 21 tests. These include the new sibling-control test,
  the existing viewer guard, responsive draft context, and clarification panel.
- Managed desktop browser run: four cases passed. Log:
  `/tmp/question-images-checks/kandev-run.e2e.lUbYxzbH.log`.
- Managed phone browser run: two cases passed. Log:
  `/tmp/question-images-checks/kandev-run.e2e.VMxvv9Jo.log`.
- A first phone run used a five-second wait after reload and failed while images
  were still loading. Its replacement uses the existing 30-second image-read
  allowance. The next full phone run passed. This required no production change.
- The final fixture gives image IDs values different from option IDs. The desktop
  recheck passed and returned the option ID. Log:
  `/tmp/question-images-checks/kandev-run.e2e.92RNssje.log`.
  The phone recheck also passed with the same distinction. Log:
  `/tmp/question-images-checks/kandev-run.e2e.sYsXIdsp.log`.
- Typecheck passed with `NODE_OPTIONS=--max-old-space-size=8192`.
  Log: `/tmp/choice-images-typecheck.log`.
- Targeted ESLint, changed-package Go lint, Prettier, and `git diff --check` passed.
  Go lint used `PATH=/tmp/ssh3735-tools:$PATH bash scripts/lint-go-changed` with
  the five changed owning packages. Log: `/tmp/choice-images-go-lint.log`.
- Localization passed. No new translated strings were needed.
  Log: `/tmp/choice-images-i18n.log`.
- Public documentation validation passed for 48 pages. Specification catalog and
  lint passed for 384 decisions and 1618 specifications. All 36 spec-linter tests passed.
- Local `.github/scripts/pr-docs.cjs::validateCoverage` covered all five work orders
  across 76 changed files, with zero reference errors. The local runner also checked
  relative links. No status was published.

The managed browser commands used `/tmp/question-images-browser-env.sh`,
`E2E_PORT_OFFSET=23`, and `CAPTURE_PR_ASSETS=true`. Desktop and phone captures were
visually inspected against UI-05. Choices retain their label above the preview,
and the next choice remains a separate selection action. The panel scrolls when
the alternatives exceed its height.

Task 04 remains incomplete. This extension does not certify real Docker/SSH
connectivity or close the earlier broad MCP handler-suite timeout.
