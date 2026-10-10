---
id: "02-prove-chat-continuity"
title: "Prove desktop and phone continuity through ACP"
status: done
wave: 2
depends_on:
  - "01-separate-evidence-and-transcript"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.20
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.21
system_design:
  - ../../specs/platform/system-design/provider-error-recovery-03.md
---

# Task 02: Prove Desktop and Phone Continuity through ACP

## Summary

Prove that separately emitted ACP chunks containing `i/o timeout` persist and
render as one intact assistant message, live and after reload. Use the existing
mock inline script and task-chat desktop/phone surfaces without production UI
or mock-agent changes.

## In scope

- Shared helper builds the exact four-chunk multiline `e2e:message(...)` script
  from the plan. Include the later inline-code occurrence from the supplied
  investigation in the same response to exercise repeated transitions.
- Create a disposable task through `apiClient.createTaskWithAgent`, retain its
  returned task/session IDs, and open `/t/<task-id>` directly. Do not manufacture
  the final assistant row via API or client-store injection.
- Wait for the successful turn to settle and inspect its assistant records via
  `listSessionMessages`. Require one relevant row with the full exact response.
- Assert one relevant card, complete inline code spans, intact `errors`, and
  no retry notice. Scope uniqueness by message ID/content; do not hide
  duplicate rows with `.first()` or `.last()`.
- Reload and repeat DOM plus persisted-record assertions on Chromium and the
  configured Pixel 5 project. Phone checks zero document horizontal overflow
  using the existing shared helper and existing direct Chat composition.

## Out of scope

Recovery policy admission (Task 01), history repair, UI markup/style changes,
new localized copy, new mock scenarios, provider capability changes and full E2E.

## Acceptance

1. Desktop and phone prove the real mock ACP-to-runtime-to-storage-to-chat
   path, with one relevant assistant row/card and intact markdown before and
   after reload. A script made of separate chunks is required.
2. Phone preserves the existing direct task-chat surface and document
   containment. Both viewports show ordinary successful completion without a
   retry banner for the error-like prose.
3. Each managed E2E command discovers and passes its intended test(s), with a
   fresh build and final results recorded in this work order and plan.

## ASCII UI preview

UI-01 from the [full plan preview](plan.md#ascii-ui-preview), mapped to `.20`
and `.21`:

```text
Desktop: one [Assistant] card
934 <code>dial tcp <ip>:6379: i/o timeout</code> errors,
meaning the TCP connect failed.

Phone: one [Assistant] card in existing Chat
934 <code>dial tcp <ip>:6379:
i/o timeout</code> errors,
meaning the TCP connect failed.
```

Line breaks are illustrative. One message/card and intact code spans are
required; existing actions, composer and chat scroll ownership are retained.

## Verification

Run from repository root. Managed runs build and clean up their isolated
instances; do not run these concurrently or override worker limits.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/chat/provider-diagnostic-continuity.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-provider-diagnostic-continuity.spec.ts)
git diff --check
```

Task 01 provides the deterministic backend RED. These browser assertions are
the integration proof after that correction. If run against the pre-fix
backend for additional RED evidence, record its actual duplicate-card failure.
If a new worktree is used, first run `(cd apps && pnpm install --frozen-lockfile)`.

## Files likely touched

- `apps/web/e2e/tests/chat/provider-diagnostic-continuity.spec.ts` (new)
- `apps/web/e2e/tests/chat/mobile-provider-diagnostic-continuity.spec.ts` (new)
- `apps/web/e2e/tests/chat/provider-diagnostic-continuity-helpers.ts` (new)
- This work order and `plan.md` for status/results only

## Dependencies

Task 01 must be done. Read `apps/web/AGENTS.md`, `/e2e`, `/mobile-parity`,
`api-client.ts`, `SessionPage`, and the existing markdown separator desktop and
phone tests before implementation. Existing multiline mock scripting already
emits one ACP notification per `e2e:message` line.

## Risks

Seeding a combined row would bypass the defect. Checking only combined visible
text could pass with multiple cards. Pair persisted uniqueness with scoped DOM
uniqueness and inline-code assertions. Managed runner discovery must match the
explicit project; a zero-test run is not proof.

## Parallelism

`sequential`

## Inputs

- [Provider recovery requirement](../../specs/platform/requirements/provider-error-recovery.md), `.20` and `.21`.
- [Part-3 design](../../specs/platform/system-design/provider-error-recovery-03.md#marker-propagation).
- `apps/backend/cmd/mock-agent/script.go`: existing `e2e:message(...)` support.
- `apps/web/e2e/tests/chat/markdown-paragraph-breaks.spec.ts` and
  `mobile-markdown-separators.spec.ts`: route, rendering, reload and containment patterns.

## Results

Complete on 2026-10-06. Both managed Playwright tests passed sequentially:

- `(cd apps/web && pnpm e2e:run --project chromium tests/chat/provider-diagnostic-continuity.spec.ts)`: passed, 26.2s.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-provider-diagnostic-continuity.spec.ts)`: passed, 29.9s.

The shared ACP helper emits the exact split `err` / `ors` boundary and a
repeated timeout occurrence. It waits for one uniquely matching persisted
assistant row with the exact complete source, scopes the rendered message to
that row's ID, and checks the rendered text and both inline code spans
separately from persisted Markdown source. Desktop reload and phone reload
retain the same single record; the phone also checks document containment. No
retry card appears on the successful turn. Prettier and focused ESLint passed
for all three new E2E files.
