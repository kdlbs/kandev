---
id: "01-initial-attachment-previews"
title: "Show initial attachment previews"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
acceptance_criteria:
  - AC-TASKS-PROMPT-ATTACHMENTS-001.8
  - AC-TASKS-PROMPT-ATTACHMENTS-001.9
  - AC-TASKS-PROMPT-ATTACHMENTS-001.10
  - AC-TASKS-PROMPT-ATTACHMENTS-001.11
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
---

# Task 01: Show initial attachment previews

## Summary

Carry a validated session preview from task creation to the preparation transcript.
Reuse the existing attachment controls and replace the preview when stored history arrives.

## In scope

- Internal preparation option and persisted session preview before publication.
- Session metadata normalization, synthetic message composition, and attachment types.
- Focused backend, hook, desktop, and phone regression coverage from the plan.

## Out of scope

Agent delivery changes, new storage tables or APIs, queues, and legacy inline preview backfill.

## Acceptance

1. Backend preparation publishes the submitted preview before workspace work,
   preserves it on fresh reads and failure, and preserves claim/session isolation.
2. Chat displays it with existing controls, handles empty text and malformed
   metadata, and replaces it without duplicates under all existing history guards.
3. Desktop and phone E2E prove opening attachments during preparation, reload,
   success replacement, and failure behavior without overflowing the phone viewport.

## ASCII UI preview

UI-01: Task Chat during workspace preparation. See the [full state preview](plan.md#ascii-ui-preview).

```text
+--------------------------+
| [image 1] [image 2]       |
| [notes.txt]              |
| Submitted text           |
+--------------------------+
Preparing environment...
  Create worktree
```

Shared phone/desktop ordering; phone content wraps in the existing Chat scroll
region. Image tap opens the existing viewer. Failure retains this row above
the existing error; stored history replaces it. Covers `.8` through `.11`.

## Verification

Run from the repository root. Bootstrap dependencies once if this worktree lacks them.
Write and run the backend/hook regression before production edits. The backend
test must fail because preparation has no snapshot; the hook test must fail
because the initial row has no attachment metadata, not because a selector is missing.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/task/handlers -run 'Test.*InitialPreview' -count=1)
(cd apps/backend && go test ./internal/task/models -run 'TestInitialPromptPreview' -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'Test.*(InitialPromptPreview|LaunchSession|InitialPrompt)' -count=1)
(cd apps/web && pnpm exec vitest run hooks/use-processed-messages-fallback.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/task/preparation-attachments.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-preparation-attachments.spec.ts)
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run changed-file ESLint and Go formatting checks. Include any additional test
files changed by implementation in this command list before marking complete.
Managed E2E builds current assets; do not reuse a stale build. Check discovered
test counts and compare the captured phone view with UI-01.

## Files likely touched

- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers_initial_preview_test.go` (new)
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/initial_prompt_preview_test.go` (new)
- `apps/backend/internal/task/models/initial_prompt_preview.go` (new typed metadata helper if needed)
- `apps/web/components/task/chat/use-chat-panel-state.ts`
- `apps/web/hooks/use-processed-messages.ts`
- `apps/web/hooks/use-processed-messages-fallback.test.ts`
- `apps/web/components/task/chat/messages/chat-message.tsx`
- `apps/web/e2e/tests/task/preparation-attachments.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-preparation-attachments.spec.ts` (new)
- A shared preparation fixture helper under `apps/web/e2e/helpers/`.

## Dependencies

None. Use the existing attachment claim service, session metadata persistence,
history initialization state, and attachment renderer.

## Risks

Do not pass display snapshots as a second launch prompt. Do not attach task-wide
files to sibling sessions or change fallback history eligibility. Preserve
passthrough prepare upgrades and unrelated session metadata.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/prompt-attachments.md), `.8` through `.11`.
- [Design](../../specs/tasks/system-design/prompt-attachments.md#initial-preview-during-workspace-preparation).
- Existing `use-processed-messages-fallback.test.ts` and `mixed-attachment-layout.ts` patterns.
- Root and scoped AGENTS.md, `/tdd`, `/e2e`, and `/mobile-parity`.

## Results

Implementation adds a display-only session snapshot before preparation publication.
The frontend validates descriptors and reuses the existing image controls and file
labels. History guards, session isolation, and stored-message precedence remain intact.
Synthetic rows cannot be favorited. Public task documentation explains the preview.

- Behavioral RED: the hook omitted attachment metadata; the task-create handler
  omitted the preview from its preparation request. Both regressions passed after wiring.
- Hook regression suite: 12 tests passed, including late hydration, attachment-only
  prompts, malformed descriptors, history guards, and stored-message replacement.
- TypeScript checking passed. Changed-file ESLint reported no errors.
- Model descriptor test passed. Public docs tests passed (62 tests); validation
  passed for 46 pages. Specification lint and whitespace checks passed.
- Desktop and phone E2E scenarios were added. The initial managed browser run
  stopped during backend build because the shared disk filled; no browser assertion ran.
- Backend verification retries encountered disappearing shared cache and temporary
  build files. A retry uses a private cache and build directory.

Final targeted backend checks passed for models, handlers, and orchestrator.
The orchestrator command includes existing initial-prompt and launch-session tests.
The preview-write failure test first demonstrated a stranded CREATED session, then
passed with the existing launch-failure handler. The sibling-session fixture now
sets its workspace ready before creating the sibling, as the product requires.
The final hook run passed all 12 tests with `--silent`, after a console-forwarding
teardown failure on a prior run. TypeScript and changed-hook lint passed again.

Final browser verification passed with fresh host-only artifacts. Build commands:

```bash
(cd apps/backend && make build-agentctl build-kandev build-mock-agent e2e-plugin-package GOFLAGS=-p=1)
(cd apps/web && pnpm build:e2e)
(cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/task/preparation-attachments.spec.ts --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-preparation-attachments.spec.ts --retries=0)
```

- Desktop: 2 passed. Phone: 1 passed. Retries were disabled; cleanup also passed.
- Screenshots were inspected against UI-01. Both layouts retain the image controls,
  file label, and prompt. The phone view has no document horizontal overflow.
- Browser coverage uses a controlled setup barrier and tests image opening,
  reload, unavailable image content, and replacement by one saved user message.
  The second desktop scenario checks non-fatal setup warnings. Terminal failure
  persistence is covered at the backend boundary.
- Initial browser failures were fixture issues: cleanup needed explicit consent
  to discard attachment-staging files; fetch responses expose `ok` as a property;
  setup-script failures produce `completed_with_warnings`, not a terminal error.
- The hook and handler provided behavioral RED evidence before implementation.
  Browser checks were added after the minimal fix to verify the rendered data path.
- Final changed-file ESLint: no errors; two test-file size/duplication warnings.
  Go formatting and whitespace checks passed.

No production agent-delivery behavior or remote executor build was changed.
