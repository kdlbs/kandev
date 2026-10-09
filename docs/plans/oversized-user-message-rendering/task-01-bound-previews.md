---
id: "01-bound-previews"
title: "Bound user-message previews"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-BOUNDED-USER-MESSAGE-001
acceptance_criteria:
  - AC-UI-BOUNDED-USER-MESSAGE-001.1
  - AC-UI-BOUNDED-USER-MESSAGE-001.2
  - AC-UI-BOUNDED-USER-MESSAGE-001.3
  - AC-UI-BOUNDED-USER-MESSAGE-001.4
  - AC-UI-BOUNDED-USER-MESSAGE-001.5
  - AC-UI-BOUNDED-USER-MESSAGE-001.6
system_design:
  - ../../specs/ui/system-design/bounded-user-message-rendering.md
---

# Task 01: Bound user-message previews

## Summary

Limit user-message rendering before expensive Markdown work and DOM creation.
Keep the complete content available through download and existing mutation paths.

## In scope

- Pure prefix selection and shared preview presentation for transcript, raw, instruction, pinned, and queued surfaces.
- English, Portuguese, Simplified Chinese, and generated Traditional Chinese copy.
- Focused unit/component regressions, desktop/phone E2E, and a short explanation in `docs/public/use-kandev.md`.

## Out of scope

Provider recovery, timeout increases, backend/storage changes, composer redesign, assistant rendering, and plugin views.

## Acceptance

1. All specified render paths obey both budgets before Markdown parsing or raw DOM creation, including hidden and expanded copies.
2. Downloads match complete source; normal messages, copy, queue editing, delivery, attachments, and instruction semantics retain their behavior.
3. Desktop and phone flows satisfy AC .1-.6 with reachable localized actions and no request-timeout diagnostics.

## ASCII UI preview

UI-01 and UI-02 excerpts from the [full plan](plan.md#ascii-ui-preview):

```text
Desktop bubble                 Phone bubble
[bounded Markdown excerpt]     [bounded Markdown excerpt]
Preview shortened              Preview shortened
[Download full text]           [   Download full text   ]

Pinned/queued collapsed        Pinned/queued expanded
[two clipped lines] [v]        [bounded excerpt]       [^]
Shortened [Download]           Shortened [Download]
```

AC .1-.5 require this order. Notices and download controls stay outside clipping.
Phone actions measure at least 44px. The transcript remains the scroll owner.
Phone uses the original bubble instead of a pinned bar.

## TDD sequence

1. Add `user-message-body.test.tsx` with `bounds oversized user text before Markdown rendering`.
   Use the 3,921-line generator from the plan and the real renderer.
   Assert that the final marker is absent from the preview and no more than 199 break elements mount.
   Run it before production edits; the current renderer mounts 3,920 breaks.
2. Add helper cases at, below, and above both limits. Include a giant single line, CRLF, CR, empty text, and surrogate pairs.
3. Implement the helper and preview component. Retain source references and create downloads only on activation.
4. Integrate transcript/raw/instruction, pinned, and queued paths. Test whole-source replacement and complete downloads.
5. Add browser scenarios, localize labels, generate Traditional Chinese, and update public documentation.
6. Run every verification command and record results. Mark this task and the plan complete only after implementation checks pass.

## Verification

Run from the repository root. In a fresh worktree, first install dependencies with `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run lib/utils/message-preview.test.ts components/task/chat/messages/user-message-body.test.tsx components/task/chat/messages/chat-message.test.tsx components/task/chat/anchored-last-prompt-bar.test.tsx components/task/chat/queued-ghost-message.test.tsx lib/utils/file-download.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/utils/message-preview.ts components/task/chat/messages/bounded-message-preview.tsx components/task/chat/messages/user-message-body.tsx components/task/chat/anchored-last-prompt-bar.tsx components/task/chat/queued-ghost-message.tsx)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/oversized-user-message.spec.ts tests/chat/last-prompt-scroll.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-oversized-user-message.spec.ts tests/chat/mobile-last-prompt-scroll.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Managed E2E commands build the production application. Run them sequentially without worker overrides.
Use fixture-owned data. Arm `watchWs` before navigation and causal waits before submission.
Download assertions compare bytes, including the omitted tail. API assertions verify the complete stored message after reload.
Use the mock agent to prove the subsequent small-message turn completes.
Record browser measurements separately from deterministic size and interaction assertions.

## Files likely touched

- `apps/web/lib/utils/message-preview.ts` and `message-preview.test.ts` (new)
- `apps/web/components/task/chat/messages/bounded-message-preview.tsx` (new)
- `apps/web/components/task/chat/messages/user-message-body.tsx` and `user-message-body.test.tsx` (new test)
- `apps/web/components/task/chat/messages/chat-message.test.tsx`
- `apps/web/components/task/chat/anchored-last-prompt-bar.tsx` and `.test.tsx`
- `apps/web/components/task/chat/queued-ghost-message.tsx` and `.test.tsx`
- `apps/web/e2e/tests/chat/oversized-user-message.spec.ts` (new)
- `apps/web/e2e/tests/chat/mobile-oversized-user-message.spec.ts` (new)
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`
- `docs/public/use-kandev.md`

Reuse `apps/web/lib/utils/file-download.ts`; change it only if the existing helper cannot satisfy exact-content download.

## Dependencies

None. Explicit implementation request after the design-package handoff.

## Risks

Original incident payload is unavailable. Provider disconnections and all reported RPC delays remain unproven by this regression.
Do not turn the rendering cap into a message-storage limit or an agent-input truncation.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/bounded-user-message-rendering.md)
- [System design](../../specs/ui/system-design/bounded-user-message-rendering.md)
- Existing chat-message, queued-message, anchored-prompt, download, and last-prompt-scroll tests.
- `apps/web/AGENTS.md`, mobile-parity, TDD, and E2E skills.

## Results

Implemented the shared 200-line and 16,000-code-unit preview bound across transcript, raw,
instruction, pinned, and queued user-message surfaces. Complete source remains available to
storage, agent delivery, copy, editing, and downloads. Segmented transcript previews retain the
full original display message as their download source, while instruction disclosures retain
their scoped source. Added localized notice and download actions, public documentation, focused
regressions, and desktop/mobile E2E coverage.

Verification passed:

- 7 focused Vitest files, 140 tests.
- Web typecheck, lint, production Vite build, and i18n checks.
- 13 desktop Chromium tests covering oversized and last-prompt flows. The oversized flow submits a
  3,921-line, approximately 328 KB generated log, checks reload independently, switches away and
  back through the in-app sidebar, and verifies a subsequent small response completes and persists.
- 2 mobile Chromium tests covering oversized and last-prompt flows. The oversized flow uses the
  same incident-sized log, switches through the in-app task drawer, verifies the queued preview and
  download, clears that preview-only queue entry, and verifies the `/slow 10s` follow-up completes
  and persists.
- Public documentation validators and catalog validation.
- `git diff --check`.

Review remediation also passed:

- The bounded preview component test keeps the download target at 44px for a mobile responsive
  state with a fine pointer.
- The desktop oversized-message suite passed 3 tests, including a 500px fine-pointer viewport;
  the mobile oversized-message suite passed 1 test. Both suites use unambiguous preview and queue
  row locators.

`pnpm run i18n:zh-hant` remains blocked by the existing `openAgentSettings` residual simplified
entries in the workflows catalogs; it refused to write and the complete Traditional Chinese task
catalogs pass `i18n:check`. Full specification lint passed after the pre-existing duplicate
acceptance criterion in `docs/specs/tasks/requirements/queued-session-ownership.md` was assigned
the unique ID `AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.10`.
