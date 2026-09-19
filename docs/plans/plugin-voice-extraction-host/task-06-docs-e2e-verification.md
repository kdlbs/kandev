---
id: "06-docs-e2e-verification"
title: "Document and verify extraction prerequisites"
status: done
wave: 3
depends_on: ["03-chat-composer-adapters", "04-creation-composer-adapters", "05-authenticated-webhook-handler"]
plan: "plan.md"
spec: "../../specs/plugins/requirements/voice-extraction-host.md"
---

# Task 06: Document And Verify Extraction Prerequisites

## Acceptance

- Public authoring/manifest docs and the plugin spec describe composer slots, webhook access/size
  fields, min-version rules, cancellation, plugin-owned secret settings, and the separate localization
  prerequisite.
- Desktop and Pixel 5 Playwright prove selection insertion plus native submit for task chat, task
  creation, and new session; Quick Chat is covered wherever its toolbar is exposed.
- Core Voice Mode remains present and its focused unit/E2E coverage still passes, establishing a safe
  prerequisite release for the later external plugin task.

## Verification

```bash
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
make build-web && make build-backend
cd apps/web && pnpm e2e:raw --project=chrome e2e/tests/plugins/composer-actions.spec.ts e2e/tests/chat/quick-chat.spec.ts e2e/tests/session/new-session-dialog.spec.ts
cd apps/web && pnpm e2e:raw --project=mobile-chrome e2e/tests/plugins/mobile-composer-actions.spec.ts e2e/tests/chat/mobile-voice-mode.spec.ts e2e/tests/session/mobile-new-session-dialog.spec.ts
node --test scripts/validate-public-docs.test.mjs
```

## Files Likely Touched

- `docs/specs/plugins/requirements/plugins.md`
- `docs/plans/plugins/PLUGIN-API.md`
- `docs/public/plugins-authoring.md`
- `docs/public/plugins-manifest.md`
- `apps/web/e2e/tests/plugins/composer-actions.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-composer-actions.spec.ts`
- fixture/plugin E2E helpers and existing Quick Chat/New Session specs where needed

## Mobile Design Contract

Pixel 5 tests use the shipped task toolbar, Task Create dialog, and New Session full-height dialog.
They assert the action is touch-reachable, native submission completes, the intended surface owns
scrolling, and the document has no horizontal overflow.

## Risks

- A mock that calls the submit callback directly does not prove native behavior; E2E must interact
  through a registered fixture plugin slot and observe the created message/task/session.
- Do not remove or weaken current Voice Mode tests. Extraction and core removal belong to the later
  dedicated plugin task.

## Outcome

The E2E pass found four defects the wave-2 unit tests could not see, all fixed in the same change:

1. `ChatInputToolbar` never forwarded `composerCapability` to `MobileChatInputToolbar`, and
   `DesktopChatInputToolbar` never forwarded it to `DesktopRightSection`. Task chat therefore always
   received the always-`unavailable` fallback on every presentation; only the creation forms worked.
   The existing toolbar unit test mocks the slot away, so nothing caught it. Covered now by
   `chat-input-toolbar-composer.test.tsx`, which renders the real top-level toolbar and asserts on
   what the slot received at desktop, mobile and tablet.
2. Both composer adapters rebuilt and revoked the capability on nearly every render, so an
   asynchronous plugin action (record, transcribe, insert) failed with `unavailable` the moment the
   host re-rendered. `useStablePluginComposerCapability` now issues one object per mount and reads
   live state through a ref, which is also what makes "revalidate at call time" true rather than
   aspirational.
3. The TipTap adapter derived the word-boundary spacing rule by indexing `getValue()` (markdown)
   with `getSelectionStart()` (a ProseMirror doc position), so the leading space was silently
   dropped. `TipTapInputHandle.getCharBefore()` now reads it from the document.
4. `submit()` ignored `hasContent`, so it submitted an empty draft while the same slot advertised
   `submittable: false`.
